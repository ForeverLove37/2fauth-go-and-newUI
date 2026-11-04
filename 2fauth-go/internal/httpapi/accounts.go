package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"2fauth/internal/mail"
	"2fauth/internal/otp"
	"2fauth/internal/services"
	"2fauth/internal/store"
)

// accountScope returns the sharing scope currently in force.
func (s *Server) accountScope() store.VisibleScope {
	return store.VisibleScope{
		EnableSharing:        s.settings.Bool("enableSharing"),
		AllUsersSharingScope: s.settings.Bool("enableAllUsersSharingScope"),
	}
}

// accountReadPayload mirrors TwoFAccountReadResource + TwoFAccountStoreResource.
func (s *Server) accountReadPayload(c *ctx, a *store.Account, withOtp bool, at int64) map[string]any {
	scope := s.accountScope()
	owned := c.user != nil && a.UserID == c.user.ID
	// Callers only pass accounts the user may see, whichever share made it visible.
	borrowed := c.user != nil && !owned
	sharedWithAll := owned && scope.EnableSharing && scope.AllUsersSharingScope && s.st.IsSharedWithAll(a.ID)
	sharedWithSome := owned && scope.EnableSharing && !sharedWithAll && s.hasExplicitShares(a.ID)

	groupID, _ := s.st.AccountGroupID(a, c.user.ID)

	payload := map[string]any{
		"id":          a.ID,
		"group_id":    nullableGroupID(groupID),
		"is_favorite": s.st.IsFavorite(a.ID, c.user.ID),
	}
	if borrowed {
		payload["is_borrowed"] = true
		if owner, err := s.st.GetUser(a.UserID); err == nil {
			payload["borrowed_by"] = owner.Name
		}
	}
	if sharedWithSome {
		payload["is_shared"] = true
	}
	if sharedWithAll {
		payload["is_shared_with_all"] = true
	}

	for k, v := range s.accountStorePayload(a, owned || c.user == nil, owned) {
		payload[k] = v
	}

	if withOtp && a.OTPType != otp.TypeHOTP {
		if otpPayload, err := s.generateTOTPPayload(a, at); err == nil {
			payload["otp"] = otpPayload
		}
	}
	return payload
}

// accountStorePayload mirrors TwoFAccountStoreResource::toArray().
func (s *Server) accountStorePayload(a *store.Account, withSecret, canViewSecret bool) map[string]any {
	payload := map[string]any{
		"otp_type":  a.OTPType,
		"account":   nullableString(a.Account),
		"service":   nullableString(a.Service),
		"icon":      s.iconIfExists(a.Icon),
		"digits":    a.Digits,
		"algorithm": a.Algorithm,
		"period":    nullablePeriod(a.Period),
		"counter":   nullableCounter(a.Counter),
	}
	if withSecret && canViewSecret {
		payload["secret"] = a.Secret
	}
	return payload
}

// accountExportPayload mirrors TwoFAccountExportResource (non otpauth mode).
func (s *Server) accountExportPayload(a *store.Account) map[string]any {
	return map[string]any{
		"otp_type":   a.OTPType,
		"account":    nullableString(a.Account),
		"service":    nullableString(a.Service),
		"icon":       nullableString(a.Icon),
		"icon_mime":  s.iconMimeType(a.Icon),
		"icon_file":  s.iconBase64(a.Icon),
		"secret":     a.Secret,
		"digits":     a.Digits,
		"algorithm":  a.Algorithm,
		"period":     nullablePeriod(a.Period),
		"counter":    nullableCounter(a.Counter),
		"legacy_uri": nullableString(a.LegacyURI),
	}
}

