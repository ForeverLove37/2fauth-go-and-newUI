package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// deviceToken issues the token of a phone app for a signed-in client.
func deviceToken(t *testing.T, c *client) (string, string) {
	t.Helper()
	rec := c.do(http.MethodPost, "/api/v1/device-tokens", `{"name":"Android test"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("device token = %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct{ AccessToken, AccessTokenID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out.AccessToken, out.AccessTokenID
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func TestDeviceTokenNeverReadsCloudSecrets(t *testing.T) {
	srv := testServer(t)
	alice := registered(t, srv, "alice")
	cloud := sshAccount(t, alice, "Cloud", "alice", "")
	synced := sshAccount(t, alice, "Synced", "alice", `,"phone_sync":true`)
	token, tokenID := deviceToken(t, alice)
	phone := newClient(t, srv)
	h := bearer(token)

	// The web session still sees every secret of its own accounts.
	if body := alice.do("GET", fmt.Sprintf("/api/v1/twofaccounts/%d", cloud), "", nil).Body.String(); !strings.Contains(body, testSecret) {
		t.Fatalf("the owner lost the secret: %s", body)
	}

	for _, path := range []string{
		fmt.Sprintf("/api/v1/twofaccounts/%d", cloud),
		fmt.Sprintf("/api/v1/twofaccounts/%d?withSecret=1", cloud),
		"/api/v1/twofaccounts?withSecret=1",
	} {
		rec := phone.do("GET", path, "", h)
		if rec.Code != 200 {
			t.Fatalf("%s = %d", path, rec.Code)
		}
		var list []map[string]any
		var one struct{ Data any }
		_ = json.Unmarshal(rec.Body.Bytes(), &one)
		raw, _ := json.Marshal(one.Data)
		if err := json.Unmarshal(raw, &list); err != nil {
			var single map[string]any
			_ = json.Unmarshal(raw, &single)
			list = []map[string]any{single}
		}
		for _, account := range list {
			if account["id"] == float64(cloud) && account["secret"] != nil {
				t.Fatalf("%s gave a phone the secret of a cloud-only account", path)
			}
		}
	}
	if rec := phone.do("GET", fmt.Sprintf("/api/v1/twofaccounts/%d/otp", cloud), "", h); rec.Code != 200 {
		t.Fatalf("a phone cannot get the code of a cloud account: %d", rec.Code)
	}

	// Everything a phone does not need is closed.
	for _, call := range [][2]string{
		{"GET", "/api/v1/twofaccounts/export?ids=" + fmt.Sprint(cloud)},
		{"POST", "/api/v1/twofaccounts/export"},
		{"GET", fmt.Sprintf("/api/v1/twofaccounts/%d/qrcode", cloud)},
		{"POST", "/oauth/personal-access-tokens"},
		{"POST", "/api/v1/device-tokens"},
		{"GET", "/oauth/personal-access-tokens"},
		{"GET", "/api/v1/ssh-keys"},
		{"PUT", "/user"},
		{"GET", "/api/v1/users"},
		{"GET", "/api/v1/settings"},
	} {
		if rec := phone.do(call[0], call[1], `{"name":"x","passphrase":"long enough passphrase"}`, h); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d for a device token, want 403", call[0], call[1], rec.Code)
		}
	}

	// It cannot make a cloud account synced, nor rewrite it.
	if rec := phone.do("PATCH", fmt.Sprintf("/api/v1/twofaccounts/%d/phone-sync", cloud), `{"phone_sync":true}`, h); rec.Code != http.StatusForbidden {
		t.Fatalf("a phone synced a cloud account to itself: %d", rec.Code)
	}
	if rec := phone.do("PUT", fmt.Sprintf("/api/v1/twofaccounts/%d", cloud), `{"otp_type":"totp","account":"x","secret":"JBSWY3DPEHPK3PXQ"}`, h); rec.Code != http.StatusForbidden {
		t.Fatalf("a phone rewrote a cloud account: %d", rec.Code)
	}

	// The sync feed holds the synced copy with its secret, and names the others.
	rec := phone.do("GET", "/api/v1/sync", "", h)
	var feed struct {
		Synced   []map[string]any `json:"synced"`
		CloudIDs []int64          `json:"cloud_ids"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &feed)
	if len(feed.Synced) != 1 || feed.Synced[0]["id"] != float64(synced) || feed.Synced[0]["secret"] != testSecret || feed.Synced[0]["revision"] == "" {
		t.Fatalf("sync feed = %s", rec.Body.String())
	}
	if len(feed.CloudIDs) != 1 || feed.CloudIDs[0] != cloud {
		t.Fatalf("cloud ids = %v", feed.CloudIDs)
	}
	before := feed.Synced[0]["revision"]

	// A change made on the web changes the revision the phone compares.
	alice.do("PUT", fmt.Sprintf("/api/v1/twofaccounts/%d", synced), `{"otp_type":"totp","service":"Synced","account":"renamed","secret":"JBSWY3DPEHPK3PXP"}`, nil)
	_ = json.Unmarshal(phone.do("GET", "/api/v1/sync", "", h).Body.Bytes(), &feed)
	if feed.Synced[0]["revision"] == before || feed.Synced[0]["account"] != "renamed" {
		t.Fatalf("the revision did not follow the change: %v", feed.Synced[0])
	}

	// A phone may give its copy up; then the feed lists it as cloud-only.
	if rec := phone.do("PATCH", fmt.Sprintf("/api/v1/twofaccounts/%d/phone-sync", synced), `{"phone_sync":false}`, h); rec.Code != 200 {
		t.Fatalf("unsync = %d", rec.Code)
	}
	_ = json.Unmarshal(phone.do("GET", "/api/v1/sync", "", h).Body.Bytes(), &feed)
	if len(feed.Synced) != 0 || len(feed.CloudIDs) != 2 {
		t.Fatalf("after unsync: %+v", feed)
	}

	// It revokes itself, and only itself.
	if rec := phone.do("DELETE", "/oauth/personal-access-tokens/"+tokenID, "", h); rec.Code != http.StatusNoContent {
		t.Fatalf("self revocation = %d", rec.Code)
	}
	if rec := phone.do("GET", "/api/v1/sync", "", h); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a revoked device token still works: %d", rec.Code)
	}
}

