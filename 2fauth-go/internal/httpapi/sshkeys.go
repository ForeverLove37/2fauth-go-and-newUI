package httpapi

import (
	"crypto/rsa"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"

	"2fauth/internal/store"
)

// minRSABits is the smallest RSA key accepted.
const minRSABits = 2048

// maxSSHKeyName bounds the label of a key.
const maxSSHKeyName = 100

var errSSHKeyFormat = errors.New("The public key is not valid: paste one line of a .pub file, such as ~/.ssh/id_ed25519.pub.")

// parseSSHPublicKey reads an authorized_keys line and returns its normal form. Key
// options are refused rather than ignored: "from=..." would look like a restriction
// that nothing applies.
func parseSSHPublicKey(text string) (ssh.PublicKey, string, error) {
	key, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(text)))
	if err != nil {
		return nil, "", errSSHKeyFormat
	}
	if len(strings.TrimSpace(string(rest))) > 0 {
		return nil, "", errors.New("Paste a single public key.")
	}
	if len(options) > 0 {
		return nil, "", errors.New("Key options are not supported: paste the key without them.")
	}
	switch key.Type() {
	case ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521, ssh.KeyAlgoSKED25519, ssh.KeyAlgoSKECDSA256:
	case ssh.KeyAlgoRSA:
		crypto, ok := key.(ssh.CryptoPublicKey)
		if !ok {
			return nil, "", errSSHKeyFormat
		}
		public, ok := crypto.CryptoPublicKey().(*rsa.PublicKey)
		if !ok || public.N.BitLen() < minRSABits {
			return nil, "", errors.New("RSA keys must have at least 2048 bits.")
		}
	default:
		return nil, "", errors.New("This key type is not supported: use Ed25519, ECDSA or RSA.")
	}
	return key, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), nil
}

func sshKeyPayload(key *store.SSHKey) map[string]any {
	return map[string]any{
		"id":           key.ID,
		"name":         key.Name,
		"fingerprint":  key.Fingerprint,
		"key_type":     strings.SplitN(key.PublicKey, " ", 2)[0],
		"all_accounts": key.AllAccounts,
		"account_ids":  key.AccountIDs,
		"group_ids":    key.GroupIDs,
		"last_used_at": key.LastUsedAt,
		"last_used_ip": key.LastUsedIP,
		"created_at":   key.CreatedAt,
	}
}

// refuseTokenAccess keeps key management to signed-in browsers. A leaked API token
// must not be able to turn itself into lasting command line access.
func refuseTokenAccess(c *ctx) bool {
	if c.viaToken {
		jsonMessage(c.w, http.StatusForbidden, "SSH keys cannot be managed with an API token")
		return true
	}
	return false
}

// listSSHKeys handles GET /api/v1/ssh-keys.
func (s *Server) listSSHKeys(c *ctx) {
	if refuseTokenAccess(c) {
		return
	}
	keys, err := s.st.ListSSHKeys(c.user.ID)
	if err != nil {
		s.srvError(c, err)
		return
	}
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		out = append(out, sshKeyPayload(key))
	}
	payload := map[string]any{"enabled": s.cfg.SSHListenAddr != "", "keys": out}
	if s.cfg.SSHListenAddr != "" {
		if _, port, err := net.SplitHostPort(s.cfg.SSHListenAddr); err == nil {
			payload["port"] = port
		}
		s.sshMu.RLock()
		payload["host_fingerprint"] = s.sshHostFingerprint
		s.sshMu.RUnlock()
	}
	jsonResponse(c.w, http.StatusOK, payload)
}

// sshKeyScopeFromInput reads and checks the scope of a key: only what the user can
// reach right now may be named.
func (s *Server) sshKeyScopeFromInput(c *ctx, v *validator) (all bool, accounts, groups []int64) {
	all = c.input("all_accounts") == "1" || strings.EqualFold(c.input("all_accounts"), "true")
	if all {
		return true, nil, nil
	}
	visible, err := s.st.ListVisibleAccounts(c.user.ID, s.accountScope(), nil)
	if err != nil {
		v.fail("account_ids", "The accounts could not be checked.")
		return false, nil, nil
	}
	allowed := map[int64]bool{}
	for _, account := range visible {
		allowed[account.ID] = true
	}
	for _, id := range parseScopeIDs(c.input("account_ids")) {
		if !allowed[id] {
			v.fail("account_ids", "One of the accounts is not available.")
			return false, nil, nil
		}
		accounts = append(accounts, id)
	}
	for _, id := range parseScopeIDs(c.input("group_ids")) {
		if _, err := s.st.GetGroup(id, c.user.ID); err != nil {
			v.fail("group_ids", "One of the groups is not available.")
			return false, nil, nil
		}
		groups = append(groups, id)
	}
	if len(accounts) == 0 && len(groups) == 0 {
		v.fail("account_ids", "Choose what this key can read, or allow all accounts.")
	}
	return false, accounts, groups
}

