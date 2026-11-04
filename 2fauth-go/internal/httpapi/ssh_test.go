package httpapi

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"2fauth/internal/config"
	otpgen "2fauth/internal/otp"
)

// sshIdentity is the key pair of a test machine.
type sshIdentity struct {
	signer ssh.Signer
	line   string
}

func newSSHIdentity(t *testing.T) sshIdentity {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return sshIdentity{signer: signer, line: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))) + " me@laptop"}
}

// startSSH runs the SSH service of a server on a free port.
func startSSH(t *testing.T, srv *Server) string {
	t.Helper()
	srv.cfg.SSHListenAddr = "127.0.0.1:0"
	listener, err := srv.ListenSSH()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.ServeSSH(ctx, listener)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return listener.Addr().String()
}

func dialSSH(addr string, id sshIdentity) (*ssh.Client, error) {
	return ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            "otp",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(id.signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
}

// runSSH runs one command and returns its output and exit status.
func runSSH(t *testing.T, addr string, id sshIdentity, command string) (string, string, int) {
	t.Helper()
	conn, err := dialSSH(addr, id)
	if err != nil {
		t.Fatalf("dial for %q: %v", command, err)
	}
	defer conn.Close()
	session, err := conn.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var stdout, stderr strings.Builder
	session.Stdout, session.Stderr = &stdout, &stderr
	status := 0
	if err := session.Run(command); err != nil {
		var exit *ssh.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("run %q: %v", command, err)
		}
		status = exit.ExitStatus()
	}
	return stdout.String(), stderr.String(), status
}

// sshAccount creates an account and returns its id.
func sshAccount(t *testing.T, c *client, service, account string, extra string) int64 {
	t.Helper()
	body := fmt.Sprintf(`{"otp_type":"totp","service":%q,"account":%q,"secret":%q%s}`, service, account, testSecret, extra)
	rec := c.do(http.MethodPost, "/api/v1/twofaccounts", body, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s = %d (%s)", service, rec.Code, rec.Body.String())
	}
	return accountID(t, rec.Body.Bytes())
}