func TestPhoneCreatesSyncedAccounts(t *testing.T) {
	srv := testServer(t)
	alice := registered(t, srv, "alice")
	token, _ := deviceToken(t, alice)
	phone := newClient(t, srv)
	rec := phone.do("POST", "/api/v1/twofaccounts", `{"otp_type":"totp","service":"Phone","account":"me","secret":"JBSWY3DPEHPK3PXP","phone_sync":true}`, bearer(token))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"phone_sync":true`) || !strings.Contains(rec.Body.String(), testSecret) {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
}

func publishAPK(t *testing.T, c *client, code, name string, apk []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("version_code", code)
	_ = form.WriteField("version_name", name)
	_ = form.WriteField("notes", "Faster start")
	part, _ := form.CreateFormFile("apk", "app.apk")
	_, _ = part.Write(apk)
	_ = form.Close()
	req := httptest.NewRequest("POST", "/api/v1/android/releases", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cookie", c.cookie)
	req.Header.Set("X-XSRF-TOKEN", c.xsrf)
	rec := httptest.NewRecorder()
	c.srv.ServeHTTP(rec, req)
	return rec
}

func TestAndroidReleases(t *testing.T) {
	srv := testServer(t)
	admin := registered(t, srv, "admin") // the first account is the administrator
	bob := registered(t, srv, "bob")
	anonymous := newClient(t, srv)

	if rec := anonymous.do("GET", "/api/v1/android/latest", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("latest without release = %d", rec.Code)
	}
	apk := append([]byte("PK\x03\x04"), []byte("....AndroidManifest.xml....classes.dex")...)
	if rec := publishAPK(t, bob, "2", "1.1.0", apk); rec.Code != http.StatusForbidden {
		t.Fatalf("a user published a release: %d", rec.Code)
	}
	if rec := publishAPK(t, admin, "2", "1.1.0", []byte("not a zip")); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a non APK was accepted: %d", rec.Code)
	}
	if rec := publishAPK(t, admin, "2", "1.1.0", apk); rec.Code != http.StatusCreated {
		t.Fatalf("publish = %d %s", rec.Code, rec.Body.String())
	}
	if rec := publishAPK(t, admin, "2", "1.1.1", apk); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an equal version code was accepted: %d", rec.Code)
	}

	rec := anonymous.do("GET", "/api/v1/android/latest", "", nil)
	var latest map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &latest)
	if latest["version_code"] != float64(2) || latest["version_name"] != "1.1.0" || latest["url"] != "/download/android/2fauth-2.apk" || latest["sha256"] == "" {
		t.Fatalf("latest = %s", rec.Body.String())
	}
	for _, path := range []string{"/download/android/latest.apk", "/download/android/2fauth-2.apk"} {
		got := anonymous.do("GET", path, "", nil)
		if got.Code != 200 || !bytes.Equal(got.Body.Bytes(), apk) || got.Header().Get("Content-Type") != "application/vnd.android.package-archive" {
			t.Fatalf("%s = %d %q", path, got.Code, got.Header().Get("Content-Type"))
		}
	}
	// Anything else under the route is the web page fallback, never a file.
	if got := anonymous.do("GET", "/download/android/../../.env", "", nil); strings.Contains(got.Body.String(), "APP_KEY") || got.Header().Get("Content-Type") == "application/vnd.android.package-archive" {
		t.Fatalf("the download route served another file: %q %.120s", got.Header().Get("Content-Type"), got.Body.String())
	}
	if rec := admin.do("DELETE", "/api/v1/android/releases/2", "", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := anonymous.do("GET", "/download/android/latest.apk", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("a deleted release is still served: %d", rec.Code)
	}
}
