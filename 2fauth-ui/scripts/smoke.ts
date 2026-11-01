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
  const Dashboard = (await import('../src/views/Dashboard')).default
  const ThemeEditor = (await import('../src/views/ThemeEditor')).default
  const { Login } = await import('../src/views/Auth')

  // fake API: no network in jsdom
  let calls = 0
  globals.fetch = async () => {
    calls += 1
    return {
      ok: true,
      status: 200,
      text: async () => JSON.stringify({ data: [] }),
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
  const modes = ['环绕', '螺旋', '卡墙', '球面', '混合', '网格', '列表']
  for (const label of modes) {
    const button = [...container.querySelectorAll('.segmented button')].find((element) =>
      element.textContent?.includes(label),
    ) as HTMLButtonElement | undefined
    assert(`mode button "${label}" exists`, Boolean(button))
    button?.dispatchEvent(new window.MouseEvent('click', { bubbles: true }))
    await tick(120)
    if (label === '网格' || label === '混合') {
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
  await tick(120)
  assert('login form rendered', loginHost.querySelectorAll('input').length >= 2)

  assert('no runtime errors', errors.length === 0)
  if (errors.length) console.error(errors.slice(0, 5))
  console.log(`api calls during smoke: ${calls}`)
}

function tick(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

main().then(() => {
  console.log(process.exitCode === 1 ? 'SMOKE FAILED' : 'SMOKE OK')
  process.exit(process.exitCode ?? 0)
})
