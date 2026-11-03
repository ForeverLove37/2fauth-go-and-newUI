package httpapi

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"2fauth/internal/otp"
	"2fauth/internal/services"
	"2fauth/internal/store"
)

// Accepted import formats (see docs/IMPORT.md for the details):
//
//	2FAuth JSON, plain or encrypted            Aegis JSON, plain or encrypted
//	2FAS JSON, plain or encrypted              Google Authenticator otpauth-migration://
//	Bitwarden JSON (unencrypted) and CSV       andOTP JSON (unencrypted)
//	FreeOTP+ JSON                              Authenticator Pro JSON (unencrypted)
//	Raivo JSON                                 LastPass Authenticator JSON
//	Proton Authenticator JSON                  generic CSV with a header line
//	otpauth:// lists (text or JSON)            issuer:account:secret lines

const (
	duplicateID = -1
	fakeID      = -2
)

// importIssue describes an entry of an import file that could not be used.
type importIssue struct {
	Index   int    `json:"index"`
	Service string `json:"service"`
	Reason  string `json:"reason"`
}

// migrationError is an error whose message is meant for the user.
type migrationError struct{ msg string }

func (e *migrationError) Error() string { return e.msg }

var (
	errInvalidPayload     = &migrationError{"Invalid data or 2FAuth format not recognized"}
	errPassphraseRequired = &migrationError{"This file is encrypted: a passphrase is required"}
	errPassphraseWrong    = &migrationError{"The passphrase is wrong or the file is damaged"}
)

// migrationFailure writes the 422 of a payload that cannot be read. needs_passphrase
// tells a client to ask for the passphrase and send the request again.
func migrationFailure(c *ctx, err error) {
	field := "payload"
	if errors.Is(err, errPassphraseRequired) || errors.Is(err, errPassphraseWrong) {
		field = "passphrase"
	}
	jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
		"message":          err.Error(),
		"errors":           map[string][]string{field: {err.Error()}},
		"needs_passphrase": errors.Is(err, errPassphraseRequired),
	})
}

// importAllowed applies THROTTLE_API_DURING_IMPORT: an import can be expensive and
// an encrypted one runs a key derivation chosen by the file.
func (s *Server) importAllowed(c *ctx) bool {
	if s.cfg.ThrottleAPIImport <= 0 {
		return true
	}
	ok, retry := s.throttle.Allow("import|"+strconv.FormatInt(c.user.ID, 10), s.cfg.ThrottleAPIImport, time.Minute)
	if !ok {
		c.w.Header().Set("Retry-After", strconv.Itoa(retry))
		jsonMessage(c.w, http.StatusTooManyRequests, "Too many imports.")
	}
	return ok
}

// migrateAccounts handles POST /api/v1/twofaccounts/migration.
//
// It parses an export payload from a supported app and returns the list of
// accounts without persisting them (ids are -2 for new accounts and -1 for
// duplicates of accounts already owned by the user).
func (s *Server) migrateAccounts(c *ctx) {
	if !s.importAllowed(c) {
		return
	}
	payload, ok := c.json()["payload"].(string)
	if !ok || strings.TrimSpace(payload) == "" {
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
			"message": "The payload field is required.",
			"errors":  map[string][]string{"payload": {"The payload field is required."}},
		})
		return
	}

	accounts, _, err := parseMigrationPayload(payload, c.input("passphrase"))
	if err != nil {
		migrationFailure(c, err)
		return
	}

	owned, _ := s.st.OwnedAccounts(c.user.ID)
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