func nullableGroupID(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullablePeriod(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableCounter(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// hasExplicitShares reports whether the account is shared with at least one user.
func (s *Server) hasExplicitShares(accountID int64) bool {
	ids, err := s.st.ShareWithUserIDs(accountID)
	return err == nil && len(ids) > 0
}

// ---------------------------------------------------------------------------
// listing / CRUD
// ---------------------------------------------------------------------------

// listAccounts handles GET /api/v1/twofaccounts.
func (s *Server) listAccounts(c *ctx) {
	v := newValidator(c)
	var ids []int64
	if c.query("ids") != "" {
		parsed, ok := parseIDList(c.query("ids"))
		if !ok {
			v.fail("ids", "The ids format is invalid.")
		}
		ids = parsed
	}
	if v.respond() {
		return
	}

	accounts, err := s.st.ListVisibleAccounts(c.user.ID, s.accountScope(), ids)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}

	withOtp := parseBoolInput(c.query("withOtp"))
	withSecret := false
	if c.query("withSecret") != "" {
		withSecret = parseBoolInput(c.query("withSecret"))
	}
	at := int64(0)
	if raw := c.query("at"); raw != "" {
		at, _ = strconv.ParseInt(raw, 10, 64)
	}

	out := make([]map[string]any, 0, len(accounts))
	for _, a := range accounts {
		payload := s.accountReadPayload(c, a, withOtp, boundedAt(a, at))
		if withOtp && a.OTPType != otp.TypeHOTP {
			s.logOtpGeneration(c, a, a.OTPType, nil)
		}
		if withSecret && a.UserID != c.user.ID {
			delete(payload, "secret")
		}
		if !withSecret {
			delete(payload, "secret")
		}
		out = append(out, payload)
	}
	jsonCollection(c.w, http.StatusOK, out)
}

// showAccount handles GET /api/v1/twofaccounts/{twofaccount}.
func (s *Server) showAccount(c *ctx) {
	a, ok := s.lookupAccount(c, false)
	if !ok {
		return
	}
	withOtp := parseBoolInput(c.query("withOtp"))
	withSecret := true
	if c.query("withSecret") != "" {
		withSecret = parseBoolInput(c.query("withSecret"))
	}
	at := int64(0)
	if raw := c.query("at"); raw != "" {
		at, _ = strconv.ParseInt(raw, 10, 64)
	}
	payload := s.accountReadPayload(c, a, withOtp, boundedAt(a, at))
	if withOtp && a.OTPType != otp.TypeHOTP {
		s.logOtpGeneration(c, a, a.OTPType, nil)
	}
	if !withSecret || a.UserID != c.user.ID {
		delete(payload, "secret")
	}
	jsonData(c.w, http.StatusOK, payload)
}

// boundedAt returns the requested OTP time when it stays within one window of now,
// and 0 (the current time) otherwise. An arbitrary timestamp would let anyone who can
// read a code, such as the recipient of a share, compute the codes of the future and
// keep using them after losing access.
func boundedAt(a *store.Account, at int64) int64 {
	if at <= 0 {
		return 0
	}
	window := int64(otp.DefaultPeriod)
	if a.Period != nil && *a.Period > 0 {
		window = int64(*a.Period)
	}
	if now := time.Now().Unix(); at < now-window || at > now+window {
		return 0
	}
	return at
}

// storeAccount handles POST /api/v1/twofaccounts.
func (s *Server) storeAccount(c *ctx) {
	v := newValidator(c)

	var account *store.Account
	if v.has("uri") {
		uri := v.required("uri")
		if !strings.HasPrefix(strings.ToLower(uri), "otpauth://t") && !strings.HasPrefix(strings.ToLower(uri), "otpauth://h") {
			v.fail("uri", "The uri format is invalid.")
		}
		if v.respond() {
			return
		}
		filled, err := s.accountFromURI(uri, v.optional("custom_otp") == otp.TypeSteam)
		if err == nil {
			err = validateStored(filled)
		}
		if err != nil {
			msg := "The uri format is invalid."
			if err == otp.ErrMissingLabel {
				msg = "The label field is required."
			}
			jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
				"message": msg,
				"errors":  map[string][]string{"uri": {msg}},
			})
			return
		}
		account = filled
	} else {
		account = s.accountFromInput(v)
		if v.respond() {
			return
		}
	}

	account.UserID = c.user.ID
	created, err := s.st.CreateAccount(account)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}

	if groupID := v.optional("group_id"); groupID != "" {
		if gid, err := strconv.ParseInt(groupID, 10, 64); err == nil && gid > 0 {
			if err := s.st.AssignAccountToGroup(created.ID, gid, c.user.ID); err == nil {
				created, _ = s.st.GetAccount(created.ID)
			}
		}
	}

	s.wakeIconFiller()
	jsonData(c.w, http.StatusCreated, s.accountReadPayload(c, created, false, 0))
}

