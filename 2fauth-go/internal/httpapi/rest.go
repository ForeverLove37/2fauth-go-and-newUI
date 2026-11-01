package httpapi

import (
	"encoding/json"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"

	"2fauth/internal/otp"
	"2fauth/internal/store"
)

// decodeUploadedQR decodes an uploaded QR code image into its text payload.
func decodeUploadedQR(r *http.Request) (string, error) {
	data, _, err := uploadedFile(r, "qrcode")
	if err != nil {
		return "", err
	}
	img, _, err := image.Decode(strings.NewReader(string(data)))
	if err != nil {
		return "", err
	}
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", err
	}
	result, err := qrcode.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		return "", err
	}
	return result.GetText(), nil
}

// ---------------------------------------------------------------------------
// icons
// ---------------------------------------------------------------------------

// iconPacks handles GET /api/v1/icons/packs.
func (s *Server) iconPacks(c *ctx) {
	jsonResponse(c.w, http.StatusOK, map[string]any{"packs": s.icons.LogoPacks()})
}

// iconFetch handles POST /api/v1/icons/default.
func (s *Server) iconFetch(c *ctx) {
	v := newValidator(c)
	service := v.required("service")
	if v.respond() {
		return
	}
	name := s.icons.BuildFromOfficialLogo(service)
	if name == "" {
		jsonMessage(c.w, http.StatusNotFound, "Icon not found for this service")
		return
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"icon": name,
		"url":  s.baseURL() + "/storage/icons/" + name,
	})
}

// iconUpload handles POST /api/v1/icons.
func (s *Server) iconUpload(c *ctx) {
	data, filename, err := uploadedFile(c.r, "icon")
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "file_upload_failed")
		return
	}
	ext := "png"
	if idx := strings.LastIndex(filename, "."); idx >= 0 {
		ext = strings.ToLower(filename[idx+1:])
	}
	if !allowedIconExtension(ext) {
		jsonMessage(c.w, http.StatusUnprocessableEntity, "The icon must be a file of type: jpg, jpeg, png, webp, gif, svg")
		return
	}
	name, err := s.icons.StoreWithExtension(ext, data)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"icon": name,
		"url":  s.baseURL() + "/storage/icons/" + name,
	})
}

func allowedIconExtension(ext string) bool {
	switch ext {
	case "png", "jpg", "jpeg", "webp", "gif", "svg":
		return true
	}
	return false
}

// iconDelete handles DELETE /api/v1/icons/{icon}.
func (s *Server) iconDelete(c *ctx) {
	name := strings.TrimPrefix(c.r.URL.Path, s.baseURL())
	name = name[strings.LastIndex(name, "/")+1:]
	if name == "" {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return
	}
	if err := s.icons.Delete(name); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonMessage(c.w, http.StatusOK, "icon deleted")
}

// ---------------------------------------------------------------------------
// groups
// ---------------------------------------------------------------------------

// groupPayload mirrors GroupResource.
func groupPayload(g *store.Group) map[string]any {
	return map[string]any{
		"id":                 g.ID,
		"name":               g.Name,
		"show_in_chips":      g.ShowInChips,
		"twofaccounts_count": g.AccountsCount,
	}
}

// listGroups handles GET /api/v1/groups.
func (s *Server) listGroups(c *ctx) {
	groups, err := s.st.ListGroups(c.user.ID)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	out := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		out = append(out, groupPayload(g))
	}
	jsonCollection(c.w, http.StatusOK, out)
}

// storeGroup handles POST /api/v1/groups.
func (s *Server) storeGroup(c *ctx) {
	v := newValidator(c)
	name := v.required("name")
	v.maxLen("name", name, 255)
	showInChips := true
	if raw, ok := v.boolean("show_in_chips"); ok {
		showInChips = raw
	}
	if v.respond() {
		return
	}
	g, err := s.st.CreateGroup(c.user.ID, name, showInChips)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonData(c.w, http.StatusCreated, groupPayload(g))
}

// showGroup handles GET /api/v1/groups/{group}.
func (s *Server) showGroup(c *ctx) {
	g, ok := s.lookupGroup(c)
	if !ok {
		return
	}
	jsonData(c.w, http.StatusOK, groupPayload(g))
}

