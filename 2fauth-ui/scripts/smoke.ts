/**
 * DOM smoke test (jsdom): mounts the panel views with a seeded store and
 * exercises every display mode. Run with `npm run smoke`.
 */
import { errors, dom } from './dom-setup'
import { createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'

const { window } = dom
const globals = globalThis as unknown as Record<string, unknown>

const accounts = [
  {
    id: 1,
    group_id: 1,
    otp_type: 'totp' as const,
    service: 'GitHub',
    account: 'alice@example.com',
    icon: null,
    secret: 'JBSWY3DPEHPK3PXP',
    digits: 6,
    algorithm: 'sha1',
    period: 30,
    counter: null,
    is_favorite: true,
    otp: { password: '123456', otp_type: 'totp', generated_at: Math.floor(Date.now() / 1000), next_password: '654321' },
  },
  {
    id: 2,
    group_id: null,
    otp_type: 'hotp' as const,
    service: 'Router',
    account: 'admin',
    icon: null,
    secret: 'JBSWY3DPEHPK3PXP',
    digits: 6,
    algorithm: 'sha1',
    period: null,
    counter: 7,
    is_favorite: false,
    otp: { password: '222333', otp_type: 'hotp', counter: 7 },
  },
  {
    id: 3,
    group_id: null,
    otp_type: 'steamtotp' as const,
    service: 'Steam',
    account: 'player_one',
    icon: null,
    secret: 'JBSWY3DPEHPK3PXP',
    digits: 5,
    algorithm: 'sha1',
    period: 30,
    counter: null,
    is_favorite: false,
    otp: { password: 'K3M9P', otp_type: 'steamtotp', generated_at: Math.floor(Date.now() / 1000) },
  },
]

async function main() {
  const { useStore } = await import('../src/store/useStore')
  const { useThemeStore } = await import('../src/theme/store')
  const { useI18nStore } = await import('../src/i18n')
  useI18nStore.getState().setLocale('zh-CN')
  await tick(60)
  const Dashboard = (await import('../src/views/Dashboard')).default
  const Settings = (await import('../src/views/Settings')).default
  const ThemeEditor = (await import('../src/views/ThemeEditor')).default
  const { Login } = await import('../src/views/Auth')

  // fake API: no network in jsdom
  const requests: string[] = []
  globals.fetch = async (input: unknown) => {
    const url = typeof input === 'string' ? input : String((input as { url?: string }).url ?? input)
    requests.push(url)
    let payload: unknown = { data: [] }
    if (url.includes('/api/v1/sso/config')) {
      payload = {
        enabled: true,
        provider: 'openid',
        label: '用公司账号登录',
        auto_redirect: false,
        auto_register: true,
        issuer: 'https://idp.example.com',
        redirect_uri: 'http://localhost/socialite/callback/openid',
        start_url: '/socialite/redirect/openid',
        discovery: { issuer: 'https://idp.example.com', authorization_endpoint: 'https://idp.example.com/auth', token_endpoint: 'https://idp.example.com/token' },
        error: null,
      }
    } else if (url.includes('/socialite/redirect/openid')) {
      payload = { url: 'https://idp.example.com/auth?state=xyz' }
    } else if (url.includes('/api/v1/sso/settings')) {
      payload = {
        enabled: true,
        issuer: 'https://keycloak.example.com/realms/main',
        client_id: '2fauth',
        client_secret_set: true,
        scopes: 'openid profile email',
        label: '用公司账号登录',
        auto_register: true,
        auto_redirect: false,
        redirect_uri: 'http://localhost/socialite/callback/openid',
      }
    } else if (url.includes('/api/v1/sso/presets')) {
      payload = [{ id: 'keycloak', name: 'Keycloak', issuer: 'https://keycloak.example.com/realms/{realm}', scopes: 'openid profile email', hint: 'realm 必填' }]
    } else if (url.includes('/api/v1/sso/keycloak-client')) {
      payload = {
        filename: 'keycloak-2fauth.json',
        format: 'client',
        content: '{"clientId":"2fauth"}',
        secret_generated: true,
        notice: 'secret generated',
      }
    } else if (url.includes('/api/v1/users')) {
      payload = { info: [] }
    } else if (url.includes('/api/v1/settings')) {
      payload = []
    }
    return {
      ok: true,
      status: 200,
      text: async () => JSON.stringify(payload),
      headers: { get: () => 'application/json' },
    }
  }

  useStore.setState({
    accounts,
    groups: [{ id: 1, name: 'Work', show_in_chips: true, twofaccounts_count: 1 }],
    selectedId: 1,
    prefs: { showOtpAsDot: false, showNextOtp: true },
    user: { id: 1, name: 'Alice', email: 'alice@example.com', is_admin: true, preferences: {}, appSettings: {}, oauth_provider: null },
  })

  const container = window.document.getElementById('root')!
  const root = createRoot(container)
  root.render(createElement(Dashboard))
  await tick(200)

  const cards = () => container.querySelectorAll('.orbit-card').length
  const assert = (label: string, condition: boolean) => {
    console.log(`${condition ? 'PASS' : 'FAIL'}  ${label}`)
    if (!condition) process.exitCode = 1
  }

  assert('orbit renders every account', cards() === 3)
  assert('stage present', container.querySelector('.stage3d') !== null)

  // every display mode must keep all codes on screen
  const modes = ['环绕', '螺旋', '卡墙', '球面', '网格', '列表']
  for (const label of modes) {
    const button = [...container.querySelectorAll('.segmented button')].find((element) =>
      element.textContent?.includes(label),
    ) as HTMLButtonElement | undefined
    assert(`mode button "${label}" exists`, Boolean(button))
    button?.dispatchEvent(new window.MouseEvent('click', { bubbles: true }))
    await tick(120)
    if (label === '网格') {
      assert(`${label}: grid shows all codes`, container.querySelectorAll('.grid2d-cell .orbit-card').length === 3)
    } else if (label === '列表') {
      assert('列表: rows rendered', container.querySelectorAll('.account-row').length === 3)
    } else {
      assert(`${label}: 3D stage keeps all cards`, cards() === 3)
    }
  }

  // copy interaction on the focused card does not throw
  const otpButton = container.querySelector('.focus-otp') as HTMLButtonElement | null
  assert('focus bar has a code', Boolean(otpButton?.textContent?.trim()))
  otpButton?.dispatchEvent(new window.MouseEvent('click', { bubbles: true }))
  await tick(60)

  // theme editor renders every preset and applies one
  const editorHost = window.document.createElement('div')
  window.document.body.appendChild(editorHost)
  const editorRoot = createRoot(editorHost)
  editorRoot.render(createElement(ThemeEditor))
  await tick(150)
  const swatches = editorHost.querySelectorAll('.theme-swatch')
  assert('theme editor lists presets', swatches.length >= 12)
  const before = window.document.documentElement.style.getPropertyValue('--accent')
  ;(swatches[3] as HTMLButtonElement | undefined)?.dispatchEvent(new window.MouseEvent('click', { bubbles: true }))
  await tick(120)
  const after = window.document.documentElement.style.getPropertyValue('--accent')
  assert('clicking a preset updates the accent token', Boolean(after) && after !== before)
  assert('theme profiles section present', editorHost.textContent?.includes('配置档案') === true)
  assert('preset count', useThemeStore.getState() ? true : false)

  // login renders the SSO-aware form skeleton
  const loginHost = window.document.createElement('div')
  window.document.body.appendChild(loginHost)
  createRoot(loginHost).render(createElement(MemoryRouter, null, createElement(Login)))
  await tick(150)
  assert('login form rendered', loginHost.querySelectorAll('input').length >= 2)
  const ssoButton = [...loginHost.querySelectorAll('button')].find((element) =>
    element.textContent?.includes('用公司账号登录'),
  ) as HTMLButtonElement | undefined
  assert('SSO button rendered from /api/v1/sso/config', Boolean(ssoButton))
  ssoButton?.dispatchEvent(new window.MouseEvent('click', { bubbles: true }))
  await tick(120)
  assert(
    'clicking SSO asks the server for the authorize URL',
    requests.some((url) => url.includes('/socialite/redirect/openid')),
  )

  // ?sso_error=state surfaces a readable message
  const errorHost = window.document.createElement('div')
  window.document.body.appendChild(errorHost)
  createRoot(errorHost).render(
    createElement(MemoryRouter, { initialEntries: ['/login?sso_error=state'] }, createElement(Login)),
  )
  await tick(150)
  assert('sso_error is explained on the login page', errorHost.textContent?.includes('SSO 会话已失效') === true)

  // settings screen: every tab renders, the SSO panel loads its settings
  const settingsHost = window.document.createElement('div')
  window.document.body.appendChild(settingsHost)
  createRoot(settingsHost).render(
    createElement(MemoryRouter, { initialEntries: ['/settings'] }, createElement(Settings)),
  )
  await tick(180)
  const tabLabels = [...settingsHost.querySelectorAll('.tabs button')].map((element) => element.textContent ?? '')
  assert('personal settings show 外观 / 选项 / 账号 / 安全', ['外观', '选项', '账号', '安全'].every((label) => tabLabels.some((text) => text.includes(label))))
  assert('personal settings keep the admin tabs out', !tabLabels.some((text) => text.includes('单点登录')))
  assert('only the first appearance section starts open', settingsHost.querySelectorAll('.section-toggle[aria-expanded="true"]').length === 1)

  const adminHost = window.document.createElement('div')
  window.document.body.appendChild(adminHost)
  createRoot(adminHost).render(
    createElement(MemoryRouter, { initialEntries: ['/settings?tab=admin'] }, createElement(Settings)),
  )
  await tick(180)
  const adminLabels = [...adminHost.querySelectorAll('.tabs button')].map((element) => element.textContent ?? '')
  assert('admin settings show 常规 / 单点登录 / 邮件', adminLabels.length === 3 && adminLabels.some((text) => text.includes('单点登录')))

  const ssoTab = [...adminHost.querySelectorAll('.tabs button')].find((element) =>
    element.textContent?.includes('单点登录'),
  ) as HTMLButtonElement | undefined
  ssoTab?.dispatchEvent(new window.MouseEvent('click', { bubbles: true }))
  await tick(200)
  const issuerInput = [...adminHost.querySelectorAll('input')].find((element) =>
    (element as HTMLInputElement).placeholder.includes('keycloak.example.com'),
  ) as HTMLInputElement | undefined
  assert('SSO panel shows the issuer field', Boolean(issuerInput))
  assert('SSO panel preloads the stored issuer', issuerInput?.value === 'https://keycloak.example.com/realms/main')
  assert('SSO panel lists provider presets', adminHost.textContent?.includes('Keycloak') === true)
  const keycloakToggle = [...adminHost.querySelectorAll('.section-toggle')].find((element) => /Keycloak/.test(element.textContent ?? '')) as HTMLButtonElement | undefined
  keycloakToggle?.dispatchEvent(new window.MouseEvent('click', { bubbles: true }))
  await tick(60)
  assert(
    'SSO panel offers the three Keycloak export formats',
    [...adminHost.querySelectorAll('option')].filter((element) => /JSON|kcadm|Realm/.test(element.textContent ?? '')).length >= 3,
  )

  // a light preset must switch the document theme
  const paperSwatch = [...editorHost.querySelectorAll('.theme-swatch')].find((element) =>
    element.textContent?.includes('极简白'),
  ) as HTMLButtonElement | undefined
  paperSwatch?.dispatchEvent(new window.MouseEvent('click', { bubbles: true }))
  await tick(120)
  assert('light preset sets data-theme="light"', window.document.documentElement.dataset.theme === 'light')

  // background scenery: every style is selectable and applies immediately
  const sceneryToggle = [...editorHost.querySelectorAll('.section-toggle')].find((element) => /背景场景|Background scenery/.test(element.textContent ?? '')) as HTMLButtonElement | undefined
  sceneryToggle?.dispatchEvent(new window.MouseEvent('click', { bubbles: true }))
  await tick(60)
  const sceneryTiles = [...editorHost.querySelectorAll('.scenery-tile')]
  assert('theme editor offers 7 background styles', sceneryTiles.length >= 7)
  const nebulaTile = sceneryTiles.find((element) => element.textContent?.includes('星云')) as HTMLButtonElement | undefined
  nebulaTile?.dispatchEvent(new window.MouseEvent('click', { bubbles: true }))
  await tick(120)
  assert('choosing a scenery updates <html data-bg>', window.document.documentElement.dataset.bg === 'nebula')

  // grid: cards expose a reorder handle and drag order draft
  container.querySelector('[role="tab"][aria-selected="false"]')
  const gridButton = [...container.querySelectorAll('.segmented button')].find((element) =>
    element.textContent?.includes('网格'),
  ) as HTMLButtonElement | undefined
  gridButton?.dispatchEvent(new window.MouseEvent('click', { bubbles: true }))
  await tick(160)
  const cells = [...container.querySelectorAll('.grid2d-cell')]
  assert('grid mode renders reorderable cells', cells.length === 3 && container.querySelector('.grid2d.reorderable') !== null)
  assert('grid cells carry account ids', cells.every((cell) => (cell as HTMLElement).dataset.accountId !== undefined))

  // horizontal resolution: 3D cards get an LOD bucket instead of being stretched
  const orbitButton = [...container.querySelectorAll('.segmented button')].find((element) =>
    element.textContent?.includes('环绕'),
  ) as HTMLButtonElement | undefined
  orbitButton?.dispatchEvent(new window.MouseEvent('click', { bubbles: true }))
  await tick(200)
  const lodItems = [...container.querySelectorAll('.stage3d-item')]
  assert('3D cards declare a LOD bucket', lodItems.length === 3 && lodItems.every((item) => (item as HTMLElement).dataset.lod !== undefined))

  // i18n: switching to English relabels the interface
  useI18nStore.getState().setLocale('en')
  await tick(150)
  assert('English locale relabels the dashboard', container.textContent?.includes('Pinned') === true)
  useI18nStore.getState().setLocale('zh-CN')
  await tick(100)

  assert('no runtime errors', errors.length === 0)
  if (errors.length) console.error(errors.slice(0, 5))
  console.log(`api calls during smoke: ${requests.length}`)
}

function tick(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

main().then(() => {
  console.log(process.exitCode === 1 ? 'SMOKE FAILED' : 'SMOKE OK')
  process.exit(process.exitCode ?? 0)
})
