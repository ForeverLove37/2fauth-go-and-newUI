package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"2fauth/internal/store"
)

// deviceRoutes is everything the token of a phone app may call. Anything else (the
// exports, QR codes of secrets, other tokens, SSH keys, administration) answers 403,
// so a stolen phone token cannot turn into more than the phone itself could do.
var deviceRoutes = map[string]bool{
	"GET api/v1/user":                                    true,
	"GET api/v1/user/preferences":                        true,
	"GET api/v1/user/preferences/{preferenceName}":       true,
	"PUT api/v1/user/preferences/{preferenceName}":       true,
	"GET api/v1/twofaccounts":                            true,
	"POST api/v1/twofaccounts":                           true,
	"DELETE api/v1/twofaccounts":                         true,
	"PATCH api/v1/twofaccounts/withdraw":                 true,
	"POST api/v1/twofaccounts/reorder":                   true,
	"POST api/v1/twofaccounts/preview":                   true,
	"POST api/v1/twofaccounts/import":                    true,
	"GET api/v1/twofaccounts/count":                      true,
	"GET api/v1/twofaccounts/{twofaccount}":              true,
	"PUT api/v1/twofaccounts/{twofaccount}":              true,
	"PATCH api/v1/twofaccounts/{twofaccount}":            true,
	"DELETE api/v1/twofaccounts/{twofaccount}":           true,
	"PATCH api/v1/twofaccounts/{twofaccount}/favorite":   true,
	"PATCH api/v1/twofaccounts/{twofaccount}/phone-sync": true,
	"GET api/v1/twofaccounts/{id}/otp":                   true,
	"POST api/v1/qrcode/decode":                          true,
	"GET api/v1/groups":                                  true,
	"POST api/v1/groups":                                 true,
	"GET api/v1/groups/{group}":                          true,
	"PUT api/v1/groups/{group}":                          true,
	"PATCH api/v1/groups/{group}":                        true,
	"DELETE api/v1/groups/{group}":                       true,
	"GET api/v1/groups/{group}/twofaccounts":             true,
	"POST api/v1/groups/{group}/assign":                  true,
	"POST api/v1/groups/reorder":                         true,
	"GET api/v1/icons/packs":                             true,
	"GET api/v1/icons/providers":                         true,
	"POST api/v1/icons/default":                          true,
	"POST api/v1/icons":                                  true,
	"GET api/v1/features":                                true,
	"GET api/v1/features/{feature}":                      true,
	"GET api/v1/sync":                                    true,
	// only its own token, checked by the handler: signing out
	"DELETE oauth/personal-access-tokens/{token_id}": true,
}

// createDeviceToken handles POST /api/v1/device-tokens: the phone app swaps the
// session (or a full token pasted by the user) for a token limited to deviceRoutes.
func (s *Server) createDeviceToken(c *ctx) {
	if c.device {
		jsonMessage(c.w, http.StatusForbidden, "A device token cannot issue tokens")
		return
	}
	if !s.patAllowed(c) {
		return
	}
	v := newValidator(c)
	name := v.required("name")
	v.maxLen("name", name, 255)
	if v.respond() {
		return
	}
	token, plain, err := s.st.CreateToken(c.user.ID, name, []string{store.DeviceScope})
	if err != nil {
		s.srvError(c, err)
		return
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"accessToken":   plain,
		"accessTokenId": token.ID,
	})
}

// accountRevision changes whenever a field the phone keeps changes. The HOTP counter
// is left out: it moves on every code and both sides keep the highest value.
func accountRevision(a *store.Account) string {
	period := 0
	if a.Period != nil {
		period = *a.Period
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00%s",
		a.OTPType, a.Service, a.Account, a.Secret, a.Algorithm, a.Digits, period, a.Icon)))
	return hex.EncodeToString(sum[:12])
}

// syncAccounts handles GET /api/v1/sync: the accounts the phone keeps a copy of, with
// their secrets, and the ids of the user's other accounts, so that a phone can drop a
// copy once its account is made cloud-only on the web.
func (s *Server) syncAccounts(c *ctx) {
	owned, err := s.st.OwnedAccounts(c.user.ID)
	if err != nil {
		s.srvError(c, err)
		return
	}
	synced := make([]map[string]any, 0)
	cloud := make([]int64, 0)
	for _, a := range owned {
		if !a.PhoneSync {
			cloud = append(cloud, a.ID)
			continue
		}
		payload := s.accountReadPayload(c, a, false, 0)
		payload["secret"] = a.Secret
		payload["revision"] = accountRevision(a)
		if a.UpdatedAt != nil {
			payload["updated_at"] = a.UpdatedAt.Unix()
		}
		synced = append(synced, payload)
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"synced":      synced,
		"cloud_ids":   cloud,
		"server_time": time.Now().Unix(),
	})
}