// importAccounts handles POST /api/v1/twofaccounts/import: it reads a payload and
// keeps its new accounts in one request. Entries that cannot be used do not stop the
// others, they are reported with their reason.
func (s *Server) importAccounts(c *ctx) {
	if !s.importAllowed(c) {
		return
	}
	payload, ok := c.json()["payload"].(string)
	if !ok || strings.TrimSpace(payload) == "" {
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
			"message": "The payload field is required.",
			"errors":  map[string][]string{"payload": {"The payload field is required."}},
		})
		return
	}
	accounts, issues, err := parseMigrationPayload(payload, c.input("passphrase"))
	if err != nil {
		migrationFailure(c, err)
		return
	}
	owned, err := s.st.OwnedAccounts(c.user.ID)
	if err != nil {
		s.srvError(c, err)
		return
	}
	imported, duplicates := 0, 0
	for i, a := range accounts {
		if isDuplicate(a, owned) {
			duplicates++
			continue
		}
		a.UserID = c.user.ID
		a.Icon = s.restoreImportedIcon(a, c.user.ID)
		created, err := s.st.CreateAccount(a)
		if err != nil {
			issues = append(issues, importIssue{Index: i, Service: a.Service, Reason: "storage error"})
			continue
		}
		owned = append(owned, created)
		imported++
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"total":      len(accounts) + len(issues),
		"imported":   imported,
		"duplicates": duplicates,
		"invalid":    len(issues),
		"issues":     issues,
	})
}

// restoreImportedIcon stores the icon embedded in an import file. Its bytes are as
// untrusted as an upload and go through the same validation. An icon that is missing
// or refused only means the account has none.
func (s *Server) restoreImportedIcon(a *store.Account, userID int64) string {
	if a.ImportIcon == "" {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(a.ImportIcon)
	if err != nil {
		return ""
	}
	name, err := s.icons.StoreUpload(raw)
	if err != nil {
		return ""
	}
	_ = s.st.AddIconUpload(name, userID)
	return name
}

func isDuplicate(a *store.Account, owned []*store.Account) bool {
	secret := otp.SanitizeSecret(a.Secret)
	for _, o := range owned {
		if otp.SanitizeSecret(o.Secret) == secret && o.Service == a.Service && o.Account == a.Account &&
			o.OTPType == a.OTPType && o.Digits == a.Digits && o.Algorithm == a.Algorithm {
			return true
		}
	}
	return false
}

// parseMigrationPayload detects the source format and returns the accounts that can
// be used, with the entries that cannot.
func parseMigrationPayload(payload, passphrase string) ([]*store.Account, []importIssue, error) {
	raw, err := parseAnyFormat(strings.TrimPrefix(strings.TrimSpace(payload), "\ufeff"), passphrase)
	if err != nil {
		return nil, nil, err
	}
	accounts := make([]*store.Account, 0, len(raw))
	var issues []importIssue
	for i, a := range raw {
		if a.OTPType == "" {
			a.OTPType = otp.TypeTOTP
		}
		a.Algorithm = strings.ToLower(a.Algorithm)
		if a.Algorithm == "" {
			a.Algorithm = otp.DefaultAlgorithm
		}
		if a.Digits == 0 {
			a.Digits = otp.DefaultDigits
		}
		if a.OTPType == otp.TypeSteam {
			a.Service, a.Digits, a.Algorithm = "Steam", 5, otp.SHA1
		}
		if a.OTPType == otp.TypeHOTP {
			a.Period = nil
			if a.Counter == nil {
				zero := int64(otp.DefaultCounter)
				a.Counter = &zero
			}
		} else {
			a.Counter = nil
			if a.Period == nil {
				p := otp.DefaultPeriod
				a.Period = &p
			}
		}
		a.Secret = otp.PadSecret(otp.SanitizeSecret(a.Secret))
		a.Service = strings.TrimSpace(strings.ReplaceAll(a.Service, ":", " "))
		a.Account = strings.TrimSpace(strings.ReplaceAll(a.Account, ":", " "))
		if a.Account == "" {
			a.Account = a.Service
		}
		if !services.ValidIconName(a.Icon) {
			a.Icon = ""
		}
		if a.Account == "" {
			a.Account = "imported"
		}
		if err := validateStored(a); err != nil {
			issues = append(issues, importIssue{Index: i, Service: a.Service, Reason: err.Error()})
			continue
		}
		accounts = append(accounts, a)
	}
	if len(accounts) == 0 && len(issues) == 0 {
		return nil, nil, errInvalidPayload
	}
	if len(accounts) == 0 {
		return nil, nil, &migrationError{"No entry of this file can be used: " + issues[0].Reason}
	}
	return accounts, issues, nil
}

func parseAnyFormat(trimmed, passphrase string) ([]*store.Account, error) {
	switch {
	case strings.HasPrefix(trimmed, "{"):
		return parseJSONObject(trimmed, passphrase)
	case strings.HasPrefix(trimmed, "["):
		return parseJSONArray(trimmed)
	}
	if accounts, ok := parseCSV(trimmed); ok {
		return accounts, nil
	}
	return parseLines(trimmed)
}

// parseJSONObject recognises an export from the shape of its top-level keys.
func parseJSONObject(payload, passphrase string) ([]*store.Account, error) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(payload), &doc); err != nil {
		return nil, errInvalidPayload
	}
	if str(doc["format"]) == encryptedExportFormat {
		plain, err := decryptExport(doc, passphrase)
		if err != nil {
			return nil, err
		}
		return parseJSONObject(string(plain), "")
	}
	// Exports written by earlier builds wrapped the document in a "data" key.
	if inner, ok := doc["data"].(map[string]any); ok && inner["app"] != nil {
		doc = inner
	}
	switch {
	case doc["app"] != nil:
		return parseTwoFAuthExport(doc)
	case doc["db"] != nil && doc["header"] != nil:
		return parseAegisDocument(doc, passphrase)
	case doc["entries"] != nil && isProtonExport(doc):
		return parseProtonExport(doc)
	case doc["entries"] != nil:
		return parseAegisEntries(doc["entries"])
	case doc["servicesEncrypted"] != nil && str(doc["servicesEncrypted"]) != "":
		plain, err := decrypt2FAS(str(doc["servicesEncrypted"]), passphrase)
		if err != nil {
			return nil, err
		}
		var services []any
		if err := json.Unmarshal(plain, &services); err != nil {
			return nil, errInvalidPayload
		}
		return parseTwoFASServices(services)
	case doc["services"] != nil:
		services, _ := doc["services"].([]any)
		return parseTwoFASServices(services)
	case doc["items"] != nil || doc["encrypted"] == true:
		return parseBitwardenExport(doc)
	case doc["tokens"] != nil:
		return parseFreeOTPExport(doc)
	case doc["Authenticators"] != nil:
		return parseAuthenticatorProExport(doc)
	case doc["accounts"] != nil:
		return parseLastPassExport(doc)
	}
	return nil, errInvalidPayload
}

