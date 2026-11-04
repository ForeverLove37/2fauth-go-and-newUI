package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"2fauth/internal/config"
	"2fauth/internal/crypt"
	"2fauth/internal/db"
)

// testDatabases returns the databases to run against: always SQLite, and the servers
// named by TEST_PGSQL_DSN and TEST_MYSQL_DSN when they are set.
func testDatabases(t *testing.T) map[string]func(t *testing.T) *Store {
	t.Helper()
	open := func(dialect, dsn string) func(t *testing.T) *Store {
		return func(t *testing.T) *Store {
			t.Helper()
			conn, err := db.Open(dialect, dsn)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			t.Cleanup(func() { conn.Close() })
			for _, table := range []string{"ssh_key_accounts", "ssh_key_groups", "ssh_keys", "icon_uploads", "user_sessions", "oauth_identities", "oauth_access_tokens", "webauthn_credentials", "auth_logs", "otp_logs",
				"twofaccount_user_favorites", "twofaccount_user_orders", "twofaccount_shares", "twofaccounts", `"groups"`, "options", "password_resets", "users", "migrations"} {
				_, _ = conn.Exec(`DROP TABLE IF EXISTS ` + table)
			}
			if err := conn.Migrate(); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			if err := conn.Migrate(); err != nil {
				t.Fatalf("second migrate: %v", err)
			}
			st := New(conn, config.Load(t.TempDir()))
			enc, err := crypt.New(crypt.GenerateKey())
			if err != nil {
				t.Fatal(err)
			}
			st.SetCipher(enc)
			return st
		}
	}
	out := map[string]func(t *testing.T) *Store{
		"sqlite": func(t *testing.T) *Store {
			t.Helper()
			return open("sqlite", filepath.Join(t.TempDir(), "t.sqlite"))(t)
		},
	}
	if dsn := os.Getenv("TEST_PGSQL_DSN"); dsn != "" {
		out["pgsql"] = open("pgsql", dsn)
	}
	if dsn := os.Getenv("TEST_MYSQL_DSN"); dsn != "" {
		out["mysql"] = open("mysql", dsn)
	}
	return out
}