// accountID reads the id of an account response.
func accountID(t *testing.T, body []byte) int64 {
	t.Helper()
	var payload struct {
		ID   int64 `json:"id"`
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ID != 0 {
		return payload.ID
	}
	return payload.Data.ID
}

// sshAddKey registers a key through the API and returns its id.
func sshAddKey(t *testing.T, c *client, line, scope string) int64 {
	t.Helper()
	body, _ := json.Marshal(line)
	rec := c.do(http.MethodPost, "/api/v1/ssh-keys", fmt.Sprintf(`{"name":"laptop","public_key":%s,"password":%q,%s}`, body, testPassword, scope), nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add key = %d (%s)", rec.Code, rec.Body.String())
	}
	return int64(jsonBody(t, rec)["id"].(float64))
}

func TestSSHKeyManagement(t *testing.T) {
	srv := testServer(t)
	alice := registered(t, srv, "alice")
	bob := registered(t, srv, "bob")
	own := sshAccount(t, alice, "GitHub", "alice", "")
	foreign := sshAccount(t, bob, "GitLab", "bob", "")
	id := newSSHIdentity(t)
	line, _ := json.Marshal(id.line)
	post := func(c *client, body string) (int, map[string]any) {
		rec := c.do(http.MethodPost, "/api/v1/ssh-keys", body, nil)
		out := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	weakKey, err := ssh.NewPublicKey(&weak.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	refused := map[string]string{
		"no password":     fmt.Sprintf(`{"name":"a","public_key":%s,"all_accounts":true}`, line),
		"wrong password":  fmt.Sprintf(`{"name":"a","public_key":%s,"all_accounts":true,"password":"nope"}`, line),
		"no scope":        fmt.Sprintf(`{"name":"a","public_key":%s,"password":%q}`, line, testPassword),
		"foreign account": fmt.Sprintf(`{"name":"a","public_key":%s,"password":%q,"account_ids":"%d"}`, line, testPassword, foreign),
		"not a key":       fmt.Sprintf(`{"name":"a","public_key":"hello","all_accounts":true,"password":%q}`, testPassword),
		"private key":     fmt.Sprintf(`{"name":"a","public_key":"-----BEGIN OPENSSH PRIVATE KEY-----","all_accounts":true,"password":%q}`, testPassword),
		"options":         fmt.Sprintf(`{"name":"a","public_key":"from=\"10.0.0.1\" %s","all_accounts":true,"password":%q}`, strings.TrimSpace(id.line), testPassword),
		"two keys":        fmt.Sprintf(`{"name":"a","public_key":"%s\n%s","all_accounts":true,"password":%q}`, id.line, id.line, testPassword),
		"weak rsa":        fmt.Sprintf(`{"name":"a","public_key":%q,"all_accounts":true,"password":%q}`, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(weakKey))), testPassword),
		"no name":         fmt.Sprintf(`{"public_key":%s,"all_accounts":true,"password":%q}`, line, testPassword),
	}
	for name, body := range refused {
		if code, out := post(alice, body); code == http.StatusCreated {
			t.Errorf("%s: key accepted: %v", name, out)
		}
	}

	code, out := post(alice, fmt.Sprintf(`{"name":"laptop","public_key":%s,"password":%q,"account_ids":"%d"}`, line, testPassword, own))
	if code != http.StatusCreated {
		t.Fatalf("create = %d %v", code, out)
	}
	keyID := int64(out["id"].(float64))
	if strings.Contains(fmt.Sprint(out), "me@laptop") || out["key_type"] != "ssh-ed25519" {
		t.Errorf("payload = %v", out)
	}

	// A key belongs to one user, as on GitHub.
	if code, _ := post(bob, fmt.Sprintf(`{"name":"same","public_key":%s,"password":%q,"all_accounts":true}`, line, testPassword)); code != http.StatusUnprocessableEntity {
		t.Errorf("same key for another user = %d", code)
	}
	path := fmt.Sprintf("/api/v1/ssh-keys/%d", keyID)
	if rec := bob.do(http.MethodDelete, path, "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("delete by another user = %d", rec.Code)
	}
	if rec := bob.do(http.MethodPut, path, fmt.Sprintf(`{"name":"x","all_accounts":true,"password":%q}`, testPassword), nil); rec.Code != http.StatusNotFound {
		t.Errorf("update by another user = %d", rec.Code)
	}
	// Widening a scope takes the password too.
	if rec := alice.do(http.MethodPut, path, `{"name":"x","all_accounts":true}`, nil); rec.Code == http.StatusOK {
		t.Error("scope widened without the password")
	}
	list := jsonBody(t, alice.do(http.MethodGet, "/api/v1/ssh-keys", "", nil))
	if list["enabled"] != false || len(list["keys"].([]any)) != 1 {
		t.Errorf("list = %v", list)
	}
	if rec := alice.do(http.MethodDelete, path, "", nil); rec.Code != http.StatusNoContent {
		t.Errorf("delete = %d", rec.Code)
	}

	// An API token cannot create a way in that outlives it.
	rec := alice.do(http.MethodPost, "/oauth/personal-access-tokens", `{"name":"t"}`, nil)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("token = %d (%s)", rec.Code, rec.Body.String())
	}
	token, _ := jsonBody(t, rec)["accessToken"].(string)
	if token == "" {
		t.Fatalf("no token: %s", rec.Body.String())
	}
	anon := &client{t: t, srv: srv}
	headers := map[string]string{"Authorization": "Bearer " + token}
	if rec := anon.do(http.MethodPost, "/api/v1/ssh-keys", fmt.Sprintf(`{"name":"a","public_key":%s,"password":%q,"all_accounts":true}`, line, testPassword), headers); rec.Code != http.StatusForbidden {
		t.Errorf("create through a token = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := anon.do(http.MethodGet, "/api/v1/ssh-keys", "", headers); rec.Code != http.StatusForbidden {
		t.Errorf("list through a token = %d", rec.Code)
	}
}

func TestSSHServiceHandsOutCodesInScope(t *testing.T) {
	srv := testServer(t)
	addr := startSSH(t, srv)
	alice := registered(t, srv, "alice")
	github := sshAccount(t, alice, "GitHub", "alice@example.org", "")
	gitlab := sshAccount(t, alice, "GitLab", "alice@example.org", "")
	mail1 := sshAccount(t, alice, "Mail", "one", "")
	mail2 := sshAccount(t, alice, "Mail", "two", "")

	stranger := newSSHIdentity(t)
	if _, err := dialSSH(addr, stranger); err == nil {
		t.Fatal("an unregistered key signed in")
	}

	// A key limited to one account.
	id := newSSHIdentity(t)
	keyID := sshAddKey(t, alice, id.line, fmt.Sprintf(`"account_ids":"%d"`, github))

	out, stderr, status := runSSH(t, addr, id, "get github")
	want, err := otpgen.Code(otpgen.Account{Type: "totp", Secret: testSecret, Algorithm: "sha1", Digits: 6}, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if status != 0 || len(strings.TrimSpace(out)) != 6 || stderr != "" {
		t.Fatalf("get = %q %q %d", out, stderr, status)
	}
	if strings.TrimSpace(out) != want {
		// The clock may have crossed a period between both computations.
		again, _ := otpgen.Code(otpgen.Account{Type: "totp", Secret: testSecret, Algorithm: "sha1", Digits: 6}, time.Now().Unix())
		if strings.TrimSpace(out) != again {
			t.Errorf("code = %q, want %q", out, want)
		}
	}

	// Out of scope and nonexistent answer alike, by id and by name.
	_, outOfScope, status := runSSH(t, addr, id, "get gitlab")
	if status != 1 || !strings.Contains(outOfScope, "No account matches") {
		t.Errorf("out of scope = %q %d", outOfScope, status)
	}
	_, unknown, _ := runSSH(t, addr, id, "get nothing-like-it")
	if strings.ReplaceAll(unknown, "nothing-like-it", "gitlab") != outOfScope {
		t.Errorf("different answers: %q / %q", unknown, outOfScope)
	}
	if _, _, status := runSSH(t, addr, id, fmt.Sprintf("get %d", gitlab)); status != 1 {
		t.Errorf("out of scope by id = %d", status)
	}
	listing, _, _ := runSSH(t, addr, id, "list")
	if !strings.Contains(listing, "GitHub") || strings.Contains(listing, "GitLab") || strings.Contains(listing, testSecret) {
		t.Errorf("list = %q", listing)
	}
	jsonOut, _, _ := runSSH(t, addr, id, "get github --json")
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil || payload["service"] != "GitHub" || payload["password"] == nil {
		t.Errorf("json = %q (%v)", jsonOut, err)
	}
	if _, ok := payload["secret"]; ok {
		t.Error("the secret left the server")
	}
	if strings.Contains(jsonOut, testSecret) {
		t.Error("the secret left the server")
	}

	// Each code is in the OTP log of the account.
	logs, err := srv.st.ListOtpLogs(github, 1, 10)
	if err != nil || len(logs) == 0 {
		t.Errorf("otp logs = %d (%v)", len(logs), err)
	}

	// Revocation works at once, including for an open connection.
	conn, err := dialSSH(addr, id)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if rec := alice.do(http.MethodDelete, fmt.Sprintf("/api/v1/ssh-keys/%d", keyID), "", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d", rec.Code)
	}
	session, err := conn.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if output, err := session.CombinedOutput("get github"); err == nil || strings.Contains(string(output), want) {
		t.Errorf("revoked key on an open connection: %q %v", output, err)
	}
	if _, err := dialSSH(addr, id); err == nil {
		t.Error("a revoked key signed in")
	}

	// All accounts: ambiguous names are refused instead of guessed.
	all := newSSHIdentity(t)
	sshAddKey(t, alice, all.line, `"all_accounts":true`)
	_, stderr, status = runSSH(t, addr, all, "get mail")
	if status != 2 || !strings.Contains(stderr, strconv.FormatInt(mail1, 10)) || !strings.Contains(stderr, strconv.FormatInt(mail2, 10)) {
		t.Errorf("ambiguous = %q %d", stderr, status)
	}
	for _, query := range []string{"mail:two", strconv.FormatInt(mail2, 10), "#" + strconv.FormatInt(mail2, 10), "two"} {
		if _, stderr, status := runSSH(t, addr, all, "get "+query); status != 0 {
			t.Errorf("get %s = %q %d", query, stderr, status)
		}
	}
	if _, _, status := runSSH(t, addr, all, "gitlab"); status != 0 {
		t.Errorf("bare name = %d", status)
	}
	if _, stderr, status := runSSH(t, addr, all, strings.Repeat("x", 600)); status != 2 {
		t.Errorf("long command = %q %d", stderr, status)
	}

	// Deleting an account also removes it from the scopes.
	narrow := newSSHIdentity(t)
	sshAddKey(t, alice, narrow.line, fmt.Sprintf(`"account_ids":"%d"`, gitlab))
	if rec := alice.do(http.MethodDelete, fmt.Sprintf("/api/v1/twofaccounts/%d", gitlab), "", nil); rec.Code >= 300 {
		t.Fatalf("delete account = %d", rec.Code)
	}
	if listing, _, _ := runSSH(t, addr, narrow, "list"); listing != "" {
		t.Errorf("list after delete = %q", listing)
	}
}

func TestSSHGroupScopeAndHOTP(t *testing.T) {
	srv := testServer(t)
	addr := startSSH(t, srv)
	alice := registered(t, srv, "alice")
	rec := alice.do(http.MethodPost, "/api/v1/groups", `{"name":"work"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("group = %d (%s)", rec.Code, rec.Body.String())
	}
	group := accountID(t, rec.Body.Bytes())
	inGroup := sshAccount(t, alice, "InGroup", "a", fmt.Sprintf(`,"group_id":%d`, group))
	sshAccount(t, alice, "Outside", "a", "")

	id := newSSHIdentity(t)
	sshAddKey(t, alice, id.line, fmt.Sprintf(`"group_ids":"%d"`, group))
	listing, _, _ := runSSH(t, addr, id, "list")
	if !strings.Contains(listing, "InGroup") || strings.Contains(listing, "Outside") {
		t.Errorf("list = %q", listing)
	}
	// An account joining the group is in scope from then on.
	later := sshAccount(t, alice, "Later", "a", fmt.Sprintf(`,"group_id":%d`, group))
	if _, _, status := runSSH(t, addr, id, fmt.Sprintf("get %d", later)); status != 0 {
		t.Errorf("account added to the group = %d", status)
	}
	if _, _, status := runSSH(t, addr, id, fmt.Sprintf("get %d", inGroup)); status != 0 {
		t.Errorf("account of the group = %d", status)
	}

	// HOTP: each code moves the counter, and is never handed out twice.
	body := fmt.Sprintf(`{"otp_type":"hotp","service":"Counter","account":"a","secret":%q,"counter":5,"group_id":%d}`, testSecret, group)
	rec = alice.do(http.MethodPost, "/api/v1/twofaccounts", body, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("hotp = %d (%s)", rec.Code, rec.Body.String())
	}
	hotp := accountID(t, rec.Body.Bytes())
	first, _, _ := runSSH(t, addr, id, "get counter")
	second, _, _ := runSSH(t, addr, id, "get counter")
	if first == second || first == "" {
		t.Errorf("hotp codes = %q %q", first, second)
	}
	account, err := srv.st.GetAccount(hotp)
	if err != nil || account.Counter == nil || *account.Counter != 7 {
		t.Errorf("counter = %v (%v)", account.Counter, err)
	}
}

func TestSSHScopeFollowsSharing(t *testing.T) {
	srv := testServer(t)
	addr := startSSH(t, srv)
	owner := registered(t, srv, "owner")
	bob := registered(t, srv, "bob")
	setSetting(t, owner, "enableSharing", true)
	shared := sshAccount(t, owner, "Shared", "a", "")
	share := fmt.Sprintf(`{"user_ids":[%d]}`, userID(t, srv, "bob"))
	base := fmt.Sprintf("/api/v1/twofaccounts/%d/shares", shared)
	if rec := owner.do(http.MethodPost, base, share, nil); rec.Code != http.StatusCreated {
		t.Fatalf("share = %d (%s)", rec.Code, rec.Body.String())
	}
	id := newSSHIdentity(t)
	sshAddKey(t, bob, id.line, fmt.Sprintf(`"account_ids":"%d"`, shared))
	if _, _, status := runSSH(t, addr, id, "get shared"); status != 0 {
		t.Errorf("shared account = %d", status)
	}
	// Taking the share back also takes it from the key.
	if rec := owner.do(http.MethodDelete, base, "", nil); rec.Code >= 300 {
		t.Fatalf("unshare = %d", rec.Code)
	}
	if _, _, status := runSSH(t, addr, id, "get shared"); status != 1 {
		t.Errorf("after the share ended = %d", status)
	}
}

func TestSSHServiceOffersNothingElse(t *testing.T) {
	srv := testServer(t)
	addr := startSSH(t, srv)
	alice := registered(t, srv, "alice")
	sshAccount(t, alice, "GitHub", "a", "")
	id := newSSHIdentity(t)
	sshAddKey(t, alice, id.line, `"all_accounts":true`)

	conn, err := dialSSH(addr, id)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Port forwarding: the classic way to use a "command only" SSH server as a proxy.
	if forwarded, err := conn.Dial("tcp", "127.0.0.1:80"); err == nil {
		forwarded.Close()
		t.Error("port forwarding is open")
	}
	if listener, err := conn.Listen("tcp", "127.0.0.1:0"); err == nil {
		listener.Close()
		t.Error("remote forwarding is open")
	}
	session, err := conn.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if err := session.RequestSubsystem("sftp"); err == nil {
		t.Error("sftp is open")
	}
	session.Close()

	// Password sign-in is not offered at all.
	if _, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User: "alice@example.org", Auth: []ssh.AuthMethod{ssh.Password(testPassword)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second,
	}); err == nil {
		t.Error("password sign-in works")
	}

	// What a command says is data: nothing runs on the server.
	marker := filepath.Join(t.TempDir(), "marker")
	if _, _, status := runSSH(t, addr, id, "get x; touch "+marker); status == 0 {
		t.Error("unexpected success")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("a shell ran the command")
	}
	greeting, _, status := runSSH(t, addr, id, "")
	if status != 0 || !strings.Contains(greeting, "Hi alice") {
		t.Errorf("greeting = %q %d", greeting, status)
	}
}

func TestSSHServiceHonoursTheAllowList(t *testing.T) {
	srv := testServerWith(t, func(cfg *config.Config) { cfg.AllowedClientIPs = "192.0.2.1" })
	addr := startSSH(t, srv)
	raw, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(2 * time.Second))
	banner := make([]byte, 16)
	if n, _ := raw.Read(banner); n > 0 {
		t.Errorf("a refused client read %q", banner[:n])
	}
}

func TestSSHServiceKeepsItsHostKey(t *testing.T) {
	srv := testServer(t)
	first, err := srv.loadSSHHostKey()
	if err != nil {
		t.Fatal(err)
	}
	second, err := srv.loadSSHHostKey()
	if err != nil {
		t.Fatal(err)
	}
	if ssh.FingerprintSHA256(first.PublicKey()) != ssh.FingerprintSHA256(second.PublicKey()) {
		t.Error("the host key changed")
	}
	path := filepath.Join(srv.cfg.BasePath, "storage", "ssh_host_ed25519_key")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
}

func TestSSHClientThatKeepsFailingIsCutOff(t *testing.T) {
	srv := testServer(t)
	addr := startSSH(t, srv)
	stranger := newSSHIdentity(t)
	for i := 0; i < sshMaxFailures; i++ {
		_, _ = dialSSH(addr, stranger)
	}
	raw, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(2 * time.Second))
	banner := make([]byte, 16)
	if n, _ := raw.Read(banner); n > 0 {
		t.Errorf("still answering: %q", banner[:n])
	}
}

func TestIconFillerGivesAccountsTheirIcons(t *testing.T) {
	srv, c := iconAdminClient(t)
	site := fakeIconSite(t)
	pointIconsAt(srv, site)
	setChain(t, c, `"selfhst"`, 1)

	missing := sshAccount(t, c, "GitHub", "a", "")
	own := sshAccount(t, c, "GitLab", "a", `,"icon":"mine.png"`)
	removed := sshAccount(t, c, "Gitea", "a", "")
	if rec := c.do(http.MethodPatch, fmt.Sprintf("/api/v1/twofaccounts/%d", removed),
		fmt.Sprintf(`{"otp_type":"totp","service":"Gitea","account":"a","secret":%q,"icon":""}`, testSecret), nil); rec.Code != http.StatusOK {
		t.Fatalf("remove icon = %d (%s)", rec.Code, rec.Body.String())
	}

	if n := srv.fillIcons(context.Background()); n != 1 {
		t.Errorf("filled = %d, want 1", n)
	}
	account, err := srv.st.GetAccount(missing)
	if err != nil || !strings.HasSuffix(account.Icon, ".svg") {
		t.Errorf("icon = %v (%v)", account, err)
	}
	if !srv.st.IconUploadedBy(account.Icon, account.UserID) {
		t.Error("the owner does not own the icon")
	}
	if kept, _ := srv.st.GetAccount(own); kept.Icon != "mine.png" {
		t.Errorf("an icon was replaced: %q", kept.Icon)
	}
	if kept, _ := srv.st.GetAccount(removed); kept.Icon != "" {
		t.Errorf("a removed icon came back: %q", kept.Icon)
	}

	// The preference turns it off: looking up tells the library the service name.
	if rec := c.do(http.MethodPut, "/api/v1/user/preferences/getOfficialIcons", `{"value":false}`, nil); rec.Code >= 300 {
		t.Fatalf("preference = %d (%s)", rec.Code, rec.Body.String())
	}
	second := sshAccount(t, c, "Another", "a", "")
	if n := srv.fillIcons(context.Background()); n != 0 {
		t.Errorf("filled with the preference off = %d", n)
	}
	if kept, _ := srv.st.GetAccount(second); kept.Icon != "" {
		t.Errorf("icon = %q", kept.Icon)
	}
}

func TestIconFillerRemembersMisses(t *testing.T) {
	srv, c := iconAdminClient(t)
	site := fakeIconSite(t)
	pointIconsAt(srv, site)
	setChain(t, c, `"walkx"`, 1)
	pointIconsAt(srv, site)
	sshAccount(t, c, "Nothing", "a", "")
	if n := srv.fillIcons(context.Background()); n != 0 {
		t.Fatalf("filled = %d", n)
	}
	srv.iconFill.mu.Lock()
	_, missed := srv.iconFill.missed["nothing"]
	srv.iconFill.mu.Unlock()
	if !missed {
		t.Error("the miss was not recorded")
	}
	rec := c.do(http.MethodPost, "/api/v1/twofaccounts/icons/fill", "", nil)
	if rec.Code != http.StatusOK || jsonBody(t, rec)["pending"] != float64(1) {
		t.Errorf("fill = %d (%s)", rec.Code, rec.Body.String())
	}
	srv.iconFill.mu.Lock()
	_, missed = srv.iconFill.missed["nothing"]
	srv.iconFill.mu.Unlock()
	if missed {
		t.Error("the miss was not forgotten")
	}
}