// updateAccount handles PUT/PATCH /api/v1/twofaccounts/{twofaccount}.
func (s *Server) updateAccount(c *ctx) {
	a, ok := s.lookupAccount(c, true)
	if !ok {
		return
	}

	v := newValidator(c)
	updated := s.accountFromInput(v)
	if v.respond() {
		return
	}

	updated.ID = a.ID
	updated.UserID = a.UserID
	updated.Icon = v.optional("icon")
	if updated.Icon == "" && a.Icon != "" && !v.has("icon") {
		updated.Icon = a.Icon
	}
	updated.LegacyURI = a.LegacyURI
	if err := s.st.UpdateAccount(updated); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	// An icon removed on purpose must not come back on its own.
	if v.has("icon") {
		_ = s.st.SetIconOptOut(a.ID, updated.Icon == "")
	}
	s.wakeIconFiller()

	if v.has("group_id") {
		groupRaw := v.optional("group_id")
		currentGroup, _ := s.st.AccountGroupID(a, c.user.ID)
		var target int64
		if groupRaw != "" {
			target, _ = strconv.ParseInt(groupRaw, 10, 64)
		}
		if target != currentGroup {
			if target == 0 {
				_ = s.st.WithdrawAccounts([]int64{a.ID}, c.user.ID)
			} else if err := s.st.AssignAccountToGroup(a.ID, target, c.user.ID); err != nil {
				_ = s.st.WithdrawAccounts([]int64{a.ID}, c.user.ID)
			}
		}
	}

	refreshed, err := s.st.GetAccount(a.ID)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonData(c.w, http.StatusOK, s.accountReadPayload(c, refreshed, false, 0))
}

// destroyAccount handles DELETE /api/v1/twofaccounts/{twofaccount}.
func (s *Server) destroyAccount(c *ctx) {
	a, ok := s.lookupAccount(c, true)
	if !ok {
		return
	}
	if err := s.st.DeleteAccount(a.ID); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonNoContent(c.w)
}

// batchDestroyAccounts handles DELETE /api/v1/twofaccounts?ids=.
func (s *Server) batchDestroyAccounts(c *ctx) {
	ids, ok := s.requireIDList(c)
	if !ok {
		return
	}
	for _, id := range ids {
		a, err := s.st.GetAccount(id)
		if err != nil || a.UserID != c.user.ID {
			jsonMessage(c.w, http.StatusForbidden, "Forbidden")
			return
		}
	}
	if err := s.st.DeleteAccounts(ids); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonNoContent(c.w)
}

// withdrawAccounts handles PATCH /api/v1/twofaccounts/withdraw?ids=.
func (s *Server) withdrawAccounts(c *ctx) {
	ids, ok := s.requireIDList(c)
	if !ok {
		return
	}
	for _, id := range ids {
		a, err := s.st.GetAccount(id)
		if err != nil || a.UserID != c.user.ID {
			jsonMessage(c.w, http.StatusForbidden, "Forbidden")
			return
		}
	}
	if err := s.st.WithdrawAccounts(ids, c.user.ID); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonMessage(c.w, http.StatusOK, "accounts withdrawn")
}

// reorderAccounts handles POST /api/v1/twofaccounts/reorder.
func (s *Server) reorderAccounts(c *ctx) {
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
	for _, id := range ids {
		a, err := s.st.GetAccount(id)
		if err != nil {
			continue
		}
		if !s.canSee(a, c.user.ID) {
			jsonMessage(c.w, http.StatusForbidden, "Forbidden")
			return
		}
	}
	final, err := s.st.SaveOrderForUser(c.user.ID, ids, s.accountScope())
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"message":    "order saved",
		"orderedIds": final,
	})
}

// favoriteAccount handles PATCH /api/v1/twofaccounts/{twofaccount}/favorite.
func (s *Server) favoriteAccount(c *ctx) {
	a, ok := s.lookupAccount(c, false)
	if !ok {
		return
	}
	v := newValidator(c)
	isFavorite, present := v.boolean("is_favorite")
	if !present {
		v.fail("is_favorite", "The is favorite field is required.")
	}
	if v.respond() {
		return
	}
	if err := s.st.ToggleFavorite(a.ID, c.user.ID, isFavorite); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonData(c.w, http.StatusOK, s.accountReadPayload(c, a, false, 0))
}