func TestStoreAcrossDatabases(t *testing.T) {
	for name, open := range testDatabases(t) {
		t.Run(name, func(t *testing.T) {
			st := open(t)

			// users, and the rule of the first administrator
			first, err := st.CreateUser("first", "First@Example.org", "hash")
			if err != nil {
				t.Fatalf("CreateUser: %v", err)
			}
			if !first.IsAdmin || first.Email != "first@example.org" || first.ID == 0 {
				t.Fatalf("first = %+v", first)
			}
			second, err := st.CreateUser("second", "second@example.org", "hash")
			if err != nil {
				t.Fatalf("CreateUser: %v", err)
			}
			if second.IsAdmin {
				t.Fatal("second user is an administrator")
			}
			if _, err := st.CreateUser("first", "other@example.org", "hash"); err == nil {
				t.Fatal("duplicate name accepted")
			}
			if u, err := st.GetUserByEmail("FIRST@example.org"); err != nil || u.ID != first.ID {
				t.Fatalf("GetUserByEmail = %v, %v", u, err)
			}
			if err := st.MarkEmailVerified(first.ID); err != nil {
				t.Fatal(err)
			}
			if u, _ := st.GetUser(first.ID); u.EmailVerifiedAt == nil {
				t.Fatal("not verified")
			}
			if changed, err := st.UpdateUsernameEmail(first.ID, "first", "new@example.org"); err != nil || !changed {
				t.Fatalf("UpdateUsernameEmail = %v, %v", changed, err)
			}
			if u, _ := st.GetUser(first.ID); u.EmailVerifiedAt != nil {
				t.Fatal("a new address stayed verified")
			}
			if err := st.SavePreferences(first.ID, map[string]any{"lang": "en"}); err != nil {
				t.Fatal(err)
			}
			if err := st.SetAdmin(second.ID, true); err != nil {
				t.Fatal(err)
			}
			if n, _ := st.CountAdmins(); n != 2 {
				t.Fatalf("admins = %d", n)
			}

			// options: a reserved word as column name
			if err := st.SetOption("someKey", "one"); err != nil {
				t.Fatalf("SetOption: %v", err)
			}
			if err := st.SetOption("someKey", "two"); err != nil {
				t.Fatalf("SetOption again: %v", err)
			}
			options, err := st.AllOptions()
			if err != nil || options["someKey"] != "two" {
				t.Fatalf("AllOptions = %v, %v", options, err)
			}
			if !st.OptionExists("someKey") {
				t.Fatal("OptionExists")
			}
			if err := st.DeleteOption("someKey"); err != nil {
				t.Fatal(err)
			}

			// groups: a reserved word as table name
			group, err := st.CreateGroup(first.ID, "Work", true)
			if err != nil {
				t.Fatalf("CreateGroup: %v", err)
			}
			if group.ID == 0 || !group.ShowInChips {
				t.Fatalf("group = %+v", group)
			}
			chips := false
			if err := st.UpdateGroup(group.ID, first.ID, "Job", &chips); err != nil {
				t.Fatal(err)
			}
			if groups, err := st.ListGroups(first.ID); err != nil || len(groups) != 1 || groups[0].Name != "Job" || groups[0].ShowInChips {
				t.Fatalf("ListGroups = %v, %v", groups, err)
			}

			// accounts: the secret stays sealed on disk
			period := 30
			account, err := st.CreateAccount(&Account{UserID: first.ID, Service: "GitHub", Account: "root", OTPType: "totp", Secret: "JBSWY3DPEHPK3PXP", Algorithm: "sha1", Digits: 6, Period: &period})
			if err != nil {
				t.Fatalf("CreateAccount: %v", err)
			}
			if account.ID == 0 || account.Secret != "JBSWY3DPEHPK3PXP" {
				t.Fatalf("account = %+v", account)
			}
			var raw string
			if err := st.DB.QueryRow(`SELECT secret FROM twofaccounts WHERE id = ?`, account.ID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(raw, "JBSWY3DP") {
				t.Fatalf("clear text secret: %q", raw)
			}
			if err := st.AssignAccountToGroup(account.ID, group.ID, first.ID); err != nil {
				t.Fatal(err)
			}
			if err := st.ToggleFavorite(account.ID, first.ID, true); err != nil {
				t.Fatal(err)
			}
			if !st.IsFavorite(account.ID, first.ID) {
				t.Fatal("favorite")
			}
			if _, err := st.SaveOrderForUser(first.ID, []int64{account.ID}, VisibleScope{}); err != nil {
				t.Fatal(err)
			}
			counter := int64(3)
			hotp, err := st.CreateAccount(&Account{UserID: first.ID, Account: "h", OTPType: "hotp", Secret: "JBSWY3DPEHPK3PXP", Algorithm: "sha1", Digits: 6, Counter: &counter})
			if err != nil {
				t.Fatal(err)
			}
			if ok, err := st.AdvanceCounter(hotp.ID, 3, 4); err != nil || !ok {
				t.Fatalf("AdvanceCounter = %v, %v", ok, err)
			}
			if ok, err := st.AdvanceCounter(hotp.ID, 3, 4); err != nil || ok {
				t.Fatalf("stale AdvanceCounter = %v, %v", ok, err)
			}

			// shares
			if created, err := st.AddShare(account.ID, second.ID, first.ID); err != nil || !created {
				t.Fatalf("AddShare = %v, %v", created, err)
			}
			if !st.IsSharedWith(account.ID, second.ID) {
				t.Fatal("IsSharedWith")
			}
			visible, err := st.ListVisibleAccounts(second.ID, VisibleScope{EnableSharing: true}, nil)
			if err != nil || len(visible) != 1 {
				t.Fatalf("ListVisibleAccounts = %v, %v", visible, err)
			}
			if err := st.AddShareWithAll(account.ID, first.ID); err != nil {
				t.Fatal(err)
			}
			if n, err := st.CountSharedAccounts(second.ID, VisibleScope{EnableSharing: true, AllUsersSharingScope: true}); err != nil || n != 1 {
				t.Fatalf("CountSharedAccounts = %d, %v", n, err)
			}
			if err := st.TransferOwnership(account.ID, second.ID); err != nil {
				t.Fatal(err)
			}
			if st.IsSharedWithAll(account.ID) || st.IsSharedWith(account.ID, second.ID) {
				t.Fatal("shares survived the transfer")
			}

			// logs
			if err := st.CreateAuthLog(first.ID, "10.0.0.1", "agent", "web-guard", "password", true); err != nil {
				t.Fatalf("CreateAuthLog: %v", err)
			}
			if !st.SeenDevice(first.ID, "10.0.0.1", "agent") || st.SeenDevice(first.ID, "10.0.0.2", "agent") {
				t.Fatal("SeenDevice")
			}
			st.CloseAuthLog(first.ID)
			logs, err := st.ListAuthLogs(first.ID, 1, 10)
			if err != nil || len(logs) != 1 || logs[0].LogoutAt == nil || !logs[0].LoginSuccessful {
				t.Fatalf("ListAuthLogs = %+v, %v", logs, err)
			}
			if err := st.CreateOtpLog(&OtpLog{RequesterName: "a", RequesterEmail: "a", OwnerName: "a", OwnerEmail: "a", TwoFAccountID: hotp.ID, IPAddress: "::1", OTPType: "hotp", GeneratedAt: time.Now()}); err != nil {
				t.Fatalf("CreateOtpLog: %v", err)
			}
			if entries, err := st.ListOtpLogs(hotp.ID, 1, 10); err != nil || len(entries) != 1 {
				t.Fatalf("ListOtpLogs = %v, %v", entries, err)
			}
			if err := st.PurgeAuthLogs(1); err != nil {
				t.Fatal(err)
			}
			if err := st.PurgeOtpLogs(1); err != nil {
				t.Fatal(err)
			}

			// tokens
			token, plain, err := st.CreateToken(first.ID, "t", nil)
			if err != nil {
				t.Fatalf("CreateToken: %v", err)
			}
			if u, err := st.UserFromToken(plain); err != nil || u.ID != first.ID {
				t.Fatalf("UserFromToken = %v, %v", u, err)
			}
			if tokens, err := st.ListTokens(first.ID); err != nil || len(tokens) != 1 || tokens[0].ID != token.ID {
				t.Fatalf("ListTokens = %v, %v", tokens, err)
			}

			// sessions
			sid, err := st.CreateSession(first.ID, "10.0.0.1", "agent")
			if err != nil {
				t.Fatalf("CreateSession: %v", err)
			}
			keep, _ := st.CreateSession(first.ID, "10.0.0.1", "agent")
			if s, err := st.GetSession(sid); err != nil || s.UserID != first.ID {
				t.Fatalf("GetSession = %v, %v", s, err)
			}
			st.TouchSession(sid, time.Now().Unix()+1)
			if err := st.DeleteUserSessions(first.ID, keep); err != nil {
				t.Fatal(err)
			}
			if _, err := st.GetSession(sid); err != ErrNotFound {
				t.Fatalf("deleted session = %v", err)
			}
			if _, err := st.GetSession(keep); err != nil {
				t.Fatalf("kept session = %v", err)
			}
			if err := st.PurgeSessions(time.Now().Unix()+100, 0); err != nil {
				t.Fatal(err)
			}

			// password resets
			if err := st.SavePasswordReset("new@example.org", "h1"); err != nil {
				t.Fatal(err)
			}
			if err := st.SavePasswordReset("new@example.org", "h2"); err != nil {
				t.Fatal(err)
			}
			if h, age, err := st.FindPasswordReset("new@example.org"); err != nil || h != "h2" || age > time.Minute {
				t.Fatalf("FindPasswordReset = %q, %v, %v", h, age, err)
			}

			// icon ownership
			if err := st.AddIconUpload("a.png", first.ID); err != nil {
				t.Fatal(err)
			}
			if !st.IconUploadedBy("a.png", first.ID) || st.CountIconUploads(first.ID) != 1 {
				t.Fatal("icon upload")
			}
			if st.IconUsedByOthers("a.png", first.ID) {
				t.Fatal("IconUsedByOthers")
			}

			// deleting a user removes what it owns
			if err := st.DeleteUser(second.ID); err != nil {
				t.Fatalf("DeleteUser: %v", err)
			}
			if _, err := st.GetAccount(account.ID); err != ErrNotFound {
				t.Fatalf("account of a deleted user = %v", err)
			}
			if err := st.DeleteUser(first.ID); err != nil {
				t.Fatalf("DeleteUser: %v", err)
			}
			if _, err := st.UserFromToken(plain); err == nil {
				t.Fatal("a token outlived its user")
			}
		})
	}
}

func TestSSHKeys(t *testing.T) {
	for name, open := range testDatabases(t) {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			owner, err := st.CreateUser("owner", "owner@example.org", "x")
			if err != nil {
				t.Fatal(err)
			}
			account, err := st.CreateAccount(&Account{UserID: owner.ID, Service: "GitHub", Account: "a", OTPType: "totp", Secret: "JBSWY3DPEHPK3PXP", Algorithm: "sha1", Digits: 6})
			if err != nil {
				t.Fatal(err)
			}
			group, err := st.CreateGroup(owner.ID, "work", true)
			if err != nil {
				t.Fatal(err)
			}
			key, err := st.CreateSSHKey(&SSHKey{UserID: owner.ID, Name: "laptop", PublicKey: "ssh-ed25519 AAAA", Fingerprint: "SHA256:one", AccountIDs: []int64{account.ID}, GroupIDs: []int64{group.ID}})
			if err != nil {
				t.Fatal(err)
			}
			if key.AllAccounts || len(key.AccountIDs) != 1 || len(key.GroupIDs) != 1 || key.LastUsedAt != nil {
				t.Errorf("created = %+v", key)
			}
			if _, err := st.CreateSSHKey(&SSHKey{UserID: owner.ID, Name: "same", PublicKey: "ssh-ed25519 AAAA", Fingerprint: "SHA256:one"}); err == nil {
				t.Error("a fingerprint was registered twice")
			}
			found, err := st.FindSSHKeyByFingerprint("SHA256:one")
			if err != nil || found.ID != key.ID {
				t.Fatalf("find = %+v (%v)", found, err)
			}
			if _, err := st.FindSSHKeyByFingerprint("SHA256:none"); err != ErrNotFound {
				t.Errorf("unknown key = %v", err)
			}
			if err := st.TouchSSHKey(key.ID, "192.0.2.1"); err != nil {
				t.Fatal(err)
			}
			key.Name, key.AllAccounts, key.AccountIDs, key.GroupIDs = "renamed", true, nil, nil
			if err := st.UpdateSSHKey(key); err != nil {
				t.Fatal(err)
			}
			fresh, err := st.GetSSHKey(key.ID)
			if err != nil || fresh.Name != "renamed" || !fresh.AllAccounts || len(fresh.AccountIDs) != 0 || fresh.LastUsedAt == nil || fresh.LastUsedIP != "192.0.2.1" {
				t.Errorf("updated = %+v (%v)", fresh, err)
			}

			// A deleted account or group leaves no scope behind.
			fresh.AllAccounts, fresh.AccountIDs, fresh.GroupIDs = false, []int64{account.ID}, []int64{group.ID}
			if err := st.UpdateSSHKey(fresh); err != nil {
				t.Fatal(err)
			}
			if err := st.DeleteAccount(account.ID); err != nil {
				t.Fatal(err)
			}
			if err := st.DeleteGroup(group.ID, owner.ID); err != nil {
				t.Fatal(err)
			}
			scoped, err := st.GetSSHKey(key.ID)
			if err != nil || len(scoped.AccountIDs) != 0 || len(scoped.GroupIDs) != 0 {
				t.Errorf("scope after deletion = %+v (%v)", scoped, err)
			}

			// Another user cannot remove it, and deleting the owner does.
			if removed, err := st.DeleteSSHKey(key.ID, owner.ID+1); err != nil || removed {
				t.Errorf("removed by another user = %v (%v)", removed, err)
			}
			if err := st.DeleteUser(owner.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := st.GetSSHKey(key.ID); err != ErrNotFound {
				t.Errorf("key of a deleted user = %v", err)
			}
		})
	}
}

