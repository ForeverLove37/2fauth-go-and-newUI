# 2FAuth — 3D control panel

React + TypeScript + React Three Fiber front-end for the Go 2FAuth backend in
[`../2fauth-go`](../2fauth-go). It replaces the Vue SPA at `/`; the original Vue
app stays reachable at `/legacy`.

## Stack

| Layer | Choice |
| --- | --- |
| UI | React 18, React Router 6, TypeScript 5 (strict) |
| 3D | React Three Fiber 8, drei 9, three r169 — particle field, wireframe core, halo rings, floating shards, infinite grid |
| Orbit | CSS `preserve-3d` ring (crisp DOM cards driven by `--ry`, `--i`, `--step`, `--radius`) layered over the WebGL backdrop |
| State | zustand (session, accounts, groups, preferences, toasts) |
| Build | Vite 5 |

## Scripts

```bash
npm install
npm run dev        # Vite dev server on :5173, proxy-free: it talks to /api/v1 on the same origin
npm run typecheck  # tsc --noEmit
npm run build      # tsc -b && vite build → dist/
```

Deploy by copying `dist/` where the Go binary expects it:

```bash
rm -rf ../run/ui && cp -r dist ../run/ui      # or set UI_ASSETS_PATH=/path/to/dist
```

## Layout

```
src/api/client.ts       typed fetch client: XSRF cookie → X-XSRF-TOKEN, 419 retry via /refresh-csrf,
                        401/418 → session-expired event, Bearer PAT support, envelope unwrapping
src/api/types.ts        DTOs mirroring the /api/v1 JSON resources
src/store/useStore.ts   zustand store: boot, login/register/logout, accounts, groups, preferences, toasts
src/three/Backdrop.tsx  R3F canvas: 520 particles, icosahedron wireframe, torus halo, octahedron shards, Grid
src/components/
  OrbitRing.tsx         drag/wheel/snap orbit, keyboard owned by the dashboard
  AccountCard.tsx       glass card: icon, labels, flags, big OTP, countdown ring, next code, actions
  AccountModal.tsx      create/edit, otpauth URI parsing, QR image decode, icon upload, sharing, delete
  Transfer.tsx          import (JSON / otpauth / migration URI / QR image) and export (JSON / otpauth)
  Shell.tsx             top bar, navigation, theme cycling, toasts
  Icon.tsx              inline stroke icons, no icon dependency
src/views/Auth.tsx      login + register (registration respected via frontend-config)
src/views/Dashboard.tsx orbit/list views, group chips, search, focus bar, shortcuts (←→ c f e h n / g)
src/views/Settings.tsx  options · account · security (PATs, passkeys) · administration (settings, users)
```

## Shortcuts

| Key | Action |
| --- | --- |
| `←` `→` | rotate the orbit / previous, next account |
| `C` | copy the focused code |
| `H` | generate the next HOTP code |
| `F` | pin/unpin |
| `E` | edit the focused account |
| `N` | new account |
| `G` | groups |
| `/` | focus search |
| `Esc` | close a dialog |

## Behaviour

* Codes refresh every 5 s while the tab is visible (`usePolling`) and the countdown
  ring is derived from `otp.generated_at` + `period`, so it stays in sync with the server.
* `showOtpAsDot`, `showNextOtp` and `theme` come from user preferences and are written
  through `PUT /api/v1/user/preferences/{key}`.
* Deleting, sharing and importing reuse the same endpoints as the Vue SPA, so both
  front-ends share one database and one feature set.