// parseJSONArray handles the formats exported as a bare list.
func parseJSONArray(payload string) ([]*store.Account, error) {
	var items []map[string]any
	if err := json.Unmarshal([]byte(payload), &items); err != nil {
		return parseLines(payload)
	}
	if len(items) == 0 {
		return nil, errInvalidPayload
	}
	first := items[0]
	switch {
	case first["uri"] != nil:
		var lines []string
		for _, item := range items {
			lines = append(lines, str(item["uri"]))
		}
		return parseLines(strings.Join(lines, "\n"))
	case first["kind"] != nil && first["secret"] != nil:
		return parseRaivoExport(items)
	case first["secret"] != nil:
		return parseAndOTPExport(items)
	}
	return nil, errInvalidPayload
}

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
			OTPType:    str(entry["otp_type"]),
			Account:    str(entry["account"]),
			Service:    str(entry["service"]),
			Secret:     str(entry["secret"]),
			Algorithm:  str(entry["algorithm"]),
			Digits:     numOr(entry["digits"], otp.DefaultDigits),
			LegacyURI:  str(entry["legacy_uri"]),
			ImportIcon: str(entry["icon_file"]),
		}
		setPeriod(acc, numOr(entry["period"], 0))
		setCounter(acc, entry["counter"])
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

// parseAegisDocument reads a real Aegis file: {"header":{"slots",...},"db":...}, where
// db is an object in a plain export and a base64 ciphertext in an encrypted one.
func parseAegisDocument(doc map[string]any, passphrase string) ([]*store.Account, error) {
	if text, encrypted := doc["db"].(string); encrypted {
		plain, err := decryptAegis(doc, text, passphrase)
		if err != nil {
			return nil, err
		}
		var db map[string]any
		if err := json.Unmarshal(plain, &db); err != nil {
			return nil, errInvalidPayload
		}
		return parseAegisEntries(db["entries"])
	}
	db, _ := doc["db"].(map[string]any)
	if db == nil {
		return nil, errInvalidPayload
	}
	return parseAegisEntries(db["entries"])
}

