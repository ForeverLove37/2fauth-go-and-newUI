# API compatibility matrix

Reference: 2FAuth **v9.0.0** (`routes/web.php`, `routes/api/v1.php`, controllers and
JSON resources of the PHP implementation). Status legend:

* ✅ implemented and exercised against the shipped Vue front-end
* 🟡 implemented with a known simplification
* ❌ not implemented (endpoint kept, answers `501`)

## Front-end routes (additions, outside the PHP contract)

| Route | Status | Notes |
| --- | --- | --- |
| `GET /` (React panel) | ➕ | index of `UI_ASSETS_PATH`; falls back to the Vue SPA when not deployed |
| `GET /assets/{file}` | ➕ | hashed React bundles, `immutable` cache |
| `GET /legacy`, `/legacy/*` | ➕ | the upstream Vue SPA, same routes as PHP `GET /` |
| `GET /favicon.svg` | ➕ | panel icon |
| `GET /api/v1/frontend-config` | ➕ | public JSON bootstrap used by the panel (`appName`, `appVersion`, `subdirectory`, `isDemoApp`, `sso`, `appSettings`, `features`). Purely additive: the Vue SPA never calls it. |

## Single sign-on (additions, outside the PHP contract)

OpenID Connect is implemented as a generic authorization-code flow with PKCE and
discovery. Configuration lives in `OIDC_*` environment variables (deployment
defaults) and in admin-editable `openid.*` runtime settings, which take
precedence; the client secret is stored encrypted (`v1:` + payload) and never
returned by any endpoint.

| Route | Status | Notes |
| --- | --- | --- |
| `GET /api/v1/sso/config` | ➕ | public: effective enablement, label, `auto_redirect`/`auto_register`, issuer, `redirect_uri`, `start_url`, discovery endpoints, human readable `error` when the issuer cannot be discovered. Never exposes the client secret. |
| `GET /socialite/redirect/openid` | ➕ | starts the flow: encrypted `oidc_tx` cookie (`state`, `nonce`, PKCE verifier), 302 to the provider; `?format=json` answers `{"url": …}` |
| `GET /socialite/callback/openid` | ➕ | validates `state`, exchanges the code, resolves the account (identity → same e-mail → auto-registration), rotates the session, logs the authentication; failures 302 to `/login?sso_error=<cancelled\|state\|provider\|exchange\|userinfo\|email\|register_disabled\|account\|server>` |
| `GET /api/v1/sso/settings` | ➕ | admin: `enabled`, `issuer`, `client_id`, `client_secret_set`, `scopes`, `label`, `auto_register`, `auto_redirect`, `redirect_uri` |
| `PUT /api/v1/sso/settings` | ➕ | admin: partial update (`enabled`, `issuer`, `client_id`, `client_secret`, `clear_secret`, `scopes`, `label`, `auto_register`, `auto_redirect`), 422 on an invalid issuer |
| `POST /api/v1/sso/test` | ➕ | admin: runs discovery, `{"ok":true,"discovery":…}` or `422 {"ok":false,"error":…}` |
| `GET /api/v1/sso/presets` | ➕ | admin: static catalogue (keycloak, authentik, auth0, okta, google, entra, gitlab, authelia) |
| `GET /api/v1/sso/keycloak-client` | ➕ | admin: importable Keycloak client (`format=client\|partial\|cli`), generates and persists a 40-character client secret when none is stored |

`openid.*` options are hidden from the generic `GET /api/v1/settings` listing and
`GET /api/v1/settings/{name}`, which keep serving only the upstream setting names.

## Public

| Route | Status | Notes |
| --- | --- | --- |
| `GET /` (SPA + globals) | ✅ | `appSettings`, `appConfig`, `urls`, `defaultPreferences`, `lockedPreferences`, `appVersion`, `isDemoApp`, `isTestingApp`, `appLocales`, `csrf-token` meta tag |
| `POST /user` (register) | ✅ | 403 when `disableRegistration`; first user becomes admin; returns `authenticatedPayload` |
| `POST /user/login` | ✅ | session rotation, auth log, 429 after `LOGIN_THROTTLE` attempts |
| `GET /user/logout` | ✅ | closes the auth log, drops the session |
| `POST /user/password/lost` | 🟡 | validates the payload and answers like Laravel, but sends no e-mail |
| `POST /user/password/reset` | 🟡 | answers success, does not reset the password |
| `GET /refresh-csrf` | ✅ | returns the token as a JSON string |
| `GET /manifest.json` | ✅ | |
| `GET /up` | ✅ | static health page |
| `GET /socialite/{redirect,callback}/{driver}` | ✅ | OpenID Connect only (see the SSO table above); any other driver, including GitHub OAuth, still answers `501` |
| `POST /webauthn/login/options`, `POST /webauthn/login` | ❌ | 501 |
| `POST /webauthn/lost`, `POST /webauthn/recover` | ❌ | 501 |

