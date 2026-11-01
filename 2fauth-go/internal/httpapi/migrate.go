package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"2fauth/internal/otp"
	"2fauth/internal/store"
)

// migrateAccounts handles POST /api/v1/twofaccounts/migration.
//
// It parses an export payload from a supported app and returns the list of
// accounts without persisting them (ids are -2 for new accounts and -1 for
// duplicates of accounts already owned by the user).
func (s *Server) migrateAccounts(c *ctx) {
	payload, ok := c.json()["payload"].(string)
	if !ok || strings.TrimSpace(payload) == "" {
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
			"message": "The payload field is required.",
			"errors":  map[string][]string{"payload": {"The payload field is required."}},
		})
		return
	}

	accounts, err := parseMigrationPayload(payload)
	if err != nil {
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
			"message": err.Error(),
			"errors":  map[string][]string{"payload": {err.Error()}},
		})
		return
	}

	owned, _ := s.st.ListVisibleAccounts(c.user.ID, s.accountScope(), nil)
	out := make([]map[string]any, 0, len(accounts))
	for _, a := range accounts {
		payload := s.accountStorePayload(a, true, true)
		payload["id"] = fakeID
		if isDuplicate(a, owned) {
			payload["id"] = duplicateID
		}
		out = append(out, payload)
	}

	// The SPA consumes this endpoint as a bare array.
	jsonResponse(c.w, http.StatusOK, out)
}

const (
	duplicateID = -1
	fakeID      = -2
)

func isDuplicate(a *store.Account, owned []*store.Account) bool {
	for _, o := range owned {
		if o.Secret == a.Secret && o.Service == a.Service && o.Account == a.Account &&
			o.OTPType == a.OTPType && o.Digits == a.Digits && o.Algorithm == a.Algorithm {
			return true
		}
	}
	return false
}

// parseMigrationPayload detects the source format and returns unsaved accounts.
func parseMigrationPayload(payload string) ([]*store.Account, error) {
	trimmed := strings.TrimSpace(payload)

	if strings.HasPrefix(strings.ToLower(trimmed), "otpauth-migration://") {
		return parseGoogleAuthenticatorMigration(trimmed)
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "otpauth://") {
		return parseOTPAuthList(trimmed)
	}
	if strings.HasPrefix(trimmed, "{") {
		return parseJSONMigration(trimmed)
	}
	if strings.HasPrefix(trimmed, "[") {
		return parseOTPAuthList(trimmed)
	}
	return parsePlainTextList(trimmed)
}

// parseJSONMigration handles 2FAuth, Aegis and 2FAS JSON exports.
func parseJSONMigration(payload string) ([]*store.Account, error) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(payload), &doc); err != nil {
		return nil, errInvalidPayload
	}

	if _, ok := doc["app"]; ok {
		return parseTwoFAuthExport(doc)
	}
	if _, ok := doc["entries"]; ok {
		return parseAegisExport(doc)
	}
	if _, ok := doc["services"]; ok {
		return parseTwoFASExport(doc)
	}
	return nil, errInvalidPayload
}

var errInvalidPayload = &migrationError{"Invalid data or 2FAuth format not recognized"}

type migrationError struct{ msg string }

func (e *migrationError) Error() string { return e.msg }

