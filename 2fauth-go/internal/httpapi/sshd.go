package httpapi

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"2fauth/internal/otp"
	"2fauth/internal/store"
)

const (
	sshHandshakeTimeout = 10 * time.Second
	// sshConnectionTimeout ends every connection: a command takes milliseconds.
	sshConnectionTimeout = time.Minute
	sshMaxConnections    = 64
	sshMaxCommand        = 512
	// A client that keeps failing is cut off before any work is done for it.
	sshMaxFailures   = 20
	sshFailureWindow = 5 * time.Minute

	sshExitOK       = 0
	sshExitFailure  = 1
	sshExitUsage    = 2
	sshKeyExtension = "2fauth-key-id"
)

// ListenSSH opens the listener of the command line service. ALLOWED_CLIENT_IPS applies
// here too, before any byte is read: this is a second way in to the same codes.
func (s *Server) ListenSSH() (net.Listener, error) {
	listener, err := net.Listen("tcp", s.cfg.SSHListenAddr)
	if err != nil {
		return nil, err
	}
	if !s.allowedClients.Empty() {
		listener = &AllowListener{Listener: listener, Allowed: s.allowedClients, OnRefused: func(remote string) {
			slog.Warn("SSH connection refused by ALLOWED_CLIENT_IPS", "remote", remote)
		}}
	}
	return listener, nil
}

// loadSSHHostKey reads the host key, creating it on first use.
func (s *Server) loadSSHHostKey() (ssh.Signer, error) {
	path := s.cfg.SSHHostKeyFile
	if path == "" {
		path = filepath.Join(s.cfg.BasePath, "storage", "ssh_host_ed25519_key")
	}
	if raw, err := os.ReadFile(path); err == nil {
		return ssh.ParsePrivateKey(raw)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	block, err := ssh.MarshalPrivateKey(private, "2fauth host key")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// O_EXCL: two instances starting together must not overwrite each other's key.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := pem.Encode(file, block); err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(private)
}

// ServeSSH answers command line clients until the context ends. A client signs in with
// a key registered in the panel, exactly like git over SSH: no password, no shell, and
// nothing but the commands below.
func (s *Server) ServeSSH(ctx context.Context, listener net.Listener) error {
	hostKey, err := s.loadSSHHostKey()
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("host key: %w", err)
	}
	s.sshMu.Lock()
	s.sshHostFingerprint = ssh.FingerprintSHA256(hostKey.PublicKey())
	s.sshMu.Unlock()

	config := &ssh.ServerConfig{
		ServerVersion: "SSH-2.0-2FAuth",
		MaxAuthTries:  6,
		PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			return s.authorizeSSHKey(key)
		},
	}
	config.AddHostKey(hostKey)

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	slots := make(chan struct{}, sshMaxConnections)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			slog.Warn("SSH accept failed", "error", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		select {
		case slots <- struct{}{}:
			go func() {
				defer func() { <-slots }()
				s.handleSSHConnection(conn, config)
			}()
		default:
			_ = conn.Close()
		}
	}
}

// authorizeSSHKey is called for each key a client offers. The library then checks the
// signature proving the client holds the private half, and only then is the connection
// authenticated with these permissions.
func (s *Server) authorizeSSHKey(offered ssh.PublicKey) (*ssh.Permissions, error) {
	key, err := s.st.FindSSHKeyByFingerprint(ssh.FingerprintSHA256(offered))
	if err != nil {
		return nil, errors.New("unknown public key")
	}
	return &ssh.Permissions{Extensions: map[string]string{sshKeyExtension: strconv.FormatInt(key.ID, 10)}}, nil
}

func (s *Server) handleSSHConnection(raw net.Conn, config *ssh.ServerConfig) {
	defer raw.Close()
	ip := peerAddr(raw.RemoteAddr().String()).String()
	if s.sshFailures.Count(ip, sshFailureWindow) >= sshMaxFailures {
		return
	}

	_ = raw.SetDeadline(time.Now().Add(sshHandshakeTimeout))
	conn, channels, requests, err := ssh.NewServerConn(raw, config)
	if err != nil {
		// Scanners that never try to sign in count as well: they get nothing either.
		s.sshFailures.Allow(ip, sshMaxFailures, sshFailureWindow)
		slog.Warn("SSH sign-in failed", "remote", ip, "error", err)
		return
	}
	defer conn.Close()
	_ = raw.SetDeadline(time.Now().Add(sshConnectionTimeout))
	go ssh.DiscardRequests(requests)

	keyID, err := strconv.ParseInt(conn.Permissions.Extensions[sshKeyExtension], 10, 64)
	if err != nil {
		return
	}
	for channel := range channels {
		if channel.ChannelType() != "session" {
			_ = channel.Reject(ssh.Prohibited, "only commands are available")
			continue
		}
		session, requests, err := channel.Accept()
		if err != nil {
			continue
		}
		go s.handleSSHSession(session, requests, keyID, ip)
	}
}

