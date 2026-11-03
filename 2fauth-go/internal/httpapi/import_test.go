package httpapi

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/scrypt"

	"2fauth/internal/store"
)

func parseOK(t *testing.T, payload, passphrase string) []*store.Account {
	t.Helper()
	accounts, issues, err := parseMigrationPayload(payload, passphrase)
	if err != nil {
		t.Fatalf("parse: %v\npayload: %s", err, payload)
	}
	if len(issues) != 0 {
		t.Fatalf("issues: %+v", issues)
	}
	return accounts
}

func TestImportFormats(t *testing.T) {
	const s = testSecret
	cases := []struct {
		name    string
		payload string
		service string
		account string
		otpType string
		digits  int
		alg     string
		period  int
	}{
		{"otpauth line", "otpauth://totp/GitHub:root?secret=" + s + "&issuer=GitHub", "GitHub", "root", "totp", 6, "sha1", 30},
		{"otpauth json", `[{"uri":"otpauth://totp/GitHub:root?secret=` + s + `"}]`, "GitHub", "root", "totp", 6, "sha1", 30},
		{"plain line", "GitHub:root:" + s, "GitHub", "root", "totp", 6, "sha1", 30},
		{"2fauth", `{"app":"2fauth_v9","schema":1,"data":[{"otp_type":"totp","account":"root","service":"GitHub","secret":"` + s + `","digits":8,"algorithm":"sha256","period":60}]}`, "GitHub", "root", "totp", 8, "sha256", 60},
		{"2fauth wrapped by an earlier build", `{"data":{"app":"2fauth_v9","schema":1,"data":[{"otp_type":"totp","account":"root","service":"GitHub","secret":"` + s + `"}]}}`, "GitHub", "root", "totp", 6, "sha1", 30},
		{"aegis file", `{"version":1,"header":{"slots":null,"params":null},"db":{"version":3,"entries":[{"type":"totp","name":"root","issuer":"GitHub","info":{"secret":"` + s + `","algo":"SHA256","digits":7,"period":45}}]}}`, "GitHub", "root", "totp", 7, "sha256", 45},
		{"aegis steam", `{"header":{},"db":{"entries":[{"type":"steam","name":"me","issuer":"Steam","info":{"secret":"` + s + `","algo":"SHA1","digits":5,"period":30}}]}}`, "Steam", "me", "steamtotp", 5, "sha1", 30},
		{"2fas", `{"schemaVersion":4,"services":[{"name":"GitHub","secret":"` + s + `","otp":{"account":"root","issuer":"GitHub","digits":6,"period":30,"algorithm":"SHA1","tokenType":"TOTP"}}]}`, "GitHub", "root", "totp", 6, "sha1", 30},
		{"2fas uri", `{"services":[{"name":"GitHub","secret":"` + s + `","otpauth":"otpauth://totp/GitHub:root?secret=` + s + `"}]}`, "GitHub", "root", "totp", 6, "sha1", 30},
		{"bitwarden uri", `{"encrypted":false,"items":[{"type":1,"name":"GitHub","login":{"username":"root","totp":"otpauth://totp/GitHub:root?secret=` + s + `"}},{"type":1,"name":"No OTP","login":{"username":"x"}}]}`, "GitHub", "root", "totp", 6, "sha1", 30},
		{"bitwarden secret", `{"items":[{"name":"GitHub","login":{"username":"root","totp":"` + s + `"}}]}`, "GitHub", "root", "totp", 6, "sha1", 30},
		{"bitwarden steam", `{"items":[{"name":"Steam","login":{"username":"me","totp":"steam://` + s + `"}}]}`, "Steam", "me", "steamtotp", 5, "sha1", 30},
		{"bitwarden csv", "folder,favorite,type,name,notes,fields,reprompt,login_uri,login_username,login_password,login_totp\n,,login,GitHub,,,0,https://github.com,root,pw," + s + "\n,,login,NoOTP,,,0,https://x.org,x,pw,\n", "GitHub", "root", "totp", 6, "sha1", 30},
		{"1password csv", "Title,Url,Username,Password,OTPAuth\nGitHub,https://github.com,root,pw,otpauth://totp/GitHub:root?secret=" + s + "\n", "GitHub", "root", "totp", 6, "sha1", 30},
		{"generic csv", "service;account;secret;digits;period;algorithm\nGitHub;root;" + s + ";8;60;SHA512\n", "GitHub", "root", "totp", 8, "sha512", 60},
		{"andotp", `[{"secret":"` + s + `","issuer":"GitHub","label":"root","digits":6,"type":"TOTP","algorithm":"SHA1","period":30,"tags":[]}]`, "GitHub", "root", "totp", 6, "sha1", 30},
		{"andotp label with issuer", `[{"secret":"` + s + `","label":"GitHub:root","digits":6,"type":"TOTP","algorithm":"SHA1","period":30}]`, "GitHub", "root", "totp", 6, "sha1", 30},
		{"raivo", `[{"pinned":"false","iconValue":"","secret":"` + s + `","kind":"TOTP","iconType":"","counter":"0","timer":"30","account":"root","algorithm":"SHA1","digits":"6","issuer":"GitHub"}]`, "GitHub", "root", "totp", 6, "sha1", 30},
		{"freeotp+", `{"tokenOrder":["GitHub:root"],"tokens":[{"algo":"SHA1","counter":0,"digits":6,"issuerExt":"GitHub","label":"root","period":30,"secret":[72,101,108,108,111,33,-34,-83,-66,-17],"type":"TOTP"}]}`, "GitHub", "root", "totp", 6, "sha1", 30},
		{"authenticator pro", `{"Authenticators":[{"Type":2,"Issuer":"GitHub","Username":"root","Secret":"` + s + `","Algorithm":1,"Digits":6,"Period":30,"Counter":0}],"Categories":[]}`, "GitHub", "root", "totp", 6, "sha256", 30},
		{"lastpass", `{"version":3,"accounts":[{"issuerName":"GitHub","userName":"root","secret":"` + s + `","timeStep":30,"digits":6,"algorithm":"SHA1"}]}`, "GitHub", "root", "totp", 6, "sha1", 30},
		{"proton", `{"version":1,"entries":[{"id":"1","content":{"uri":"otpauth://totp/GitHub:root?secret=` + s + `&issuer=GitHub","entry_type":"Totp","name":"GitHub"},"note":""}]}`, "GitHub", "root", "totp", 6, "sha1", 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			accounts := parseOK(t, tc.payload, "")
			if len(accounts) != 1 {
				t.Fatalf("accounts = %d", len(accounts))
			}
			a := accounts[0]
			if a.Service != tc.service || a.Account != tc.account || a.OTPType != tc.otpType || a.Digits != tc.digits || a.Algorithm != tc.alg {
				t.Errorf("account = %+v", a)
			}
			if a.Period == nil || *a.Period != tc.period {
				t.Errorf("period = %v", a.Period)
			}
			if a.Secret == "" {
				t.Error("no secret")
			}
		})
	}
}