// parseScopeIDs reads a comma separated list of ids, dropping duplicates.
func parseScopeIDs(raw string) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, part := range strings.Split(raw, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// createSSHKey handles POST /api/v1/ssh-keys. A key is lasting access to codes, so it
// takes the password, like every other change of that weight.
func (s *Server) createSSHKey(c *ctx) {
	if refuseTokenAccess(c) {
		return
	}
	v := newValidator(c)
	name := v.required("name")
	v.maxLen("name", name, maxSSHKeyName)
	text := v.required("public_key")

	var (
		key       ssh.PublicKey
		canonical string
		err       error
	)
	if text != "" {
		if key, canonical, err = parseSSHPublicKey(text); err != nil {
			v.fail("public_key", err.Error())
		}
	}
	all, accounts, groups := s.sshKeyScopeFromInput(c, v)
	if v.respond() {
		return
	}
	if !s.confirmIdentity(c, c.input("password")) {
		return
	}

	fingerprint := ssh.FingerprintSHA256(key)
	if _, err := s.st.FindSSHKeyByFingerprint(fingerprint); err == nil {
		v.fail("public_key", "This key is already in use.")
		v.respond()
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		s.srvError(c, err)
		return
	}

	created, err := s.st.CreateSSHKey(&store.SSHKey{
		UserID: c.user.ID, Name: name, PublicKey: canonical, Fingerprint: fingerprint,
		AllAccounts: all, AccountIDs: accounts, GroupIDs: groups,
	})
	if err != nil {
		s.srvError(c, err)
		return
	}
	jsonResponse(c.w, http.StatusCreated, sshKeyPayload(created))
}

// sshKeyFromPath resolves the {key} parameter among the keys of the user.
func (s *Server) sshKeyFromPath(c *ctx) (*store.SSHKey, bool) {
	id, err := strconv.ParseInt(c.param("key"), 10, 64)
	if err == nil {
		if key, err := s.st.GetSSHKey(id); err == nil && key.UserID == c.user.ID {
			return key, true
		}
	}
	jsonMessage(c.w, http.StatusNotFound, "not found")
	return nil, false
}

// updateSSHKey handles PUT /api/v1/ssh-keys/{key}. The key itself cannot change.
func (s *Server) updateSSHKey(c *ctx) {
	if refuseTokenAccess(c) {
		return
	}
	key, ok := s.sshKeyFromPath(c)
	if !ok {
		return
	}
	v := newValidator(c)
	name := v.required("name")
	v.maxLen("name", name, maxSSHKeyName)
	all, accounts, groups := s.sshKeyScopeFromInput(c, v)
	if v.respond() {
		return
	}
	if !s.confirmIdentity(c, c.input("password")) {
		return
	}
	key.Name, key.AllAccounts, key.AccountIDs, key.GroupIDs = name, all, accounts, groups
	if err := s.st.UpdateSSHKey(key); err != nil {
		s.srvError(c, err)
		return
	}
	fresh, err := s.st.GetSSHKey(key.ID)
	if err != nil {
		s.srvError(c, err)
		return
	}
	jsonResponse(c.w, http.StatusOK, sshKeyPayload(fresh))
}

// deleteSSHKey handles DELETE /api/v1/ssh-keys/{key}. Taking access away needs no
// confirmation, and even an API token may do it.
func (s *Server) deleteSSHKey(c *ctx) {
	key, ok := s.sshKeyFromPath(c)
	if !ok {
		return
	}
	if _, err := s.st.DeleteSSHKey(key.ID, c.user.ID); err != nil {
		s.srvError(c, err)
		return
	}
	c.w.WriteHeader(http.StatusNoContent)
}