// handleSSHSession runs the one command of a session. Everything else a client can ask
// for (subsystems such as sftp, environment variables) is declined.
func (s *Server) handleSSHSession(session ssh.Channel, requests <-chan *ssh.Request, keyID int64, ip string) {
	defer session.Close()
	terminal := false
	for request := range requests {
		switch request.Type {
		case "pty-req":
			terminal = true
			_ = request.Reply(true, nil)
		case "shell":
			_ = request.Reply(true, nil)
			s.finishSSHCommand(session, terminal, keyID, ip, "")
			return
		case "exec":
			var payload struct{ Command string }
			if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
				_ = request.Reply(false, nil)
				return
			}
			_ = request.Reply(true, nil)
			s.finishSSHCommand(session, terminal, keyID, ip, payload.Command)
			return
		default:
			_ = request.Reply(false, nil)
		}
	}
}

func (s *Server) finishSSHCommand(session ssh.Channel, terminal bool, keyID int64, ip, command string) {
	var stdout, stderr io.Writer = session, session.Stderr()
	if terminal {
		stdout, stderr = crlfWriter{session}, crlfWriter{session.Stderr()}
	}
	status := s.runSSHCommand(stdout, stderr, keyID, ip, command)
	_, _ = session.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(status)}))
}

// crlfWriter is what a terminal expects when a client asked for one.
type crlfWriter struct{ w io.Writer }

func (c crlfWriter) Write(p []byte) (int, error) {
	if _, err := c.w.Write([]byte(strings.ReplaceAll(string(p), "\n", "\r\n"))); err != nil {
		return 0, err
	}
	return len(p), nil
}

const sshHelp = `Commands:
  get <account> [--json]   print the current code of an account
  list [--json]            list the accounts this key can read
  whoami                   show this key and what it can read
  help                     show this message

<account> is an id from "list", a service name, "service:account", or part of them.
Example: ssh -p <port> otp@<host> get github
`

// runSSHCommand executes a command and returns its exit status. The key and its owner
// are read again for each command, so a revoked key stops working at once.
func (s *Server) runSSHCommand(stdout, stderr io.Writer, keyID int64, ip, command string) int {
	key, err := s.st.GetSSHKey(keyID)
	if err != nil {
		fmt.Fprintln(stderr, "This key is no longer registered.")
		return sshExitFailure
	}
	user, err := s.st.GetUser(key.UserID)
	if err != nil {
		fmt.Fprintln(stderr, "This key is no longer registered.")
		return sshExitFailure
	}
	if len(command) > sshMaxCommand {
		fmt.Fprintln(stderr, "The command is too long.")
		return sshExitUsage
	}
	_ = s.st.TouchSSHKey(key.ID, ip)

	args := strings.Fields(command)
	asJSON := false
	kept := args[:0]
	for _, arg := range args {
		if arg == "--json" {
			asJSON = true
		} else {
			kept = append(kept, arg)
		}
	}
	args = kept

	if len(args) == 0 {
		fmt.Fprintf(stdout, "Hi %s! Key %q is registered.\n\n%s", user.Name, key.Name, sshHelp)
		return sshExitOK
	}
	accounts, err := s.sshKeyAccounts(key, user.ID)
	if err != nil {
		slog.Error("SSH command failed", "error", err)
		fmt.Fprintln(stderr, "Server error.")
		return sshExitFailure
	}

	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprint(stdout, sshHelp)
		return sshExitOK
	case "whoami":
		scope := fmt.Sprintf("%d account(s)", len(accounts))
		if key.AllAccounts {
			scope = fmt.Sprintf("all accounts, %d right now", len(accounts))
		}
		fmt.Fprintf(stdout, "%s (%s)\nkey: %s %s\nreads: %s\n", user.Name, user.Email, key.Name, key.Fingerprint, scope)
		return sshExitOK
	case "list", "ls":
		return writeSSHList(stdout, accounts, asJSON)
	case "get":
		args = args[1:]
		if len(args) == 0 {
			fmt.Fprintln(stderr, "Usage: get <account> [--json]")
			return sshExitUsage
		}
	}

	account, candidates := findSSHAccount(accounts, strings.Join(args, " "))
	if account == nil {
		if len(candidates) == 0 {
			// The same answer whether the account does not exist or is out of scope.
			fmt.Fprintf(stderr, "No account matches %q for this key.\n", strings.Join(args, " "))
			return sshExitFailure
		}
		fmt.Fprintf(stderr, "%q matches several accounts, use an id:\n", strings.Join(args, " "))
		for _, candidate := range candidates {
			fmt.Fprintf(stderr, "  %d\t%s\t%s\n", candidate.ID, candidate.Service, candidate.Account)
		}
		return sshExitUsage
	}
	return s.writeSSHCode(stdout, stderr, account, user, ip, asJSON)
}