func TestImportHOTPEntries(t *testing.T) {
	for name, payload := range map[string]string{
		"aegis":  `{"header":{},"db":{"entries":[{"type":"hotp","name":"a","issuer":"x","info":{"secret":"` + testSecret + `","algo":"SHA1","digits":6,"counter":7}}]}}`,
		"2fas":   `{"services":[{"name":"x","secret":"` + testSecret + `","otp":{"account":"a","tokenType":"HOTP","counter":7}}]}`,
		"andotp": `[{"secret":"` + testSecret + `","issuer":"x","label":"a","digits":6,"type":"HOTP","algorithm":"SHA1","counter":7}]`,
		"auth":   `{"Authenticators":[{"Type":1,"Issuer":"x","Username":"a","Secret":"` + testSecret + `","Algorithm":0,"Digits":6,"Counter":7}]}`,
	} {
		accounts := parseOK(t, payload, "")
		if a := accounts[0]; a.OTPType != "hotp" || a.Counter == nil || *a.Counter != 7 || a.Period != nil {
			t.Errorf("%s: %+v", name, a)
		}
	}
}

// protoField builds a protobuf field for the Google Authenticator fixtures.
func protoBytes(field int, value []byte) []byte {
	return append([]byte{byte(field<<3 | 2), byte(len(value))}, value...)
}

func protoVarint(field, value int) []byte { return []byte{byte(field << 3), byte(value)} }