// countAccounts handles GET /api/v1/twofaccounts/count.
func (s *Server) countAccounts(c *ctx) {
	owned, _ := s.st.CountOwnedAccounts(c.user.ID)
	shared, _ := s.st.CountSharedAccounts(c.user.ID, s.accountScope())
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"owned":  owned,
		"shared": shared,
		"total":  owned + shared,
	})
}

// transferOwnership handles PATCH /api/v1/twofaccounts/{twofaccount}/owner.
func (s *Server) transferOwnership(c *ctx) {
	a, ok := s.lookupAccount(c, true)
	if !ok {
		return
	}
	v := newValidator(c)
	password := v.optional("confirm_password")
	newOwnerID := v.requireInteger("new_owner_id")
	if v.respond() {
		return
	}
	if !s.confirmIdentity(c, password) {
		return
	}
	newOwner, err := s.st.GetUser(int64(newOwnerID))
	if err != nil {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return
	}
	if newOwner.ID == c.user.ID {
		jsonResponse(c.w, http.StatusOK, map[string]any{"twofaccount_id": a.ID, "owner_id": newOwner.ID})
		return
	}
	if err := s.st.TransferOwnership(a.ID, newOwner.ID); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	s.notifyAccountEvent(mail.KindTransferred, "notifyOnOwnershipTransfer", c.user, newOwner, a)
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"twofaccount_id": a.ID,
		"owner_id":       newOwner.ID,
	})
}

// canSee reports whether the user owns the account or currently receives it through
// a share. Disabling sharing removes the access of every recipient at once.
func (s *Server) canSee(a *store.Account, userID int64) bool {
	if a.UserID == userID {
		return true
	}
	scope := s.accountScope()
	if !scope.EnableSharing {
		return false
	}
	return s.st.IsSharedWith(a.ID, userID) || (scope.AllUsersSharingScope && s.st.IsSharedWithAll(a.ID))
}

// lookupAccount resolves the {twofaccount} parameter and checks access.
func (s *Server) lookupAccount(c *ctx, requireOwnership bool) (*store.Account, bool) {
	raw := c.param("twofaccount")
	if raw == "" {
		raw = c.param("id")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return nil, false
	}
	a, err := s.st.GetAccount(id)
	if err != nil {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return nil, false
	}
	if requireOwnership {
		if a.UserID != c.user.ID {
			jsonMessage(c.w, http.StatusForbidden, "Forbidden")
			return nil, false
		}
		return a, true
	}
	if !s.canSee(a, c.user.ID) {
		jsonMessage(c.w, http.StatusForbidden, "Forbidden")
		return nil, false
	}
	return a, true
}

// requireIDList validates the ids query parameter.
func (s *Server) requireIDList(c *ctx) ([]int64, bool) {
	return s.requireIDs(c, c.query("ids"))
}

func (s *Server) requireIDs(c *ctx, raw string) ([]int64, bool) {
	ids, ok := parseIDList(raw)
	if !ok {
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
			"message": "The ids field is required.",
			"errors":  map[string][]string{"ids": {"The ids field is required."}},
		})
		return nil, false
	}
	if len(ids) > 99 {
		jsonResponse(c.w, http.StatusBadRequest, map[string]any{
			"message": "bad request",
			"reason":  []string{"you cannot request more than 99 items at once"},
		})
		return nil, false
	}
	return ids, true
}

// ---------------------------------------------------------------------------
// OTP
// ---------------------------------------------------------------------------

// otpByID handles GET /api/v1/twofaccounts/{id}/otp.
func (s *Server) otpByID(c *ctx) {
	a, ok := s.lookupAccount(c, false)
	if !ok {
		return
	}
	shared := a.UserID != c.user.ID
	if shared && !s.settings.Bool("enableSharing") {
		jsonMessage(c.w, http.StatusForbidden, "Sharing is disabled")
		return
	}
	s.writeOTP(c, a, true)
}