// updateGroup handles PUT/PATCH /api/v1/groups/{group}.
func (s *Server) updateGroup(c *ctx) {
	g, ok := s.lookupGroup(c)
	if !ok {
		return
	}
	v := newValidator(c)
	name := v.required("name")
	v.maxLen("name", name, 255)
	if v.respond() {
		return
	}
	var chips *bool
	if raw, present := v.boolean("show_in_chips"); present {
		chips = &raw
	}
	if err := s.st.UpdateGroup(g.ID, c.user.ID, name, chips); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	updated, err := s.st.GetGroup(g.ID, c.user.ID)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonData(c.w, http.StatusOK, groupPayload(updated))
}

// destroyGroup handles DELETE /api/v1/groups/{group}.
func (s *Server) destroyGroup(c *ctx) {
	g, ok := s.lookupGroup(c)
	if !ok {
		return
	}
	if err := s.st.DeleteGroup(g.ID, c.user.ID); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonNoContent(c.w)
}

// reorderGroups handles POST /api/v1/groups/reorder.
func (s *Server) reorderGroups(c *ctx) {
	raw, ok := c.json()["orderedIds"].([]any)
	if !ok {
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
			"message": "The ordered ids field is required.",
			"errors":  map[string][]string{"orderedIds": {"The ordered ids field is required."}},
		})
		return
	}
	ids := make([]int64, 0, len(raw))
	for _, item := range raw {
		if n, err := strconv.ParseInt(toString(item), 10, 64); err == nil {
			ids = append(ids, n)
		}
	}
	if err := s.st.SaveGroupOrder(c.user.ID, ids); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{"message": "order saved", "orderedIds": ids})
}

// assignAccountsToGroup handles POST /api/v1/groups/{group}/assign.
func (s *Server) assignAccountsToGroup(c *ctx) {
	g, ok := s.lookupGroup(c)
	if !ok {
		return
	}
	raw, ok := c.json()["ids"].([]any)
	if !ok {
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
			"message": "The ids field is required.",
			"errors":  map[string][]string{"ids": {"The ids field is required."}},
		})
		return
	}
	for _, item := range raw {
		id, err := strconv.ParseInt(toString(item), 10, 64)
		if err != nil {
			continue
		}
		a, err := s.st.GetAccount(id)
		if err != nil || a.UserID != c.user.ID {
			jsonMessage(c.w, http.StatusForbidden, "Forbidden")
			return
		}
		if err := s.st.AssignAccountToGroup(id, g.ID, c.user.ID); err != nil {
			jsonMessage(c.w, http.StatusInternalServerError, "server error")
			return
		}
	}
	jsonMessage(c.w, http.StatusOK, "accounts assigned")
}

// groupAccounts handles GET /api/v1/groups/{group}/twofaccounts.
func (s *Server) groupAccounts(c *ctx) {
	g, ok := s.lookupGroup(c)
	if !ok {
		return
	}
	accounts, err := s.st.ListVisibleAccounts(c.user.ID, s.accountScope(), nil)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	out := make([]map[string]any, 0)
	for _, a := range accounts {
		if a.GroupID != nil && *a.GroupID == g.ID {
			out = append(out, s.accountReadPayload(c, a, false, 0))
		}
	}
	jsonCollection(c.w, http.StatusOK, out)
}

// lookupGroup resolves the {group} route parameter.
func (s *Server) lookupGroup(c *ctx) (*store.Group, bool) {
	id, err := strconv.ParseInt(c.param("group"), 10, 64)
	if err != nil {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return nil, false
	}
	g, err := s.st.GetGroup(id, c.user.ID)
	if err != nil {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return nil, false
	}
	return g, true
}

// ---------------------------------------------------------------------------
// settings (admin)
// ---------------------------------------------------------------------------

// listSettings handles GET /api/v1/settings.
func (s *Server) listSettings(c *ctx) {
	out := make([]map[string]any, 0)
	for _, key := range s.settings.Keys() {
		out = append(out, map[string]any{"key": key, "value": s.settings.Get(key)})
	}
	jsonResponse(c.w, http.StatusOK, out)
}

// showSetting handles GET /api/v1/settings/{settingName}.
func (s *Server) showSetting(c *ctx) {
	name := c.param("settingName")
	value := s.settings.Get(name)
	if value == nil && !s.settings.Exists(name) {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{"key": name, "value": value})
}