func googleMigration(t *testing.T, hotp bool) string {
	t.Helper()
	key, _ := base64.StdEncoding.DecodeString("SGVsbG8h3q2+7w==")
	kind := 2
	if hotp {
		kind = 1
	}
	var params []byte
	params = append(params, protoBytes(1, key)...)
	params = append(params, protoBytes(2, []byte("GitHub:root"))...)
	params = append(params, protoBytes(3, []byte("GitHub"))...)
	params = append(params, protoVarint(4, 1)...)
	params = append(params, protoVarint(5, 1)...)
	params = append(params, protoVarint(6, kind)...)
	if hotp {
		params = append(params, protoVarint(7, 5)...)
	}
	return base64.StdEncoding.EncodeToString(protoBytes(1, params))
}

func TestGoogleAuthenticatorMigration(t *testing.T) {
	encoded := googleMigration(t, false)
	// As QR scanners really give it: percent-encoded, including + / and =.
	for name, uri := range map[string]string{
		"percent encoded": "otpauth-migration://offline?data=" + url.QueryEscape(encoded),
		"raw":             "otpauth-migration://offline?data=" + strings.ReplaceAll(encoded, "+", "%2B"),
	} {
		accounts := parseOK(t, uri, "")
		a := accounts[0]
		if a.Service != "GitHub" || a.Account != "root" || a.OTPType != "totp" || a.Period == nil || *a.Period != 30 {
			t.Errorf("%s: %+v", name, a)
		}
	}
	accounts := parseOK(t, "otpauth-migration://offline?data="+url.QueryEscape(googleMigration(t, true)), "")
	if a := accounts[0]; a.OTPType != "hotp" || a.Counter == nil || *a.Counter != 5 {
		t.Errorf("hotp: %+v", a)
	}
	// An export of several QR codes gives several lines.
	multi := "otpauth-migration://offline?data=" + url.QueryEscape(encoded) + "\notpauth-migration://offline?data=" + url.QueryEscape(encoded) + "\n"
	if accounts := parseOK(t, multi, ""); len(accounts) != 2 {
		t.Errorf("multi = %d", len(accounts))
	}
}

