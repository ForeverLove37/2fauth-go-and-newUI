# 2FAuth — 3D control panel

React + TypeScript + React Three Fiber front-end for the Go 2FAuth backend in
[`../2fauth-go`](../2fauth-go). It replaces the Vue SPA at `/`; the original Vue
app stays reachable at `/legacy`.

## Appearance & display modes

* **15 预设主题** (深空霓虹 / 极夜蓝 / 冰川薄荷 / 赛博紫 / 日落琥珀 / 森林苔藓 / 玫瑰石英 / 蒸汽波 / 深海青 / 岩浆橙 / 石墨单色 / 极简白 / 晨雾灰 / 亚麻米色 / 薄荷纸感) plus a full appearance editor in `Settings → 外观`: background glows, panel opacity, border, radius, glass blur, fonts, code size, accent and status colours, motion switch and duration. Every token is compiled to CSS variables by `src/theme/apply.ts`, so the whole UI (including portals/modals) follows immediately.
* **命名配置档案**: save the current look, apply/rename/delete it, and export/import the theme as JSON.
* **显示模式** (switchable, persisted in `localStorage`):
  | 模式 | 说明 |
  | --- | --- |
  | 环绕 / 螺旋 / 卡墙 / 球面 | 纯 3D 舞台，卡片在空间中投影，**每张卡片都实时显示自己的验证码** |
  | 网格 / 列表 | 纯 2D |
* **3D 自由度**: drag to rotate on **both axes** (X and Y), inertia, wheel zoom (0.45×–2.2×), double-click reset, auto-rotate toggle (`R`). Cards are billboarded — they stay readable while the scene spins.
* **动态分辨率 (LOD)**: every card is laid out at the size it will occupy on screen (0.8× / 1× / 1.2× / 1.4× / 1.6×) and the transform only scales the remainder (0.74–1.06×), so cards closest to the camera stay sharp instead of being stretched. Cards render their code and buttons natively, so copying and editing work in 3D as well.
* **网格拖拽排序**: in grid mode, press and drag a card to reorder the panel like a phone home screen; the new order is persisted with `POST /api/v1/twofaccounts/reorder`.
* **背景场景**: 7 scenery styles — `grid` 网格线 / `stars` 星空 / `nebula` 星云 / `waves` 波纹 / `aurora` 极光 (pure CSS) / `bokeh` 光斑 / `minimal` 极简 — each theme preset ships its own, and brightness + speed are adjustable.
* **多语言**: English source ids with a 简体中文 catalogue (`src/i18n/`), 繁體中文 falls back to it; the language can be picked on the sign-in screen or in `Settings → 选项` and is stored as the server-side `lang` preference.

## Stack

| Layer | Choice |
| --- | --- |
| UI | React 18, React Router 6, TypeScript 5 (strict) |
| 3D | React Three Fiber 8, drei 9, three r169 — particle field, wireframe core, halo rings, floating shards, infinite grid |
| Stage | custom projection (`src/stage/`): 3D layout math + perspective projection to DOM transforms, all cards billboarded, WebGL backdrop behind |
| State | zustand (session, accounts, groups, preferences, toasts) |
| Build | Vite 5 |

## Scripts

```bash
npm install
npm run dev        # Vite dev server on :5173, proxy-free: it talks to /api/v1 on the same origin
npm run typecheck  # tsc --noEmit
npm run build      # tsc -b && vite build → dist/
npm run smoke      # jsdom smoke test: display modes, LOD, reorder grid, scenery, i18n, SSO, login
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
src/three/Backdrop.tsx  R3F canvas: per-style scenery (grid, starfield, nebula, waves, bokeh) + CSS aurora/minimal
src/i18n/               message catalogs: English ids, zh-CN dictionary, locale store, useT() hook
src/stage/layouts.ts    orbit / helix / wall / sphere placement + projection
src/stage/Stage3D.tsx   LOD-based resolution, free X/Y rotation, inertia, wheel zoom, auto-rotate, double-click reset
src/theme/              15 presets, token→CSS-variable compiler, zustand theme store with profiles
src/api/sso.ts          OIDC config / start / admin settings / Keycloak export client
src/components/
  AccountCard.tsx       glass card: icon, labels, flags, big OTP, countdown ring, next code, actions
  SsoSettingsPanel.tsx  issuer/client/secret/scopes, connection test, Keycloak JSON export
  IconProvidersPanel.tsx admin: icon library order, enable/disable, how many libraries to try
  ThemeEditor.tsx       (in src/views) preset swatches, token editors, live preview, profiles
  AccountModal.tsx      create/edit, otpauth URI parsing, QR image decode, icon upload, sharing, delete
  Transfer.tsx          import (JSON / otpauth / migration URI / QR image) and export (JSON / otpauth)
  Shell.tsx             top bar, navigation, theme cycling, toasts
  Icon.tsx              inline stroke icons, no icon dependency
src/views/Auth.tsx      login + register, SSO button and sso_error handling
src/views/Dashboard.tsx 7 display modes, group chips, search, focus bar, shortcuts (←→ c f r e h n / g)
src/views/Settings.tsx  appearance · options · account · security (PATs, passkeys) · SSO · administration
src/views/ThemeEditor.tsx  appearance centre
scripts/smoke.ts        jsdom smoke test: every display mode, theme editor, login (npm run smoke)
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
| `R` | toggle auto-rotate |
| `/` | focus search |
| `Esc` | close a dialog |

## Behaviour

* Codes refresh every 5 s while the tab is visible (`usePolling`) and the countdown
  ring is derived from `otp.generated_at` + `period`, so it stays in sync with the server.
* `showOtpAsDot`, `showNextOtp` and `theme` come from user preferences and are written
  through `PUT /api/v1/user/preferences/{key}`.
* Deleting, sharing and importing reuse the same endpoints as the Vue SPA, so both
  front-ends share one database and one feature set.