## Authenticated user

| Route | Status | Notes |
| --- | --- | --- |
| `GET /api/v1/user` | ✅ | `UserResource` shape |
| `PUT /user` | ✅ | password confirmation + uniqueness checks |
| `PATCH /user/password` | ✅ | `currentPassword` + confirmation |
| `DELETE /user` | ✅ | refuses to delete the only administrator |
| `GET|POST|DELETE /oauth/personal-access-tokens` | ✅ | opaque tokens stored as SHA-256 hashes, accepted as `Authorization: Bearer`; no OAuth2 dance |
| `POST /webauthn/register/options`, `POST /webauthn/register` | ❌ | 501 |
| `GET /webauthn/credentials` | ✅ | list of registered credentials |
| `PATCH /webauthn/credentials/{id}/name`, `DELETE /webauthn/credentials/{id}` | ✅ | rename / delete |
| `GET /api/v1/user/preferences` | ✅ | array of `{key, value, locked}` |
| `GET|PUT /api/v1/user/preferences/{name}` | ✅ | 404 unknown, 403 locked |
| `GET /api/v1/user/name` | ✅ | deprecated marker |

## 2FA accounts

| Route | Status | Notes |
| --- | --- | --- |
| `GET /api/v1/twofaccounts` | ✅ | `ids`, `withOtp`, `at`, `withSecret`; per-user ordering |
| `POST /api/v1/twofaccounts` | ✅ | advanced form or `uri`; validation messages and 422 shape mirror Laravel |
| `GET /api/v1/twofaccounts/{id}` | ✅ | secret visible to the owner only |
| `PUT|PATCH /api/v1/twofaccounts/{id}` | ✅ | owner only |
| `DELETE /api/v1/twofaccounts/{id}` | ✅ | 204 |
| `DELETE /api/v1/twofaccounts?ids=` | ✅ | 400 when more than 99 ids |
| `PATCH /api/v1/twofaccounts/withdraw?ids=` | ✅ | |
| `POST /api/v1/twofaccounts/reorder` | ✅ | persisted in `twofaccount_user_orders` |
| `PATCH /api/v1/twofaccounts/{id}/favorite` | ✅ | |
| `PATCH /api/v1/twofaccounts/{id}/owner` | ✅ | ownership transfer |
| `GET /api/v1/twofaccounts/count` | ✅ | `{owned, shared, total}` |
| `GET /api/v1/twofaccounts/{id}/otp` | ✅ | TOTP `{password, otp_type, generated_at, period, next_password}`, HOTP `{password, otp_type, counter}` |
| `POST /api/v1/twofaccounts/otp` | ✅ | generates without persisting |
| `GET /api/v1/twofaccounts/{id}/qrcode` | ✅ | PNG data URI |
| `POST /api/v1/qrcode/decode` | ✅ | multipart `qrcode`, decoded with `gozxing` |
| `GET /api/v1/twofaccounts/export` | ✅ | 2FAuth JSON envelope and `otpauth` list |
| `POST /api/v1/twofaccounts/preview` | ✅ | |
| `POST /api/v1/twofaccounts/migration` | ✅ | 2FAuth JSON, Aegis, 2FAS, Google migration URI, plain lists; duplicates flagged `-1`, new accounts `-2` |
| `GET /api/v1/twofaccounts/{id}/otp-logs` | ✅ | records requester/owner/ip/type/counter |

## Groups and sharing