func TestMalformedProtobufIsRefused(t *testing.T) {
	for _, raw := range [][]byte{
		{0x0a, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01},
		{0x0a, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
		{0x0a, 0x05, 0x01},
		{0x0f},
	} {
		uri := "otpauth-migration://offline?data=" + url.QueryEscape(base64.StdEncoding.EncodeToString(raw))
		if _, _, err := parseMigrationPayload(uri, ""); err == nil {
			t.Errorf("%x was accepted", raw)
		}
	}
}

func TestUnusableEntriesAreReported(t *testing.T) {
	accounts, issues, err := parseMigrationPayload(`[{"secret":"`+testSecret+`","issuer":"ok","label":"a","type":"TOTP","digits":6},`+
		`{"secret":"not base32!","issuer":"bad","label":"b","type":"TOTP"},`+
		`{"secret":"`+testSecret+`","issuer":"digits","label":"c","type":"TOTP","digits":32},`+
		`{"secret":"`+testSecret+`","issuer":"motp","label":"d","type":"mOTP"}]`, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || len(issues) != 3 {
		t.Fatalf("accounts = %d, issues = %+v", len(accounts), issues)
	}
	if _, _, err := parseMigrationPayload(`[{"secret":"not base32!","issuer":"bad","label":"b","type":"TOTP"}]`, ""); err == nil {
		t.Error("a file with no usable entry was accepted")
	}
	for _, payload := range []string{"", "hello", "{}", `{"foo":1}`, "[1,2]", "[]", `{"encrypted":true,"data":"x"}`} {
		if _, _, err := parseMigrationPayload(payload, ""); err == nil {
			t.Errorf("%q was accepted", payload)
		}
	}
}

// --- encrypted files --------------------------------------------------------

func gcmSeal(t *testing.T, key, plain []byte) (nonce, sealed []byte) {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce = make([]byte, 12)
	_, _ = rand.Read(nonce)
	return nonce, gcm.Seal(nil, nonce, plain, nil)
}

// aegisVault builds an encrypted vault as described by Aegis' vault documentation.
func aegisVault(t *testing.T, passphrase string) string {
	t.Helper()
	db := `{"version":3,"entries":[{"type":"totp","name":"root","issuer":"GitHub","info":{"secret":"` + testSecret + `","algo":"SHA1","digits":6,"period":30}}]}`
	master := make([]byte, 32)
	_, _ = rand.Read(master)
	dbNonce, dbSealed := gcmSeal(t, master, []byte(db))
	salt := make([]byte, 32)
	_, _ = rand.Read(salt)
	derived, err := scrypt.Key([]byte(passphrase), salt, 1<<12, 8, 1, 32)
	if err != nil {
		t.Fatal(err)
	}
	slotNonce, slotSealed := gcmSeal(t, derived, master)
	split := func(sealed []byte) (string, string) {
		return hex.EncodeToString(sealed[:len(sealed)-16]), hex.EncodeToString(sealed[len(sealed)-16:])
	}
	slotKey, slotTag := split(slotSealed)
	dbTag := hex.EncodeToString(dbSealed[len(dbSealed)-16:])
	raw, _ := json.Marshal(map[string]any{
		"version": 1,
		"header": map[string]any{
			"slots": []any{
				map[string]any{"type": 2, "uuid": "biometric"},
				map[string]any{"type": 1, "uuid": "x", "key": slotKey, "key_params": map[string]any{"nonce": hex.EncodeToString(slotNonce), "tag": slotTag},
					"n": 1 << 12, "r": 8, "p": 1, "salt": hex.EncodeToString(salt)},
			},
			"params": map[string]any{"nonce": hex.EncodeToString(dbNonce), "tag": dbTag},
		},
		"db": base64.StdEncoding.EncodeToString(dbSealed[:len(dbSealed)-16]),
	})
	return string(raw)
}

func twoFASBackup(t *testing.T, passphrase string) string {
	t.Helper()
	services := `[{"name":"GitHub","secret":"` + testSecret + `","otp":{"account":"root","issuer":"GitHub","digits":6,"period":30,"algorithm":"SHA1","tokenType":"TOTP"}}]`
	salt := make([]byte, 256)
	_, _ = rand.Read(salt)
	key, err := pbkdf2.Key(sha256.New, passphrase, salt, twoFASIterations, 32)
	if err != nil {
		t.Fatal(err)
	}
	nonce, sealed := gcmSeal(t, key, []byte(services))
	field := base64.StdEncoding.EncodeToString(sealed) + ":" + base64.StdEncoding.EncodeToString(salt) + ":" + base64.StdEncoding.EncodeToString(nonce)
	raw, _ := json.Marshal(map[string]any{"schemaVersion": 4, "services": []any{}, "servicesEncrypted": field})
	return string(raw)
}

func TestEncryptedThirdPartyFiles(t *testing.T) {
	const passphrase = "open sesame phrase"
	for name, payload := range map[string]string{"aegis": aegisVault(t, passphrase), "2fas": twoFASBackup(t, passphrase)} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := parseMigrationPayload(payload, ""); err != errPassphraseRequired {
				t.Errorf("no passphrase = %v", err)
			}
			if _, _, err := parseMigrationPayload(payload, "wrong"); err != errPassphraseWrong {
				t.Errorf("wrong passphrase = %v", err)
			}
			accounts := parseOK(t, payload, passphrase)
			if len(accounts) != 1 || accounts[0].Service != "GitHub" || accounts[0].Account != "root" {
				t.Errorf("accounts = %+v", accounts)
			}
		})
	}
}