// otpFromParameters handles POST /api/v1/twofaccounts/otp.
func (s *Server) otpFromParameters(c *ctx) {
	v := newValidator(c)
	var account *store.Account

	if v.has("uri") {
		uri := v.required("uri")
		if v.respond() {
			return
		}
		filled, err := s.accountFromURI(uri, v.optional("custom_otp") == otp.TypeSteam)
		if err == nil {
			err = validateStored(filled)
		}
		if err != nil {
			jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
				"message": "The uri format is invalid.",
				"errors":  map[string][]string{"uri": {"The uri format is invalid."}},
			})
			return
		}
		account = filled
	} else {
		account = s.accountFromInput(v)
		if v.respond() {
			return
		}
	}
	s.writeOTP(c, account, false)
}

// writeOTP generates and returns the OTP payload.
func (s *Server) writeOTP(c *ctx, a *store.Account, persist bool) {
	acc := otp.Account{
		Type:      a.OTPType,
		Secret:    a.Secret,
		Algorithm: a.Algorithm,
		Digits:    a.Digits,
		Period:    a.Period,
		Counter:   a.Counter,
		Service:   a.Service,
		Account:   a.Account,
	}

	payload, err := s.generateOTPPayload(acc, a, persist)
	if err != nil {
		jsonMessage(c.w, http.StatusBadRequest, "Invalid secret for this account")
		return
	}
	if persist && a != nil && a.ID > 0 {
		var counter *int64
		if acc.Type == otp.TypeHOTP && a.Counter != nil {
			counter = a.Counter
		}
		s.logOtpGeneration(c, a, acc.Type, counter)
	}
	jsonResponse(c.w, http.StatusOK, payload)
}

// generateOTPPayload builds the OtpDto payload.
func (s *Server) generateOTPPayload(acc otp.Account, a *store.Account, persist bool) (map[string]any, error) {
	now := time.Now().Unix()

	if acc.Type == otp.TypeHOTP {
		code, next, err := s.nextHOTP(&acc, a, now, persist)
		if err != nil {
			return nil, err
		}
		payload := map[string]any{
			"password": code,
			"otp_type": acc.Type,
		}
		if a != nil && a.ID > 0 {
			payload["otp_type"] = a.OTPType
		}
		payload["counter"] = next
		return payload, nil
	}

	if acc.Type == otp.TypeSteam {
		code, err := otp.Code(acc, now)
		if err != nil {
			return nil, err
		}
		nextCode, _ := otp.Code(acc, now+int64(otp.ExpiresIn(acc, now))+2)
		period := otp.Period(acc)
		return map[string]any{
			"password":      code,
			"otp_type":      acc.Type,
			"generated_at":  now,
			"period":        period,
			"next_password": nextCode,
		}, nil
	}

	code, err := otp.Code(acc, now)
	if err != nil {
		return nil, err
	}
	expiresIn := otp.ExpiresIn(acc, now)
	nextCode, _ := otp.Code(acc, now+int64(expiresIn)+2)
	return map[string]any{
		"password":      code,
		"otp_type":      acc.Type,
		"generated_at":  now,
		"period":        otp.Period(acc),
		"next_password": nextCode,
	}, nil
}

// nextHOTP computes an HOTP code and moves the counter forward. If another request
// used the same counter first, the code is computed again from the new value: a
// counter value, and so a code, is only ever handed out once.
func (s *Server) nextHOTP(acc *otp.Account, a *store.Account, now int64, persist bool) (string, int64, error) {
	for attempt := 0; ; attempt++ {
		code, err := otp.Code(*acc, now)
		if err != nil {
			return "", 0, err
		}
		current := int64(0)
		if acc.Counter != nil {
			current = *acc.Counter
		}
		if !persist || a == nil || a.ID <= 0 {
			return code, current + 1, nil
		}
		advanced, err := s.st.AdvanceCounter(a.ID, current, current+1)
		if err != nil {
			return "", 0, err
		}
		if advanced {
			return code, current + 1, nil
		}
		fresh, err := s.st.GetAccount(a.ID)
		if err != nil || attempt >= 8 {
			return "", 0, otp.ErrInvalidParameter
		}
		acc.Counter = fresh.Counter
		a.Counter = fresh.Counter
	}
}