| Route | Status | Notes |
| --- | --- | --- |
| `GET|POST /api/v1/groups` | ✅ | `GroupResource` |
| `GET|PUT|PATCH|DELETE /api/v1/groups/{id}` | ✅ | |
| `GET /api/v1/groups/{id}/twofaccounts` | ✅ | |
| `POST /api/v1/groups/{id}/assign` | ✅ | |
| `POST /api/v1/groups/reorder` | ✅ | |
| `GET|POST|DELETE /api/v1/twofaccounts/{id}/shares` | ✅ | 201 when at least one share was created |
| `POST /api/v1/twofaccounts/{id}/shares/all` | ✅ | requires `enableAllUsersSharingScope` |
| `DELETE /api/v1/twofaccounts/{id}/shares/{user}` | ✅ | |
| `GET /api/v1/twofaccounts/{id}/recipients` | ✅ | `filter[nameOrEmail]` |

## Icons and features

| Route | Status | Notes |
| --- | --- | --- |
| `GET /api/v1/icons/packs` | ✅ | static pack list: every provider with its `id`, `label`, `url` (home page) and `template` (URL template); `name` is kept for the legacy front-ends |
| `POST /api/v1/icons/default` | 🟡 | resolves the service name through the ordered **provider chain** (see “Icon providers” below) and returns `{"icon", "url", "provider"}`; a total miss answers `404 {"message": "Icon not found for this service"}` instead of Laravel's exception payload |
| `POST /api/v1/icons` | ✅ | multipart upload, file stored in `storage/icons` |
| `DELETE /api/v1/icons/{icon}` | ✅ | |
| `GET /storage/icons/{file}` | ✅ | served with a long cache header |
| `GET /api/v1/features`, `/features/{feature}` | ✅ | `enabled`/`disabled` per setting |

## Icon providers (additions, outside the PHP contract)

`POST /api/v1/icons/default` no longer hard-codes a single icon set: it walks an
ordered, admin-editable provider chain and stores the first image that passes the
content sniffing (`http.DetectContentType` **plus** an `<svg…></svg>` structural
check, because Go reports SVG as `text/xml`/`text/plain`). A provider misses on a
transport error, a non-200 status, an empty body or a body that is neither an SVG
nor a PNG/JPEG/WebP/GIF image. Each attempt is limited to 6 s, the whole chain to
15 s, and a partially downloaded file is never written.

`iconProviders` (comma separated ids, default `selfhst,dashboard,walkx,simpleicons,lobehub,iconify`)
and `iconProviderMaxTries` (1-6, default `3`, only the first N *enabled*
providers are attempted) are ordinary settings, seeded by `ICON_PROVIDERS` /
`ICON_PROVIDER_MAX_TRIES`; a value stored through the API or the settings
endpoints wins over the environment. The chain is also readable/writable through
the dedicated endpoints:

| Route | Status | Notes |
| --- | --- | --- |
| `GET /api/v1/icons/providers` | ➕ | authenticated: `{"providers": [{id,label,source,template,enabled,order,status}], "max_tries": 3, "all": [{id,label,source,template}]}`. `status` is `available` (enabled and within `max_tries`), `skipped` (enabled but beyond `max_tries`) or `disabled`; `order` is the 1-based position in the configured chain (0 when disabled), the array is returned enabled-first in chain order. No network call is made. |
| `PUT /api/v1/icons/providers` | ➕ | admin: body `{"providers": ["selfhst","iconify"], "max_tries": 2}` (a comma separated string is accepted too, and `max_tries` is optional — omitting it keeps the current value); ids are validated against the known set, length 1-6, `max_tries` 1-6, otherwise `422 {"message","errors"}`. Answers the same payload as `GET`. |

The providers, in their default order:

| id | label | URL template |
| --- | --- | --- |
| `selfhst` | selfh.st | `https://cdn.jsdelivr.net/gh/selfhst/icons/svg/{slug}.svg` |
| `dashboard` | Homarr dashboard-icons | `https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons/svg/{slug}.svg` |
| `walkx` | walkxcode legacy | `https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/svg/{slug}.svg` |
| `simpleicons` | Simple Icons | `https://cdn.simpleicons.org/{slug}` (alphanumerics only) |
| `lobehub` | LobeHub icons | `https://cdn.jsdelivr.net/gh/lobehub/lobe-icons/packages/static-png/{slug}.png` (`-color` retry) |
| `iconify` | Iconify search | `https://api.iconify.design/search?query={service}&limit=1` → `https://api.iconify.design/{prefix}/{name}.svg` |

