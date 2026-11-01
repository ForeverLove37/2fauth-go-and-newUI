# 2FAuth (Go) — a PHP-free 2FAuth

A from-scratch Go implementation of the [2FAuth](https://github.com/Bubka/2FAuth) v9.0.0
backend. It serves the **unmodified upstream Vue 3 front-end** and the original
`/api/v1` JSON contract, so the shipped 2FAuth UI works as-is: no PHP, no Composer,
no Laravel, no `artisan`.

```
┌────────────────────────────────┐        ┌────────────────────────────────────┐
│ 3D panel (React 18 · R3F · TS) │  HTTP  │ 2fauth (Go)                        │
│ ``2fauth-ui`` → /ui            │◄──────►│  /api/v1 · session · CSRF · PATs   │
├────────────────────────────────┤        │  SQLite / MySQL / PostgreSQL       │
│ legacy SPA (Vue 3, compiled)   │        │                                    │
│ ``public/build`` → /legacy     │        │                                    │
└────────────────────────────────┘        └────────────────────────────────────┘
```

Two front-ends are served by the same binary:

* **`/`** — the modern **React + TypeScript + React Three Fiber** panel in
  [`../2fauth-ui`](../2fauth-ui). Accounts orbit on a 3D ring, the focused card
  shows a large code with a countdown ring; drag to rotate, wheel/←→ to switch,
  `C` to copy. Includes account CRUD with QR decoding, import/export, groups,
  sharing, preferences, personal access tokens, passkey listing and the admin
  console.
* **`/legacy`** — the unmodified upstream Vue 3 SPA (`public/build`), kept as a
  reference and fallback whenever the React bundle is not deployed.

## Why

The upstream project needs PHP 8.4 + Composer + a full Laravel install to run.
This rewrite keeps the front-end and the REST contract but replaces the server
with a single static binary (~29 MB, no runtime dependency, no cgo).

## Features

Implemented and verified end to end against the shipped SPA:

| Area | Details |
| --- | --- |
| Accounts | create / read / update / delete, batch delete, withdraw from group, reorder, favourites, transfer of ownership, count, preview from `otpauth://` URI, QR code rendering, export (2FAuth JSON and `otpauth` list) |
| OTP | TOTP (RFC 6238), HOTP (RFC 4226, counter persisted), Steam TOTP; sha1/sha256/sha512/md5; 5-10 digits; custom period/counter; `otp` object embedded in list/show responses with `withOtp` |
| Import | 2FAuth JSON (schema 1), Aegis JSON, 2FAS JSON, Google Authenticator `otpauth-migration://` (protobuf decoded in-house), plain `otpauth://` lists, `issuer:account:secret` lists, duplicate detection |
| Groups | CRUD, ordering, `show_in_chips`, assign accounts, per-group listing |
| Sharing | per-user shares, `is_borrowed`/`borrowed_by`, share with all users scope, recipients listing, borrowed OTP generation, OTP generation logs |
| Users & auth | register (first user is admin), login, logout, profile update, password change, session cookie + `XSRF-TOKEN`/`X-XSRF-TOKEN` (419 on mismatch), 418 on idle, login throttling (429), personal access tokens (`Authorization: Bearer`, stored hashed) |
| Administration | user list/show/create/delete, promote/demote, password reset, revoke PATs and credentials, authentication logs, app settings CRUD, system infos, release radar, cache/optimize stubs |
| Icons | upload, delete, serve from `/storage/icons/*`, default-icon fetch, icon packs listing |
| Storage | SQLite (pure Go `modernc.org/sqlite`, default), MySQL and PostgreSQL through the same schema |

### Front-ends

| Path | Content | Notes |
| --- | --- | --- |
| `/` | React panel (`UI_ASSETS_PATH`, default `<base>/ui`) | served when that directory contains an `index.html` |
| `/assets/*` | hashed React bundles | immutable cache headers |
| `/legacy`, `/legacy/*` | Vue 3 SPA from `ASSETS_PATH` | full upstream UI, same API |
| `/build/*`, `/storage/*` | legacy SPA assets and icons | unchanged |
| `/api/v1/frontend-config` | public bootstrap for the React panel | `{appName, appVersion, subdirectory, isDemoApp, sso, appSettings, features}` |

### Not implemented

These endpoints exist but answer `501 Not Implemented` (or a stub) instead of
performing work:

* **WebAuthn / passkeys** — `/webauthn/*`. Credential management endpoints work,
  registration and assertion are stubs.
* **SSO** — `/socialite/*`. No OpenID Connect or GitHub OAuth flow.
* **E-mail** — password reset mails and the admin "test e-mail" button do not send
  anything; `/user/password/lost` and `/user/password/reset` answer success
  without side effects.

See [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) for the endpoint-by-endpoint
status.

## Build

Requires Go 1.26+.

```bash
cd 2fauth-go
go build -o 2fauth ./cmd/2fauth
```

The React panel is a separate Vite project:

```bash
cd 2fauth-ui
npm install
npm run build            # tsc -b && vite build  →  dist/
```

Point the binary at the bundle with `UI_ASSETS_PATH=/path/to/dist` (default
`<APP_BASE_PATH>/ui`). Without that directory the root URL falls back to the Vue
SPA, so a Go-only deployment keeps working.

Pure Go dependencies only, so `CGO_ENABLED=0` cross-compilation works:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o 2fauth ./cmd/2fauth
```

## Run

1. Copy the compiled front-end next to the binary (it is committed in the
   upstream repository, so no Node.js is needed):

   ```bash
   git clone --depth 1 --branch v9.0.0 https://github.com/Bubka/2FAuth.git
   mkdir -p /srv/2fauth
   cp -r 2FAuth/public /srv/2fauth/public
   ```

   `ASSETS_PATH` may point anywhere (`public/` of upstream v9 is all that is needed).

2. Configure:

   ```bash
   cd /srv/2fauth
   cp /path/to/2fauth-go/.env.example .env
   /path/to/2fauth key:generate     # writes APP_KEY into .env
   ```

3. Start:

   ```bash
   APP_BASE_PATH=/srv/2fauth ./2fauth
   # 2FAuth 9.0.0 (Go) listening on :8000
   ```

   `PORT`/`LISTEN_ADDR` override the listening address, `APP_BASE_PATH` the working
   directory holding `.env`, `database/`, `storage/`.

4. Open `http://localhost:8000` and register: the first account becomes the
   administrator.

### Docker-ish layout

```
/srv/2fauth
├── .env
├── 2fauth            # the Go binary
├── public/           # upstream front-end (public/ of 2FAuth v9)
├── database/
│   └── database.sqlite
└── storage/
    └── icons/
```

## Migrating from the PHP version

The schema mirrors the Laravel migrations (same table and column names), and the
encryption format matches Laravel's `APP_KEY`/`encryptString` payload. Two paths:

* **Fresh start (recommended)** — run the Go server, then import your PHP-2FAuth
  JSON export (`Import ▸ 2FAuth (JSON)`). Accounts, groups and secrets are re-created.
* **Reuse the existing SQLite file** — point `DB_DATABASE` at the copy of
  `database/database.sqlite` produced by the PHP install. Keep the same `APP_KEY`,
  and note that PHP-encrypted `secret` columns stay readable because the crypto
  format is identical. Tables added by this implementation are created on start-up.

## Development

```bash
go test ./...   # crypto/OTP vectors + HTTP integration tests (temp SQLite database)
go vet ./...
gofmt -l .
```

The test suite covers Laravel-compatible decryption of a reference ciphertext, the
RFC 4226/6238 vectors (sha1/sha256/sha512), Steam codes, `otpauth://` round trips and
the full register → login → create account → OTP flow over HTTP, including CSRF (419),
bearer tokens and validation payloads.

Layout:

```
cmd/2fauth            entrypoint, CLI (key:generate, migrate, optimize…)
internal/config       .env + environment loader, preference/setting defaults
internal/crypt        Laravel-compatible AES-256-CBC encrypter, bcrypt, random helpers
internal/db           connection, dialect handling, schema, additive migrations
internal/store        data access (users, accounts, groups, shares, tokens, logs)
internal/services     settings service (boolean/typed serialisation), icon store
internal/otp          TOTP/HOTP/Steam generators, base32, otpauth URI parser/builder
internal/httpapi      router, session/CSRF, validators, JSON resources, handlers
internal/httpapi/ui.go  React panel serving (/ui, /assets, /legacy dispatch, frontend-config)
```

The panel itself lives in `../2fauth-ui`:

```
src/api        typed fetch client (CSRF, 419 retry, Bearer PAT) and DTOs
src/store      zustand stores (session, accounts, groups, preferences, toasts)
src/three      React Three Fiber backdrop (particles, wireframe core, grid)
src/components orbital ring, account card, modals (account, transfer, groups), shell
src/views      Auth (login/register), Dashboard (orbit + list), Settings (options/account/security/admin)
```

The router is a small hand-written matcher (literal segments beat `{param}`, which
beats a `{rest...}` catch-all) with per-route auth/admin requirements, so response
shapes can be kept byte-compatible with Laravel's `JsonResource` wrappers
(`{"data": …}`, `{"info": …}` for the admin user manager).

## License

The upstream 2FAuth front-end and this reimplementation of its API are distributed
under the **AGPL-3.0**, like the original project.