// generateTOTPPayload is the read-resource flavour of the OTP payload.
func (s *Server) generateTOTPPayload(a *store.Account, at int64) (map[string]any, error) {
	acc := otp.Account{
		Type:      a.OTPType,
		Secret:    a.Secret,
		Algorithm: a.Algorithm,
		Digits:    a.Digits,
		Period:    a.Period,
		Counter:   a.Counter,
	}
	if at <= 0 {
		at = time.Now().Unix()
	}
	code, err := otp.Code(acc, at)
	if err != nil {
		return nil, err
	}
	nextCode, _ := otp.Code(acc, at+int64(otp.ExpiresIn(acc, at))+2)
	return map[string]any{
		"password":      code,
		"generated_at":  at,
		"next_password": nextCode,
	}, nil
}

// ---------------------------------------------------------------------------
// input parsing
// ---------------------------------------------------------------------------

// accountFromURI builds an unsaved account from an otpauth:// uri.
func (s *Server) accountFromURI(uri string, steam bool) (*store.Account, error) {
	parsed, err := otp.ParseURI(uri)
	if err != nil {
		return nil, err
	}
	if steam {
		parsed.Type = otp.TypeSteam
		parsed.Service = "Steam"
		parsed.Digits = 5
		parsed.Algorithm = otp.SHA1
		period := otp.DefaultPeriod
		parsed.Period = &period
	} else if parsed.Type == otp.TypeTOTP && strings.EqualFold(parsed.Service, "steam") {
		parsed.Type = otp.TypeSteam
		parsed.Service = "Steam"
		parsed.Digits = 5
		parsed.Algorithm = otp.SHA1
		period := otp.DefaultPeriod
		parsed.Period = &period
	}
	return s.accountFromOtp(parsed), nil
}

// accountFromOtp converts an otp.Account to a store.Account.
func (s *Server) accountFromOtp(parsed otp.Account) *store.Account {
	acc := &store.Account{
		Service:   parsed.Service,
		Account:   parsed.Account,
		OTPType:   parsed.Type,
		Secret:    otp.PadSecret(parsed.Secret),
		Algorithm: strings.ToLower(parsed.Algorithm),
		Digits:    parsed.Digits,
		Period:    parsed.Period,
		Counter:   parsed.Counter,
	}
	if acc.Algorithm == "" {
		acc.Algorithm = otp.DefaultAlgorithm
	}
	if acc.Digits == 0 {
		acc.Digits = otp.DefaultDigits
	}
	if acc.OTPType == otp.TypeHOTP {
		acc.Period = nil
		if acc.Counter == nil {
			c := int64(otp.DefaultCounter)
			acc.Counter = &c
		}
	} else {
		acc.Counter = nil
		if acc.Period == nil {
			p := otp.DefaultPeriod
			acc.Period = &p
		}
	}
	if acc.OTPType == otp.TypeSteam {
		acc.Service = "Steam"
		acc.Digits = 5
		acc.Algorithm = otp.SHA1
		p := otp.DefaultPeriod
		acc.Period = &p
	}
	return acc
}

// validateStored checks an account built from a URI or an import before it is kept:
// a value no code can be computed from must be refused now, not fail on every listing.
func validateStored(a *store.Account) error {
	return otp.Validate(otp.Account{
		Type:      a.OTPType,
		Secret:    a.Secret,
		Algorithm: a.Algorithm,
		Digits:    a.Digits,
		Period:    a.Period,
		Counter:   a.Counter,
		Service:   a.Service,
		Account:   a.Account,
	})
}