func TestMissingIcons(t *testing.T) {
	for name, open := range testDatabases(t) {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			owner, err := st.CreateUser("owner", "owner@example.org", "x")
			if err != nil {
				t.Fatal(err)
			}
			create := func(service, icon string) *Account {
				a, err := st.CreateAccount(&Account{UserID: owner.ID, Service: service, Account: "a", Icon: icon, OTPType: "totp", Secret: "JBSWY3DPEHPK3PXP", Algorithm: "sha1", Digits: 6})
				if err != nil {
					t.Fatal(err)
				}
				return a
			}
			missing := create("GitHub", "")
			create("GitLab", "own.png")
			create("", "")
			removed := create("Gitea", "")
			if err := st.SetIconOptOut(removed.ID, true); err != nil {
				t.Fatal(err)
			}
			list, err := st.AccountsMissingIcon()
			if err != nil || len(list) != 1 || list[0].ID != missing.ID {
				t.Fatalf("missing = %+v (%v)", list, err)
			}
			if err := st.SetMissingIcon(missing.ID, "found.svg"); err != nil {
				t.Fatal(err)
			}
			if err := st.SetMissingIcon(missing.ID, "later.svg"); err != nil {
				t.Fatal(err)
			}
			if a, _ := st.GetAccount(missing.ID); a.Icon != "found.svg" {
				t.Errorf("icon = %q", a.Icon)
			}
		})
	}
}