// storeSetting handles POST /api/v1/settings.
func (s *Server) storeSetting(c *ctx) {
	v := newValidator(c)
	key := v.required("key")
	if !isAlpha(key) {
		v.fail("key", "The key may only contain letters.")
	}
	v.maxLen("key", key, 128)
	value, present := c.json()["value"]
	if !present {
		v.fail("value", "The value field is required.")
	}
	if v.respond() {
		return
	}
	if s.st.OptionExists(key) {
		v.fail("key", "The key has already been taken.")
		if v.respond() {
			return
		}
	}
	if err := s.settings.Set(key, value); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonResponse(c.w, http.StatusCreated, map[string]any{"key": key, "value": value})
}

// updateSetting handles PUT /api/v1/settings/{settingName}.
func (s *Server) updateSetting(c *ctx) {
	name := c.param("settingName")
	value, present := c.json()["value"]
	if !present {
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
			"message": "The value field is required.",
			"errors":  map[string][]string{"value": {"The value field is required."}},
		})
		return
	}
	if name == "restrictList" {
		for _, entry := range strings.Split(toString(value), "\n") {
			entry = strings.TrimSpace(entry)
			if entry != "" && !strings.Contains(entry, "@") {
				jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
					"message": "The value must be a list of valid email addresses.",
					"errors":  map[string][]string{"value": {"The value must be a list of valid email addresses."}},
				})
				return
			}
		}
	}
	if err := s.settings.Set(name, value); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{"key": name, "value": s.settings.Get(name)})
}

// deleteSetting handles DELETE /api/v1/settings/{settingName}.
func (s *Server) deleteSetting(c *ctx) {
	name := c.param("settingName")
	if !s.settings.Exists(name) || s.settings.Get(name) == nil {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return
	}
	if _, isDefault := s.cfg.Settings[name]; isDefault {
		if def, _ := s.cfg.Settings[name].(string); def != "" {
			jsonResponse(c.w, http.StatusBadRequest, map[string]any{
				"message": "bad request",
				"reason":  []string{"Only additional settings added by the administrator can be deleted"},
			})
			return
		}
	}
	if err := s.settings.Delete(name); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonNoContent(c.w)
}

func isAlpha(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// system (admin)
// ---------------------------------------------------------------------------

// systemInfos handles GET /system/infos.
func (s *Server) systemInfos(c *ctx) {
	common := map[string]any{
		"Date":             timeNowRFC1123(),
		"userAgent":        c.r.UserAgent(),
		"Version":          s.cfg.Version,
		"Environment":      s.cfg.Env,
		"Install path":     "/" + s.cfg.Subdirectory,
		"Debug":            boolExport(s.cfg.Debug),
		"Cache driver":     "file",
		"Log channel":      "stderr",
		"Log level":        "notice",
		"DB driver":        s.st.DB.Dialect,
		"PHP version":      "n/a (Go runtime)",
		"Operating system": runtimeOS(),
		"interface":        "go",
		"Auth guard":       s.cfg.AuthenticationGuard,
	}
	if s.cfg.AuthenticationGuard == "reverse-proxy-guard" {
		common["Auth proxy logout url"] = s.cfg.ProxyLogoutURL
		common["Auth proxy header for user"] = s.cfg.AuthProxyHeaderForUser
		common["Auth proxy header for email"] = s.cfg.AuthProxyHeaderForMail
	}
	common["webauthn user verification"] = s.cfg.WebAuthnUserValidation
	common["Trusted proxies"] = orNone(s.cfg.TrustedProxies)
	common["lastRadarScan"] = timeNowFormatted()
	jsonResponse(c.w, http.StatusOK, map[string]any{"common": common})
}

// systemLatestRelease handles GET /system/latestRelease.
func (s *Server) systemLatestRelease(c *ctx) {
	release := s.latestReleaseScan()
	if release != false {
		_ = s.settings.Set("latestRelease", release)
		_ = s.settings.Set("lastRadarScan", timeNowUnix())
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{"newRelease": release})
}

// systemTestEmail handles POST /system/test-email.
func (s *Server) systemTestEmail(c *ctx) {
	if s.cfg.MailMailer == "" || s.cfg.MailMailer == "log" {
		jsonResponse(c.w, 424, map[string]any{
			"message": "Failed to send the test email",
			"details": "no mail transport configured (MAIL_MAILER=log)",
		})
		return
	}
	jsonMessage(c.w, http.StatusOK, "Ok")
}

// systemOptimize handles GET /system/optimize.
func (s *Server) systemOptimize(c *ctx) {
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"config-exit-code": 0,
		"route-exit-code":  0,
		"event-exit-code":  0,
		"view-exit-code":   0,
	})
}