// accountFromInput validates and builds an account from the advanced form fields.
func (s *Server) accountFromInput(v *validator) *store.Account {
	otpType := v.required("otp_type")
	v.in("otp_type", otpType, otp.TypeTOTP, otp.TypeHOTP, otp.TypeSteam)
	account := v.required("account")
	if strings.Contains(account, ":") {
		v.fail("account", "The account format is invalid.")
	}
	service := v.optional("service")
	if strings.Contains(service, ":") {
		v.fail("service", "The service format is invalid.")
	}
	v.maxLen("account", account, otp.MaxLabelLength)
	v.maxLen("service", service, otp.MaxLabelLength)
	secret := v.required("secret")
	secret = otp.SanitizeSecret(secret)
	if secret != "" {
		if _, err := otp.DecodeBase32(secret); err != nil {
			v.fail("secret", "The secret is not a valid base32 encoded string.")
		}
	}

	algorithm := strings.ToLower(v.optional("algorithm"))
	if algorithm == "" {
		algorithm = otp.DefaultAlgorithm
	}
	v.in("algorithm", algorithm, otp.SHA1, otp.SHA256, otp.SHA512, otp.MD5)

	digits, hasDigits := v.integer("digits")
	if !hasDigits || digits == 0 {
		digits = otp.DefaultDigits
	}
	v.between("digits", digits, 5, 10)

	acc := &store.Account{
		Service:   service,
		Account:   account,
		OTPType:   otpType,
		Secret:    otp.PadSecret(secret),
		Algorithm: algorithm,
		Digits:    digits,
		Icon:      v.optional("icon"),
	}
	if acc.Icon != "" && !services.ValidIconName(acc.Icon) {
		v.fail("icon", "The icon name is invalid.")
	}

	if otpType == otp.TypeHOTP {
		counter, hasCounter := v.integer("counter")
		if !hasCounter {
			counter = otp.DefaultCounter
		}
		if counter < 0 {
			v.fail("counter", "The counter must be at least 0.")
		}
		c := int64(counter)
		acc.Counter = &c
	} else {
		period, hasPeriod := v.integer("period")
		if !hasPeriod || period <= 0 {
			period = otp.DefaultPeriod
		}
		v.between("period", period, 1, otp.MaxPeriod)
		acc.Period = &period
	}

	if otpType == otp.TypeSteam || strings.EqualFold(service, "steam") {
		acc.OTPType = otp.TypeSteam
		acc.Service = "Steam"
		acc.Digits = 5
		acc.Algorithm = otp.SHA1
		p := otp.DefaultPeriod
		acc.Period = &p
		acc.Counter = nil
	}
	return acc
}

// ---------------------------------------------------------------------------
// export / preview / qrcode
// ---------------------------------------------------------------------------

// exportAccounts handles GET /api/v1/twofaccounts/export.
func (s *Server) exportAccounts(c *ctx) {
	accounts, ok := s.accountsToExport(c, c.query("ids"))
	if !ok {
		return
	}
	if parseBoolInput(c.query("otpauth")) {
		jsonCollection(c.w, http.StatusOK, otpauthItems(accounts))
		return
	}
	jsonResponse(c.w, http.StatusOK, s.exportDocument(accounts))
}

// accountsToExport resolves the requested ids. An export contains the secrets, so
// only the owner may export an account: a share only grants generating its codes.
func (s *Server) accountsToExport(c *ctx, rawIDs string) ([]*store.Account, bool) {
	ids, ok := s.requireIDs(c, rawIDs)
	if !ok {
		return nil, false
	}
	accounts := make([]*store.Account, 0, len(ids))
	for _, id := range ids {
		a, err := s.st.GetAccount(id)
		if err != nil {
			continue
		}
		if a.UserID != c.user.ID {
			jsonMessage(c.w, http.StatusForbidden, "Forbidden")
			return nil, false
		}
		accounts = append(accounts, a)
	}
	return accounts, true
}

func otpauthItems(accounts []*store.Account) []map[string]any {
	items := make([]map[string]any, 0, len(accounts))
	for _, a := range accounts {
		items = append(items, map[string]any{"uri": otp.BuildURI(otp.Account{
			Type:      a.OTPType,
			Secret:    a.Secret,
			Algorithm: a.Algorithm,
			Digits:    a.Digits,
			Period:    a.Period,
			Counter:   a.Counter,
			Service:   a.Service,
			Account:   a.Account,
		})})
	}
	return items
}

// exportDocument builds the export file, following the 2FAuth export schema.
func (s *Server) exportDocument(accounts []*store.Account) map[string]any {
	items := make([]map[string]any, 0, len(accounts))
	for _, a := range accounts {
		items = append(items, s.accountExportPayload(a))
	}
	return map[string]any{
		"app":      "2fauth_v" + s.cfg.Version,
		"schema":   1,
		"datetime": time.Now().UTC().Format("2006-01-02T15:04:05.000000Z"),
		"data":     items,
	}
}