func parseTwoFAuthExport(doc map[string]any) ([]*store.Account, error) {
	raw, _ := doc["data"].([]any)
	if raw == nil {
		return nil, errInvalidPayload
	}
	accounts := make([]*store.Account, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		acc := &store.Account{
			OTPType:   str(entry["otp_type"]),
			Account:   str(entry["account"]),
			Service:   str(entry["service"]),
			Icon:      str(entry["icon"]),
			Secret:    otp.PadSecret(str(entry["secret"])),
			Algorithm: defaultString(strings.ToLower(str(entry["algorithm"])), otp.DefaultAlgorithm),
			Digits:    numOr(entry["digits"], otp.DefaultDigits),
			LegacyURI: str(entry["legacy_uri"]),
		}
		if acc.OTPType == "" {
			acc.OTPType = otp.TypeTOTP
		}
		if v, ok := entry["period"]; ok && v != nil {
			period := numOr(v, 0)
			if period > 0 {
				acc.Period = &period
			}
		}
		if v, ok := entry["counter"]; ok && v != nil {
			counter := int64(numOr(v, 0))
			acc.Counter = &counter
		}
		if acc.OTPType == otp.TypeTOTP && acc.Period == nil {
			p := otp.DefaultPeriod
			acc.Period = &p
		}
		if acc.OTPType == otp.TypeHOTP && acc.Counter == nil {
			c := int64(otp.DefaultCounter)
			acc.Counter = &c
		}
		if acc.HOTP() {
			acc.Period = nil
		}
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func parseAegisExport(doc map[string]any) ([]*store.Account, error) {
	raw, _ := doc["entries"].([]any)
	if raw == nil {
		return nil, errInvalidPayload
	}
	accounts := make([]*store.Account, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		info, _ := entry["info"].(map[string]any)
		otpType := strings.ToLower(str(entry["type"]))
		acc := &store.Account{
			Account:   str(entry["account"]),
			Service:   str(entry["issuer"]),
			OTPType:   otp.TypeTOTP,
			Algorithm: otp.DefaultAlgorithm,
			Digits:    otp.DefaultDigits,
		}
		if otpType == "hotp" {
			acc.OTPType = otp.TypeHOTP
		}
		if otpType == "steam" {
			acc.OTPType = otp.TypeSteam
		}
		if info != nil {
			acc.Secret = otp.PadSecret(str(info["secret"]))
			if algo := str(info["algo"]); algo != "" {
				acc.Algorithm = strings.ToLower(algo)
			}
			if d := numOr(info["digits"], 0); d > 0 {
				acc.Digits = d
			}
			if p := numOr(info["period"], 0); p > 0 {
				acc.Period = &p
			}
			if c := numOr(info["counter"], 0); c > 0 {
				v := int64(c)
				acc.Counter = &v
			}
		}
		if acc.Service == "" {
			acc.Service = str(entry["name"])
		}
		if acc.Account == "" {
			acc.Account = str(entry["name"])
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
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func parseTwoFASExport(doc map[string]any) ([]*store.Account, error) {
	raw, _ := doc["services"].([]any)
	if raw == nil {
		return nil, errInvalidPayload
	}
	accounts := make([]*store.Account, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if uri := str(entry["otpauth"]); uri != "" {
			parsed, err := otp.ParseURI(uri)
			if err == nil {
				accounts = append(accounts, accountFromParsed(parsed))
				continue
			}
		}
		name := str(entry["name"])
		acc := &store.Account{
			Service:   name,
			Account:   name,
			OTPType:   otp.TypeTOTP,
			Secret:    otp.PadSecret(str(entry["secret"])),
			Algorithm: otp.DefaultAlgorithm,
			Digits:    otp.DefaultDigits,
		}
		p := otp.DefaultPeriod
		acc.Period = &p
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

// parseOTPAuthList parses a list of otpauth:// URIs (one per line or a JSON array).
func parseOTPAuthList(payload string) ([]*store.Account, error) {
	var uris []string
	if strings.HasPrefix(payload, "[") {
		var raw []map[string]any
		if err := json.Unmarshal([]byte(payload), &raw); err == nil {
			for _, item := range raw {
				if u := str(item["uri"]); u != "" {
					uris = append(uris, u)
				}
			}
		}
	}
	if len(uris) == 0 {
		uris = strings.Split(payload, "\n")
	}
	accounts := make([]*store.Account, 0, len(uris))
	for _, line := range uris {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), "otpauth://") {
			continue
		}
		parsed, err := otp.ParseURI(line)
		if err != nil {
			continue
		}
		accounts = append(accounts, accountFromParsed(parsed))
	}
	if len(accounts) == 0 {
		return nil, errInvalidPayload
	}
	return accounts, nil
}

// parsePlainTextList handles the "issuer:account:secret" and otpauth URI formats.
func parsePlainTextList(payload string) ([]*store.Account, error) {
	accounts := make([]*store.Account, 0)
	for _, line := range strings.Split(payload, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "otpauth://") {
			if parsed, err := otp.ParseURI(line); err == nil {
				accounts = append(accounts, accountFromParsed(parsed))
			}
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}
		secret := parts[len(parts)-1]
		issuer := parts[0]
		account := strings.Join(parts[1:len(parts)-1], ":")
		acc := &store.Account{
			Service:   strings.TrimSpace(issuer),
			Account:   strings.TrimSpace(account),
			OTPType:   otp.TypeTOTP,
			Secret:    otp.PadSecret(strings.ReplaceAll(strings.TrimSpace(secret), " ", "")),
			Algorithm: otp.DefaultAlgorithm,
			Digits:    otp.DefaultDigits,
		}
		p := otp.DefaultPeriod
		acc.Period = &p
		accounts = append(accounts, acc)
	}
	if len(accounts) == 0 {
		return nil, errInvalidPayload
	}
	return accounts, nil
}

// accountFromParsed converts an otp.Account into a store.Account.
func accountFromParsed(parsed otp.Account) *store.Account {
	acc := &store.Account{
		Service:   parsed.Service,
		Account:   parsed.Account,
		OTPType:   parsed.Type,
		Secret:    otp.PadSecret(parsed.Secret),
		Algorithm: defaultString(strings.ToLower(parsed.Algorithm), otp.DefaultAlgorithm),
		Digits:    parsed.Digits,
		Period:    parsed.Period,
		Counter:   parsed.Counter,
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

// parseGoogleAuthenticatorMigration decodes an otpauth-migration:// payload.
func parseGoogleAuthenticatorMigration(uri string) ([]*store.Account, error) {
	idx := strings.Index(uri, "data=")
	if idx < 0 {
		return nil, errInvalidPayload
	}
	encoded := uri[idx+len("data="):]
	encoded, _ = urlQueryUnescape(encoded)
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, errInvalidPayload
		}
	}
	fields, err := parseProtoMessage(raw)
	if err != nil {
		return nil, errInvalidPayload
	}
	entries, _ := fields[1] // repeated otp_parameters
	accounts := make([]*store.Account, 0, len(entries))
	for _, entry := range entries {
		sub := entry.bytes
		params, err := parseProtoMessage(sub)
		if err != nil {
			continue
		}
		acc := &store.Account{
			OTPType:   otp.TypeTOTP,
			Algorithm: otp.DefaultAlgorithm,
			Digits:    otp.DefaultDigits,
		}
		if v, ok := params[1]; ok && len(v) > 0 {
			acc.Secret = otp.EncodeBase32(v[0].bytes)
		}
		if v, ok := params[2]; ok && len(v) > 0 {
			acc.Account = string(v[0].bytes)
		}
		if v, ok := params[3]; ok && len(v) > 0 {
			acc.Service = string(v[0].bytes)
		}
		if v, ok := params[4]; ok && len(v) > 0 {
			switch v[0].varint {
			case 1:
				acc.Algorithm = otp.SHA1
			case 2:
				acc.Algorithm = otp.SHA256
			case 3:
				acc.Algorithm = otp.SHA512
			case 4:
				acc.Algorithm = otp.MD5
			}
		}
		if v, ok := params[5]; ok && len(v) > 0 {
			switch v[0].varint {
			case 1:
				acc.Digits = 6
			case 2:
				acc.Digits = 8
			}
		}
		if v, ok := params[6]; ok && len(v) > 0 {
			switch v[0].varint {
			case 2:
				acc.OTPType = otp.TypeHOTP
			}
		}
		if v, ok := params[7]; ok && len(v) > 0 {
			c := int64(v[0].varint)
			acc.Counter = &c
		}
		if acc.OTPType == otp.TypeHOTP {
			if acc.Counter == nil {
				c := int64(0)
				acc.Counter = &c
			}
		} else {
			p := otp.DefaultPeriod
			acc.Period = &p
		}
		if acc.Account == "" {
			acc.Account = acc.Service
		}
		accounts = append(accounts, acc)
	}
	if len(accounts) == 0 {
		return nil, errInvalidPayload
	}
	return accounts, nil
}

// protoField is one decoded protobuf field value.
type protoField struct {
	varint uint64
	bytes  []byte
}

// parseProtoMessage decodes protobuf wire format into field values.
func parseProtoMessage(data []byte) (map[int][]protoField, error) {
	out := map[int][]protoField{}
	i := 0
	for i < len(data) {
		key, n := readVarint(data[i:])
		if n == 0 {
			return nil, errInvalidPayload
		}
		i += n
		field := int(key >> 3)
		wire := int(key & 0x7)
		switch wire {
		case 0:
			v, n := readVarint(data[i:])
			if n == 0 {
				return nil, errInvalidPayload
			}
			i += n
			out[field] = append(out[field], protoField{varint: v})
		case 2:
			l, n := readVarint(data[i:])
			if n == 0 || i+n+int(l) > len(data) {
				return nil, errInvalidPayload
			}
			i += n
			out[field] = append(out[field], protoField{bytes: data[i : i+int(l)]})
			i += int(l)
		case 5:
			i += 4
		case 1:
			i += 8
		default:
			return nil, errInvalidPayload
		}
	}
	return out, nil
}

func readVarint(b []byte) (uint64, int) {
	var result uint64
	var shift uint
	for i := 0; i < len(b); i++ {
		result |= uint64(b[i]&0x7f) << shift
		if b[i]&0x80 == 0 {
			return result, i + 1
		}
		shift += 7
		if shift > 63 {
			return 0, 0
		}
	}
	return 0, 0
}

func urlQueryUnescape(s string) (string, error) {
	return strings.ReplaceAll(s, "%2B", "+"), nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func numOr(v any, def int) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case json.Number:
		n, err := strconv.Atoi(t.String())
		if err == nil {
			return n
		}
	case string:
		n, err := strconv.Atoi(t)
		if err == nil {
			return n
		}
	}
	return def
}

func defaultString(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