// systemClearCache handles GET /system/clear-cache.
func (s *Server) systemClearCache(c *ctx) {
	jsonResponse(c.w, http.StatusOK, map[string]any{"exit-code": 0})
}

// ---------------------------------------------------------------------------
// feature flags
// ---------------------------------------------------------------------------

var featureNames = []string{"iconPack", "sharing", "allUsersSharingScope", "showNextOtp", "groupChips", "favorites"}

func (s *Server) featureState(name string) string {
	enabled := false
	for _, f := range featureNames {
		if f == name {
			enabled = true
		}
	}
	switch name {
	case "sharing":
		enabled = enabled && s.settings.Bool("enableSharing")
	case "allUsersSharingScope":
		enabled = enabled && s.settings.Bool("enableAllUsersSharingScope")
	}
	if enabled {
		return "enabled"
	}
	return "disabled"
}

// featuresIndex handles GET /api/v1/features.
func (s *Server) featuresIndex(c *ctx) {
	out := make([]map[string]any, 0, len(featureNames))
	for _, name := range featureNames {
		out = append(out, map[string]any{"name": name, "state": s.featureState(name)})
	}
	jsonResponse(c.w, http.StatusOK, out)
}

// featureShow handles GET /api/v1/features/{feature}.
func (s *Server) featureShow(c *ctx) {
	name := c.param("feature")
	for _, f := range featureNames {
		if f == name {
			jsonResponse(c.w, http.StatusOK, map[string]any{"name": name, "state": s.featureState(name)})
			return
		}
	}
	jsonMessage(c.w, http.StatusNotFound, "not found")
}

// ---------------------------------------------------------------------------
// sharing
// ---------------------------------------------------------------------------

// recipientPayload mirrors UserShareRecipientResource.
func (s *Server) recipientPayload(accountID int64, u *store.User, sharedSince map[int64]*time.Time) map[string]any {
	shared := false
	var since any
	if t, ok := sharedSince[u.ID]; ok && t != nil {
		shared = true
		since = t.Format("Mon, Jan 2, 2006 3:04 PM")
	}
	payload := map[string]any{
		"id":             u.ID,
		"name":           u.Name,
		"is_shared_with": shared,
	}
	if shared {
		payload["is_shared_since"] = since
	}
	return payload
}

// shareState builds the index/store payload of the share endpoints.
func (s *Server) shareState(c *ctx, a *store.Account) (map[string]any, bool) {
	if !s.settings.Bool("enableSharing") {
		jsonMessage(c.w, http.StatusForbidden, "Sharing is disabled")
		return nil, false
	}
	if a.UserID != c.user.ID {
		jsonMessage(c.w, http.StatusForbidden, "Forbidden")
		return nil, false
	}
	return nil, true
}

// listShares handles GET /api/v1/twofaccounts/{twofaccount}/shares.
func (s *Server) listShares(c *ctx) {
	a, ok := s.lookupAccount(c, true)
	if !ok {
		return
	}
	if _, ok := s.shareState(c, a); !ok {
		return
	}
	sharedWithAll := s.settings.Bool("enableAllUsersSharingScope") && s.st.IsSharedWithAll(a.ID)
	payload := map[string]any{
		"twofaccount_id":     a.ID,
		"is_shared_with_all": sharedWithAll,
	}
	if !sharedWithAll {
		userIDs, _ := s.st.ShareWithUserIDs(a.ID)
		users, _ := s.st.FindUsersByIDs(userIDs)
		since := s.shareCreationTimes(a.ID)
		items := make([]map[string]any, 0, len(users))
		for _, u := range users {
			items = append(items, s.recipientPayload(a.ID, u, since))
		}
		payload["specific_users"] = items
	}
	jsonResponse(c.w, http.StatusOK, payload)
}