func TestEncryptedFilesCannotChooseAnyCost(t *testing.T) {
	sealed, err := sealExport([]byte(`{"app":"x","data":[]}`), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	sealed["kdf"].(map[string]any)["memory_kib"] = 1 << 30
	raw, _ := json.Marshal(sealed)
	if _, _, err := parseMigrationPayload(string(raw), "correct horse battery"); err != errInvalidPayload {
		t.Errorf("huge argon2 memory = %v", err)
	}
	var vault map[string]any
	_ = json.Unmarshal([]byte(aegisVault(t, "x")), &vault)
	slots := vault["header"].(map[string]any)["slots"].([]any)
	slots[1].(map[string]any)["n"] = 1 << 30
	raw, _ = json.Marshal(vault)
	if _, _, err := parseMigrationPayload(string(raw), "x"); err != errInvalidPayload {
		t.Errorf("huge scrypt N = %v", err)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	srv := testServer(t)
	owner := registered(t, srv, "owner")

	icon := jsonBody(t, upload(t, owner, "icon", "a.png", onePixelPNG))["icon"].(string)
	body := fmt.Sprintf(`{"otp_type":"totp","service":"GitHub","account":"root","secret":%q,"digits":8,"algorithm":"sha256","period":60,"icon":%q}`, testSecret, icon)
	rec := owner.do(http.MethodPost, "/api/v1/twofaccounts", body, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d (%s)", rec.Code, rec.Body.String())
	}
	id := int64(jsonBody(t, rec)["data"].(map[string]any)["id"].(float64))
	hotp := owner.do(http.MethodPost, "/api/v1/twofaccounts", `{"otp_type":"hotp","service":"Bank","account":"me","secret":"`+testSecret+`","counter":12}`, nil)
	hotpID := int64(jsonBody(t, hotp)["data"].(map[string]any)["id"].(float64))
	ids := fmt.Sprintf("%d,%d", id, hotpID)

	// The file follows the 2FAuth schema, with no wrapper around it.
	plain := owner.do(http.MethodGet, "/api/v1/twofaccounts/export?ids="+ids, "", nil)
	doc := jsonBody(t, plain)
	if doc["app"] == nil || doc["schema"] != float64(1) {
		t.Fatalf("export = %s", plain.Body.String())
	}

	check := func(t *testing.T, c *client, payload, passphrase string) {
		t.Helper()
		raw, _ := json.Marshal(map[string]string{"payload": payload, "passphrase": passphrase})
		rec := c.do(http.MethodPost, "/api/v1/twofaccounts/import", string(raw), nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("import = %d (%s)", rec.Code, rec.Body.String())
		}
		result := jsonBody(t, rec)
		if result["imported"] != float64(2) || result["invalid"] != float64(0) {
			t.Fatalf("import = %v", result)
		}
		list := jsonBody(t, c.do(http.MethodGet, "/api/v1/twofaccounts?withSecret=1", "", nil))["data"].([]any)
		if len(list) != 2 {
			t.Fatalf("list = %v", list)
		}
		var totp, counter map[string]any
		for _, item := range list {
			m := item.(map[string]any)
			if m["otp_type"] == "totp" {
				totp = m
			} else {
				counter = m
			}
		}
		if totp["service"] != "GitHub" || totp["digits"] != float64(8) || totp["algorithm"] != "sha256" || totp["period"] != float64(60) || !strings.HasPrefix(totp["secret"].(string), testSecret) {
			t.Errorf("totp = %v", totp)
		}
		if totp["icon"] != icon {
			t.Errorf("the icon was not restored: %v", totp["icon"])
		}
		if counter["counter"] != float64(12) {
			t.Errorf("hotp = %v", counter)
		}
		// Importing the same file again creates nothing.
		rec = c.do(http.MethodPost, "/api/v1/twofaccounts/import", string(raw), nil)
		if again := jsonBody(t, rec); again["imported"] != float64(0) || again["duplicates"] != float64(2) {
			t.Errorf("second import = %v", again)
		}
	}

	t.Run("plain", func(t *testing.T) {
		check(t, registered(t, srv, "plainuser"), plain.Body.String(), "")
	})

	t.Run("otpauth", func(t *testing.T) {
		rec := owner.do(http.MethodGet, "/api/v1/twofaccounts/export?otpauth=1&ids="+ids, "", nil)
		if !strings.Contains(rec.Body.String(), "otpauth://") {
			t.Fatalf("otpauth export = %s", rec.Body.String())
		}
	})

	t.Run("encrypted", func(t *testing.T) {
		if rec := owner.do(http.MethodPost, "/api/v1/twofaccounts/export", fmt.Sprintf(`{"ids":%q,"passphrase":"short"}`, ids), nil); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("short passphrase = %d", rec.Code)
		}
		const passphrase = "correct horse battery staple"
		rec := owner.do(http.MethodPost, "/api/v1/twofaccounts/export", fmt.Sprintf(`{"ids":%q,"passphrase":%q}`, ids, passphrase), nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("export = %d (%s)", rec.Code, rec.Body.String())
		}
		file := rec.Body.String()
		for _, leak := range []string{testSecret, "GitHub", "root", passphrase} {
			if strings.Contains(file, leak) {
				t.Errorf("%q is readable in the encrypted file", leak)
			}
		}
		user := registered(t, srv, "encuser")
		raw, _ := json.Marshal(map[string]string{"payload": file})
		rec = user.do(http.MethodPost, "/api/v1/twofaccounts/import", string(raw), nil)
		if rec.Code != http.StatusUnprocessableEntity || jsonBody(t, rec)["needs_passphrase"] != true {
			t.Errorf("no passphrase = %d (%s)", rec.Code, rec.Body.String())
		}
		raw, _ = json.Marshal(map[string]string{"payload": file, "passphrase": "not the right one"})
		rec = user.do(http.MethodPost, "/api/v1/twofaccounts/import", string(raw), nil)
		if rec.Code != http.StatusUnprocessableEntity || jsonBody(t, rec)["needs_passphrase"] != false {
			t.Errorf("wrong passphrase = %d (%s)", rec.Code, rec.Body.String())
		}
		check(t, user, file, passphrase)

		// A modified file is detected rather than imported.
		var doc map[string]any
		_ = json.Unmarshal([]byte(file), &doc)
		sealed, _ := base64.StdEncoding.DecodeString(doc["ciphertext"].(string))
		sealed[0] ^= 1
		doc["ciphertext"] = base64.StdEncoding.EncodeToString(sealed)
		tampered, _ := json.Marshal(doc)
		if _, _, err := parseMigrationPayload(string(tampered), passphrase); err != errPassphraseWrong {
			t.Errorf("tampered file = %v", err)
		}
	})
}

func TestMigrationPreviewAcceptsAPassphrase(t *testing.T) {
	srv := testServer(t)
	c := registered(t, srv, "alice")
	raw, _ := json.Marshal(map[string]string{"payload": aegisVault(t, "my passphrase"), "passphrase": "my passphrase"})
	rec := c.do(http.MethodPost, "/api/v1/twofaccounts/migration", string(raw), nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":-2`) {
		t.Fatalf("migration = %d (%s)", rec.Code, rec.Body.String())
	}
	createAccount(t, c)
	raw, _ = json.Marshal(map[string]string{"payload": "otpauth://totp/GitHub:root?secret=" + testSecret + "&issuer=GitHub"})
	rec = c.do(http.MethodPost, "/api/v1/twofaccounts/migration", string(raw), nil)
	if !strings.Contains(rec.Body.String(), `"id":-1`) {
		t.Errorf("duplicate = %s", rec.Body.String())
	}
}

func TestImportStillRefusesHostileIcons(t *testing.T) {
	srv := testServer(t)
	c := registered(t, srv, "alice")
	evil := base64.StdEncoding.EncodeToString([]byte(`<!DOCTYPE html><script>alert(1)</script>`))
	payload := `{"app":"2fauth_v9","schema":1,"data":[{"otp_type":"totp","account":"a","service":"s","secret":"` + testSecret + `","icon":"../../x","icon_file":"` + evil + `"}]}`
	raw, _ := json.Marshal(map[string]string{"payload": payload})
	if rec := c.do(http.MethodPost, "/api/v1/twofaccounts/import", string(raw), nil); rec.Code != http.StatusOK {
		t.Fatalf("import = %d (%s)", rec.Code, rec.Body.String())
	}
	list := jsonBody(t, c.do(http.MethodGet, "/api/v1/twofaccounts", "", nil))["data"].([]any)
	if icon := list[0].(map[string]any)["icon"]; icon != nil {
		t.Errorf("icon = %v", icon)
	}
}

func TestImportsAreThrottled(t *testing.T) {
	srv := testServer(t)
	srv.cfg.ThrottleAPIImport = 2
	c := registered(t, srv, "alice")
	raw, _ := json.Marshal(map[string]string{"payload": "otpauth://totp/a:b?secret=" + testSecret})
	var code int
	for i := 0; i < 4; i++ {
		code = c.do(http.MethodPost, "/api/v1/twofaccounts/migration", string(raw), nil).Code
	}
	if code != http.StatusTooManyRequests {
		t.Errorf("after the limit = %d", code)
	}
}