// previewAccount handles POST /api/v1/twofaccounts/preview.
func (s *Server) previewAccount(c *ctx) {
	v := newValidator(c)
	uri := v.required("uri")
	if v.respond() {
		return
	}
	account, err := s.accountFromURI(uri, v.optional("custom_otp") == otp.TypeSteam)
	if err == nil {
		err = validateStored(account)
	}
	if err != nil {
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
			"message": "The uri format is invalid.",
			"errors":  map[string][]string{"uri": {"The uri format is invalid."}},
		})
		return
	}
	jsonData(c.w, http.StatusOK, s.accountStorePayload(account, true, true))
}

// accountQrCode handles GET /api/v1/twofaccounts/{twofaccount}/qrcode.
func (s *Server) accountQrCode(c *ctx) {
	a, ok := s.lookupAccount(c, false)
	if !ok {
		return
	}
	if a.UserID != c.user.ID {
		jsonMessage(c.w, http.StatusForbidden, "Forbidden")
		return
	}
	uri := otp.BuildURI(otp.Account{
		Type:      a.OTPType,
		Secret:    a.Secret,
		Algorithm: a.Algorithm,
		Digits:    a.Digits,
		Period:    a.Period,
		Counter:   a.Counter,
		Service:   a.Service,
		Account:   a.Account,
	})
	img, err := qrDataURI(uri)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{"qrcode": img})
}

// decodeQrCode handles POST /api/v1/qrcode/decode.
func (s *Server) decodeQrCode(c *ctx) {
	uri, err := decodeUploadedQR(c.r)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "file_upload_failed")
		return
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{"data": uri})
}

// accountOtpLogs handles GET /api/v1/twofaccounts/{twofaccount}/otp-logs.
func (s *Server) accountOtpLogs(c *ctx) {
	a, ok := s.lookupAccount(c, true)
	if !ok {
		return
	}
	period, _ := parseIntQS(c.query("period"))
	limit, _ := parseIntQS(c.query("limit"))
	logs, err := s.st.ListOtpLogs(a.ID, period, limit)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	out := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		item := map[string]any{
			"requester_name":  l.RequesterName,
			"requester_email": l.RequesterEmail,
			"owner_name":      l.OwnerName,
			"owner_email":     l.OwnerEmail,
			"ip_address":      l.IPAddress,
			"generated_at":    l.GeneratedAt.Format("2006-01-02 15:04:05"),
			"otp_type":        l.OTPType,
		}
		if l.OTPType == "hotp" && l.Counter != nil {
			item["counter"] = *l.Counter
		}
		out = append(out, item)
	}
	jsonCollection(c.w, http.StatusOK, out)
}

// logOtpGeneration records an OTP generation event.
func (s *Server) logOtpGeneration(c *ctx, a *store.Account, otpType string, counter *int64) {
	s.logOtpFor(c.user, s.clientIP(c.r), a, otpType, counter)
}

// logOtpFor records an OTP generation event by a user, from any front end.
func (s *Server) logOtpFor(user *store.User, ip string, a *store.Account, otpType string, counter *int64) {
	if a == nil || a.ID <= 0 {
		return
	}
	entry := &store.OtpLog{
		RequesterName:  user.Name,
		RequesterEmail: user.Email,
		OwnerName:      user.Name,
		OwnerEmail:     user.Email,
		TwoFAccountID:  a.ID,
		IPAddress:      ip,
		OTPType:        otpType,
		Counter:        counter,
		GeneratedAt:    time.Now(),
	}
	if a.UserID != user.ID {
		if owner, err := s.st.GetUser(a.UserID); err == nil {
			entry.OwnerName = owner.Name
			entry.OwnerEmail = owner.Email
			entry.OwnerID = &owner.ID
		}
	} else {
		entry.OwnerID = &user.ID
	}
	entry.RequesterID = &user.ID
	_ = s.st.CreateOtpLog(entry)
}

var _ = fmt.Sprintf
var _ = json.Marshal