// createShares handles POST /api/v1/twofaccounts/{twofaccount}/shares.
func (s *Server) createShares(c *ctx) {
	a, ok := s.lookupAccount(c, true)
	if !ok {
		return
	}
	if _, ok := s.shareState(c, a); !ok {
		return
	}
	rawIDs, _ := c.json()["user_ids"].([]any)
	if len(rawIDs) == 0 {
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
			"message": "The user ids field is required.",
			"errors":  map[string][]string{"user_ids": {"The user ids field is required."}},
		})
		return
	}
	created := 0
	var shared []*store.User
	for _, raw := range rawIDs {
		id, err := strconv.ParseInt(toString(raw), 10, 64)
		if err != nil {
			continue
		}
		if id == a.UserID {
			jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
				"message": "The user ids and owner must be different.",
				"errors":  map[string][]string{"user_ids": {"The user ids and owner must be different."}},
			})
			return
		}
		u, err := s.st.GetUser(id)
		if err != nil {
			continue
		}
		wasCreated, err := s.st.AddShare(a.ID, u.ID, c.user.ID)
		if err != nil {
			continue
		}
		if wasCreated {
			created++
		}
		shared = append(shared, u)
	}
	since := s.shareCreationTimes(a.ID)
	items := make([]map[string]any, 0, len(shared))
	for _, u := range shared {
		items = append(items, s.recipientPayload(a.ID, u, since))
	}
	status := http.StatusOK
	if created > 0 {
		status = http.StatusCreated
	}
	jsonResponse(c.w, status, map[string]any{
		"twofaccount_id":     a.ID,
		"is_shared_with_all": false,
		"specific_users":     items,
	})
}

// deleteShare handles DELETE /api/v1/twofaccounts/{twofaccount}/shares/{user}.
func (s *Server) deleteShare(c *ctx) {
	a, ok := s.lookupAccount(c, true)
	if !ok {
		return
	}
	if _, ok := s.shareState(c, a); !ok {
		return
	}
	id, err := strconv.ParseInt(c.param("user"), 10, 64)
	if err != nil {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return
	}
	if err := s.st.RemoveShare(a.ID, id); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonNoContent(c.w)
}

// deleteAllShares handles DELETE /api/v1/twofaccounts/{twofaccount}/shares.
func (s *Server) deleteAllShares(c *ctx) {
	a, ok := s.lookupAccount(c, true)
	if !ok {
		return
	}
	if _, ok := s.shareState(c, a); !ok {
		return
	}
	_ = s.st.RemoveAllShares(a.ID)
	if s.settings.Bool("enableAllUsersSharingScope") {
		_ = s.st.RemoveShareWithAll(a.ID)
	}
	jsonNoContent(c.w)
}

// shareWithAll handles POST /api/v1/twofaccounts/{twofaccount}/shares/all.
func (s *Server) shareWithAll(c *ctx) {
	a, ok := s.lookupAccount(c, true)
	if !ok {
		return
	}
	if _, ok := s.shareState(c, a); !ok {
		return
	}
	_ = s.st.RemoveAllShares(a.ID)
	if err := s.st.AddShareWithAll(a.ID, c.user.ID); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonResponse(c.w, http.StatusCreated, map[string]any{
		"twofaccount_id":     a.ID,
		"is_shared_with_all": true,
	})
}

// shareRecipients handles GET /api/v1/twofaccounts/{twofaccount}/recipients.
func (s *Server) shareRecipients(c *ctx) {
	a, ok := s.lookupAccount(c, true)
	if !ok {
		return
	}
	filter := c.query("filter[nameOrEmail]")
	users, err := s.st.SearchRecipients(c.user.ID, filter)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	since := s.shareCreationTimes(a.ID)
	items := make([]map[string]any, 0, len(users))
	for _, u := range users {
		items = append(items, s.recipientPayload(a.ID, u, since))
	}
	jsonCollection(c.w, http.StatusOK, items)
}

// shareCreationTimes maps recipient ids to their share creation time.
func (s *Server) shareCreationTimes(accountID int64) map[int64]*time.Time {
	return s.st.ShareCreationTimes(accountID)
}

var _ = json.Marshal
var _ = otp.TypeTOTP