`{slug}` is the lower-cased service name with spaces/underscores turned into
dashes and everything outside `[a-z0-9.-]` removed (`Grafana Cloud` →
`grafana-cloud`, `Proxmox VE` → `proxmox-ve`); names that yield no slug, or that
carry markup characters, answer the usual “not found”. `dashboard` additionally
retries the original casing variant.

## Administration

| Route | Status | Notes |
| --- | --- | --- |
| `GET|POST /api/v1/users`, `GET|DELETE /api/v1/users/{id}` | ✅ | wrapped in `{"info": …}` like `UserManagerResource` |
| `PATCH /api/v1/users/{id}/promote` | ✅ | |
| `PATCH /api/v1/users/{id}/password/reset` | 🟡 | returns a new random password; no e-mail |
| `DELETE /api/v1/users/{id}/pats`, `/credentials` | ✅ | |
| `GET /api/v1/users/{id}/authentications` | ✅ | login/logout sessions with basic user-agent parsing |
| `GET|POST /api/v1/settings`, `GET|PUT|DELETE /api/v1/settings/{name}` | ✅ | `{{1}}`/`{{}}` boolean encoding preserved |
| `GET /system/infos` | 🟡 | PHP-specific rows replaced by Go equivalents |
| `POST /system/test-email` | 🟡 | 424 with a "no transport" detail |
| `GET /system/latestRelease` | ✅ | GitHub release lookup, stores `latestRelease`/`lastRadarScan` |
| `GET /system/optimize`, `/system/clear-cache` | 🟡 | no-op, all exit codes 0 |

## Behavioural notes

* **Sessions** are encrypted cookies (`2fauth_session`) carrying the user id, a CSRF
  token and the last activity timestamp. Unsafe methods require `X-XSRF-TOKEN`,
  otherwise the server answers **419** (Laravel semantics). Idle sessions answer
  **418** on API calls so the SPA logs out by itself.
* **CSRF cookie** (`XSRF-TOKEN`) is readable by JavaScript, exactly like Laravel's.
* **Passport** is reduced to personal access tokens: random 40-character secrets,
  SHA-256 hashed at rest, string primary keys.
* **Encryption** of OTP secrets matches Laravel's `encryptString` (AES-256-CBC with
  HMAC, `{iv, value, mac, tag}` JSON payload), so an existing database stays readable
  with the same `APP_KEY`.
* **Timestamps** are stored as `YYYY-MM-DD HH:MM:SS` UTC strings (SQLite) and as
  native timestamp columns on MySQL/PostgreSQL.
* **Throttling** is in-process (sliding window), not shared between instances.
* **OTP values** are computed locally; no external service is involved.
* **Icons** are fetched from a configurable provider chain (see “Icon providers”);
  the HTTP client honours `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY`, certificate
  verification stays enabled and only a fully received, sniffed image is written to
  `storage/icons`.
* **SSO (OpenID Connect)** links external accounts through the `oauth_identities`
  table (`UNIQUE(provider, subject)`); the same identity always resolves to the
  same user, an existing account is matched by e-mail (case insensitive) and
  auto-registration honours `disableRegistration`. Provider documents are cached
  in memory for 30 minutes, outbound requests honour `HTTP_PROXY`/`HTTPS_PROXY`
  and certificate verification is never disabled.
* **Client secret at rest**: stored as `v1:<Laravel payload>` encrypted with
  `APP_KEY`, decrypted on use, and never included in an API response.

## Notes on the React panel

The panel consumes the same `/api/v1` contract as the Vue SPA, so every status in
this document applies to both front-ends: session cookie `2fauth_session` plus
`XSRF-TOKEN` → `X-XSRF-TOKEN` on unsafe methods, `419` retried once through
`GET /refresh-csrf`, `418`/`401` redirect to the login screen, `{"data": …}` and
`{"info": …}` envelopes unwrapped client-side.

`GET /webauthn/credentials` is backed by the Laravel `webauthn_credentials` table
(`id` is the credential identifier string, `alias` the display name), so the
panel can list and delete credentials even though registration itself answers
`501`.