func parseAegisEntries(value any) ([]*store.Account, error) {
	raw, _ := value.([]any)
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
		name := str(entry["name"])
		if name == "" {
			name = str(entry["account"])
		}
		acc := &store.Account{
			Account:   name,
			Service:   str(entry["issuer"]),
			OTPType:   typeFromName(str(entry["type"])),
			Algorithm: str(info["algo"]),
			Digits:    numOr(info["digits"], 0),
			Secret:    str(info["secret"]),
		}
		setPeriod(acc, numOr(info["period"], 0))
		setCounter(acc, info["counter"])
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

// typeFromName maps the OTP type names used by the other applications.
func typeFromName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "hotp":
		return otp.TypeHOTP
	case "steam", "steamtotp":
		return otp.TypeSteam
	case "", "totp":
		return otp.TypeTOTP
	}
	// An unknown type (mOTP, Yandex...) must not be silently computed as TOTP.
	return "unsupported:" + name
}

func parseTwoFASServices(raw []any) ([]*store.Account, error) {
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
			if parsed, err := otp.ParseURI(uri); err == nil {
				accounts = append(accounts, accountFromParsed(parsed))
				continue
			}
		}
		details, _ := entry["otp"].(map[string]any)
		service := str(details["issuer"])
		if service == "" {
			service = str(entry["name"])
		}
		account := str(details["account"])
		if account == "" {
			account = str(details["label"])
		}
		acc := &store.Account{
			Service:   service,
			Account:   account,
			OTPType:   typeFromName(str(details["tokenType"])),
			Secret:    str(entry["secret"]),
			Algorithm: str(details["algorithm"]),
			Digits:    numOr(details["digits"], 0),
		}
		setPeriod(acc, numOr(details["period"], 0))
		setCounter(acc, details["counter"])
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func parseBitwardenExport(doc map[string]any) ([]*store.Account, error) {
	if doc["encrypted"] == true {
		return nil, &migrationError{"Password-protected Bitwarden exports are not supported: export again without a password"}
	}
	raw, _ := doc["items"].([]any)
	if raw == nil {
		return nil, errInvalidPayload
	}
	accounts := make([]*store.Account, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		login, _ := entry["login"].(map[string]any)
		if totp := str(login["totp"]); totp != "" {
			accounts = append(accounts, accountFromTOTPField(totp, str(entry["name"]), str(login["username"])))
		}
	}
	return accounts, nil
}

// accountFromTOTPField reads a password manager field: an otpauth URI, a steam://
// value or a bare base32 secret.
func accountFromTOTPField(value, service, account string) *store.Account {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "otpauth://") {
		if parsed, err := otp.ParseURI(value); err == nil {
			acc := accountFromParsed(parsed)
			if acc.Service == "" {
				acc.Service = service
			}
			if acc.Account == "" {
				acc.Account = account
			}
			return acc
		}
		return &store.Account{Service: service, Account: account, OTPType: otp.TypeTOTP}
	}
	if strings.HasPrefix(lower, "steam://") {
		return &store.Account{Service: "Steam", Account: account, OTPType: otp.TypeSteam, Secret: value[len("steam://"):]}
	}
	return &store.Account{Service: service, Account: account, OTPType: otp.TypeTOTP, Secret: value}
}