// sshKeyAccounts lists what a key reaches: what its owner can see now, within its scope.
func (s *Server) sshKeyAccounts(key *store.SSHKey, userID int64) ([]*store.Account, error) {
	visible, err := s.st.ListVisibleAccounts(userID, s.accountScope(), nil)
	if err != nil {
		return nil, err
	}
	if key.AllAccounts {
		return visible, nil
	}
	accounts := map[int64]bool{}
	for _, id := range key.AccountIDs {
		accounts[id] = true
	}
	groups := map[int64]bool{}
	for _, id := range key.GroupIDs {
		groups[id] = true
	}
	out := make([]*store.Account, 0, len(visible))
	for _, account := range visible {
		if accounts[account.ID] {
			out = append(out, account)
			continue
		}
		if len(groups) > 0 {
			if group, err := s.st.AccountGroupID(account, userID); err == nil && groups[group] {
				out = append(out, account)
			}
		}
	}
	return out, nil
}

// findSSHAccount resolves a query. It returns the account, or the candidates when the
// best kind of match is not unique. A wrong code is worse than no code, so a query is
// never settled by picking the first of several accounts.
func findSSHAccount(accounts []*store.Account, query string) (*store.Account, []*store.Account) {
	query = strings.TrimSpace(query)
	if id, err := strconv.ParseInt(strings.TrimPrefix(query, "#"), 10, 64); err == nil {
		for _, account := range accounts {
			if account.ID == id {
				return account, nil
			}
		}
	}
	folded := strings.ToLower(query)
	service, name, qualified := strings.Cut(folded, ":")

	matchers := []func(a *store.Account) bool{
		func(a *store.Account) bool {
			return qualified && strings.EqualFold(a.Service, service) && strings.EqualFold(a.Account, name)
		},
		func(a *store.Account) bool { return strings.EqualFold(a.Service, query) },
		func(a *store.Account) bool { return strings.EqualFold(a.Account, query) },
		func(a *store.Account) bool {
			return strings.Contains(strings.ToLower(a.Service+":"+a.Account), folded)
		},
	}
	for _, match := range matchers {
		var hits []*store.Account
		for _, account := range accounts {
			if match(account) {
				hits = append(hits, account)
			}
		}
		if len(hits) == 1 {
			return hits[0], nil
		}
		if len(hits) > 1 {
			return nil, hits
		}
	}
	return nil, nil
}

func writeSSHList(stdout io.Writer, accounts []*store.Account, asJSON bool) int {
	if asJSON {
		out := make([]map[string]any, 0, len(accounts))
		for _, account := range accounts {
			out = append(out, map[string]any{"id": account.ID, "service": account.Service, "account": account.Account, "otp_type": account.OTPType})
		}
		_ = json.NewEncoder(stdout).Encode(out)
		return sshExitOK
	}
	for _, account := range accounts {
		fmt.Fprintf(stdout, "%d\t%s\t%s\t%s\n", account.ID, account.Service, account.Account, account.OTPType)
	}
	return sshExitOK
}

// writeSSHCode prints a code. It goes through the same generation as the API, so an
// HOTP counter moves and the event is in the OTP log of the account.
func (s *Server) writeSSHCode(stdout, stderr io.Writer, account *store.Account, user *store.User, ip string, asJSON bool) int {
	acc := otp.Account{
		Type: account.OTPType, Secret: account.Secret, Algorithm: account.Algorithm, Digits: account.Digits,
		Period: account.Period, Counter: account.Counter, Service: account.Service, Account: account.Account,
	}
	payload, err := s.generateOTPPayload(acc, account, true)
	if err != nil {
		fmt.Fprintln(stderr, "This account has an invalid secret.")
		return sshExitFailure
	}
	var counter *int64
	if account.HOTP() {
		counter = account.Counter
	}
	s.logOtpFor(user, ip, account, account.OTPType, counter)

	if !asJSON {
		fmt.Fprintln(stdout, payload["password"])
		return sshExitOK
	}
	payload["id"] = account.ID
	payload["service"] = account.Service
	payload["account"] = account.Account
	if !account.HOTP() {
		now := time.Now().Unix()
		payload["expires_in"] = otp.ExpiresIn(acc, now)
	}
	_ = json.NewEncoder(stdout).Encode(payload)
	return sshExitOK
}