func parseAndOTPExport(items []map[string]any) ([]*store.Account, error) {
	accounts := make([]*store.Account, 0, len(items))
	for _, entry := range items {
		service := str(entry["issuer"])
		label := str(entry["label"])
		if service == "" {
			if head, tail, ok := strings.Cut(label, ":"); ok {
				service, label = strings.TrimSpace(head), strings.TrimSpace(tail)
			}
		}
		acc := &store.Account{
			Service:   service,
			Account:   label,
			OTPType:   typeFromName(str(entry["type"])),
			Secret:    str(entry["secret"]),
			Algorithm: str(entry["algorithm"]),
			Digits:    numOr(entry["digits"], 0),
		}
		setPeriod(acc, numOr(entry["period"], 0))
		setCounter(acc, entry["counter"])
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func parseRaivoExport(items []map[string]any) ([]*store.Account, error) {
	accounts := make([]*store.Account, 0, len(items))
	for _, entry := range items {
		acc := &store.Account{
			Service:   str(entry["issuer"]),
			Account:   str(entry["account"]),
			OTPType:   typeFromName(str(entry["kind"])),
			Secret:    str(entry["secret"]),
			Algorithm: str(entry["algorithm"]),
			Digits:    numOr(entry["digits"], 0),
		}
		setPeriod(acc, numOr(entry["timer"], 0))
		setCounter(acc, entry["counter"])
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func parseFreeOTPExport(doc map[string]any) ([]*store.Account, error) {
	raw, _ := doc["tokens"].([]any)
	if raw == nil {
		return nil, errInvalidPayload
	}
	accounts := make([]*store.Account, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// The secret is a JSON array of signed bytes.
		var key []byte
		list, _ := entry["secret"].([]any)
		for _, b := range list {
			key = append(key, byte(numOr(b, 0)))
		}
		service := str(entry["issuerExt"])
		if service == "" {
			service = str(entry["issuerInt"])
		}
		acc := &store.Account{
			Service:   service,
			Account:   str(entry["label"]),
			OTPType:   typeFromName(str(entry["type"])),
			Secret:    otp.EncodeBase32(key),
			Algorithm: str(entry["algo"]),
			Digits:    numOr(entry["digits"], 0),
		}
		setPeriod(acc, numOr(entry["period"], 0))
		setCounter(acc, entry["counter"])
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func parseAuthenticatorProExport(doc map[string]any) ([]*store.Account, error) {
	raw, _ := doc["Authenticators"].([]any)
	if raw == nil {
		return nil, errInvalidPayload
	}
	accounts := make([]*store.Account, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		kinds := map[int]string{1: "hotp", 2: "totp", 3: "motp", 4: "steam", 5: "yandex"}
		algorithms := map[int]string{0: otp.SHA1, 1: otp.SHA256, 2: otp.SHA512}
		acc := &store.Account{
			Service:   str(entry["Issuer"]),
			Account:   str(entry["Username"]),
			OTPType:   typeFromName(kinds[numOr(entry["Type"], 2)]),
			Secret:    str(entry["Secret"]),
			Algorithm: algorithms[numOr(entry["Algorithm"], 0)],
			Digits:    numOr(entry["Digits"], 0),
		}
		setPeriod(acc, numOr(entry["Period"], 0))
		setCounter(acc, entry["Counter"])
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func parseLastPassExport(doc map[string]any) ([]*store.Account, error) {
	raw, _ := doc["accounts"].([]any)
	if raw == nil {
		return nil, errInvalidPayload
	}
	accounts := make([]*store.Account, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		service := str(entry["issuerName"])
		if service == "" {
			service = str(entry["originalIssuerName"])
		}
		account := str(entry["userName"])
		if account == "" {
			account = str(entry["originalUserName"])
		}
		acc := &store.Account{
			Service:   service,
			Account:   account,
			OTPType:   otp.TypeTOTP,
			Secret:    str(entry["secret"]),
			Algorithm: str(entry["algorithm"]),
			Digits:    numOr(entry["digits"], 0),
		}
		setPeriod(acc, numOr(entry["timeStep"], 0))
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func isProtonExport(doc map[string]any) bool {
	entries, _ := doc["entries"].([]any)
	if len(entries) == 0 {
		return false
	}
	first, _ := entries[0].(map[string]any)
	return first != nil && first["content"] != nil
}

func parseProtonExport(doc map[string]any) ([]*store.Account, error) {
	raw, _ := doc["entries"].([]any)
	accounts := make([]*store.Account, 0, len(raw))
	for _, item := range raw {
		entry, _ := item.(map[string]any)
		content, _ := entry["content"].(map[string]any)
		uri := str(content["uri"])
		if strings.HasPrefix(strings.ToLower(uri), "steam://") {
			accounts = append(accounts, accountFromTOTPField(uri, "Steam", str(content["name"])))
			continue
		}
		parsed, err := otp.ParseURI(uri)
		if err != nil {
			accounts = append(accounts, &store.Account{Service: str(content["name"]), OTPType: otp.TypeTOTP})
			continue
		}
		acc := accountFromParsed(parsed)
		if acc.Service == "" {
			acc.Service = str(content["name"])
		}
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

// csvColumns maps the header names of the CSV exports to the fields they fill.
var csvColumns = map[string]string{
	"login_totp": "secret", "totp": "secret", "otp": "secret", "secret": "secret", "otpauth": "secret", "otpurl": "secret",
	"one-time password": "secret", "uri": "secret", "seed": "secret", "key": "secret",
	"name": "service", "service": "service", "issuer": "service", "title": "service", "site": "service",
	"account": "account", "username": "account", "login_username": "account", "user": "account", "label": "account", "email": "account",
	"digits": "digits", "period": "period", "algorithm": "algorithm", "type": "type", "otp_type": "type", "counter": "counter",
}

// parseCSV reads a CSV file whose header names a secret column, such as the exports
// of Bitwarden or 1Password. Text without such a header is not a CSV file here.
func parseCSV(payload string) ([]*store.Account, bool) {
	firstLine, _, _ := strings.Cut(payload, "\n")
	if !strings.Contains(firstLine, ",") && !strings.Contains(firstLine, ";") {
		return nil, false
	}
	reader := csv.NewReader(strings.NewReader(payload))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	if strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
		reader.Comma = ';'
	}
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		return nil, false
	}
	columns := map[string]int{}
	for i, name := range records[0] {
		field, known := csvColumns[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "\ufeff")))]
		if !known {
			continue
		}
		if _, taken := columns[field]; !taken {
			columns[field] = i
		}
	}
	if _, ok := columns["secret"]; !ok {
		return nil, false
	}
	value := func(record []string, field string) string {
		if i, ok := columns[field]; ok && i < len(record) {
			return strings.TrimSpace(record[i])
		}
		return ""
	}
	accounts := make([]*store.Account, 0, len(records)-1)
	for _, record := range records[1:] {
		secret := value(record, "secret")
		// A password manager export also lists the logins that have no OTP.
		if secret == "" {
			continue
		}
		acc := accountFromTOTPField(secret, value(record, "service"), value(record, "account"))
		// In a password manager export this column says "login": not an OTP type.
		switch t := strings.ToLower(value(record, "type")); t {
		case "hotp", "steam", "steamtotp", "motp", "yandex":
			if acc.OTPType == otp.TypeTOTP {
				acc.OTPType = typeFromName(t)
			}
		}
		if a := value(record, "algorithm"); a != "" && !strings.HasPrefix(strings.ToLower(secret), "otpauth://") {
			acc.Algorithm = a
		}
		if d := numOr(value(record, "digits"), 0); d > 0 {
			acc.Digits = d
		}
		setPeriod(acc, numOr(value(record, "period"), 0))
		if c := value(record, "counter"); c != "" {
			setCounter(acc, c)
		}
		accounts = append(accounts, acc)
	}
	return accounts, true
}

// parseLines handles text exports, one item per line: otpauth:// URIs,
// otpauth-migration:// URIs (each QR code of a Google Authenticator export gives one)
// and issuer:account:secret. Lines of other kinds are ignored.
func parseLines(payload string) ([]*store.Account, error) {
	accounts := make([]*store.Account, 0)
	unreadable := 0
	for _, line := range strings.Split(payload, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		switch {
		case line == "":
		case strings.HasPrefix(lower, "otpauth-migration://"):
			batch, err := parseGoogleAuthenticatorMigration(line)
			if err != nil {
				return nil, err
			}
			accounts = append(accounts, batch...)
		case strings.HasPrefix(lower, "otpauth://"), strings.HasPrefix(lower, "steam://"):
			acc := accountFromTOTPField(line, "", "")
			if acc.Secret == "" {
				unreadable++
				continue
			}
			accounts = append(accounts, acc)
		default:
			parts := strings.Split(line, ":")
			if len(parts) < 3 {
				unreadable++
				continue
			}
			accounts = append(accounts, &store.Account{
				Service: strings.TrimSpace(parts[0]),
				Account: strings.TrimSpace(strings.Join(parts[1:len(parts)-1], ":")),
				OTPType: otp.TypeTOTP,
				Secret:  strings.TrimSpace(parts[len(parts)-1]),
			})
		}
	}
	if len(accounts) == 0 {
		return nil, errInvalidPayload
	}
	return accounts, nil
}

// accountFromParsed converts an otp.Account into a store.Account.
func accountFromParsed(parsed otp.Account) *store.Account {
	return &store.Account{
		Service:   parsed.Service,
		Account:   parsed.Account,
		OTPType:   parsed.Type,
		Secret:    parsed.Secret,
		Algorithm: parsed.Algorithm,
		Digits:    parsed.Digits,
		Period:    parsed.Period,
		Counter:   parsed.Counter,
	}
}

func setPeriod(a *store.Account, period int) {
	if period > 0 {
		a.Period = &period
	}
}

func setCounter(a *store.Account, value any) {
	if value == nil {
		return
	}
	counter := int64(numOr(value, 0))
	a.Counter = &counter
}

// parseGoogleAuthenticatorMigration decodes an otpauth-migration:// payload.
func parseGoogleAuthenticatorMigration(uri string) ([]*store.Account, error) {
	encoded := ""
	if parsed, err := url.Parse(uri); err == nil {
		encoded = parsed.Query().Get("data")
	}
	if encoded == "" {
		return nil, errInvalidPayload
	}
	// A "+" left unescaped by the QR code reader decodes as a space.
	encoded = strings.ReplaceAll(encoded, " ", "+")
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(encoded, "="))
		if err != nil {
			raw, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(encoded, "="))
			if err != nil {
				return nil, errInvalidPayload
			}
		}
	}
	fields, err := parseProtoMessage(raw)
	if err != nil {
		return nil, errInvalidPayload
	}
	entries := fields[1] // repeated otp_parameters
	accounts := make([]*store.Account, 0, len(entries))
	for _, entry := range entries {
		params, err := parseProtoMessage(entry.bytes)
		if err != nil {
			continue
		}
		acc := &store.Account{OTPType: otp.TypeTOTP}
		if v := params[1]; len(v) > 0 {
			acc.Secret = otp.EncodeBase32(v[0].bytes)
		}
		if v := params[2]; len(v) > 0 {
			acc.Account = string(v[0].bytes)
		}
		if v := params[3]; len(v) > 0 {
			acc.Service = string(v[0].bytes)
		}
		// In the label, the issuer is often repeated as a prefix.
		if head, tail, ok := strings.Cut(acc.Account, ":"); ok && (acc.Service == "" || acc.Service == head) {
			acc.Service, acc.Account = strings.TrimSpace(head), strings.TrimSpace(tail)
		}
		if v := params[4]; len(v) > 0 {
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
		if v := params[5]; len(v) > 0 {
			switch v[0].varint {
			case 1:
				acc.Digits = 6
			case 2:
				acc.Digits = 8
			}
		}
		if v := params[6]; len(v) > 0 && v[0].varint == 1 {
			acc.OTPType = otp.TypeHOTP
		}
		if v := params[7]; len(v) > 0 {
			c := int64(v[0].varint)
			acc.Counter = &c
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
			// Compared as unsigned: a huge length must not wrap to a negative int.
			if n == 0 || l > uint64(len(data)-i-n) {
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
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err == nil {
			return n
		}
	}
	return def
}
