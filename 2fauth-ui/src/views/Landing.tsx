import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, BASE, type AndroidRelease } from '../api/client'
import Icon from '../components/Icon'
import Logo from '../components/Logo'
import { useT } from '../i18n'
import { useStore } from '../store/useStore'

/** Services shown by the demo; the codes are made up and change like real ones. */
const demo = [
  { service: 'GitHub', account: 'octo@example.org', hue: 220 },
  { service: 'Cloudflare', account: 'ops@example.org', hue: 28 },
  { service: 'Proton', account: 'me@proton.me', hue: 265 },
  { service: 'AWS', account: 'root', hue: 38 },
  { service: 'Discord', account: 'me#0001', hue: 235 },
  { service: 'Tailscale', account: 'admin', hue: 200 },
]

/** A made-up six digit code for a demo card and a 30 s window. */
function fakeCode(seed: number, window: number): string {
  // Bitwise operators give signed numbers: >>> 0 keeps every step unsigned.
  let x = (seed * 2654435761 + window * 40503) >>> 0
  x = (x ^ (x >>> 15)) >>> 0
  x = Math.imul(x, 2246822507) >>> 0
  x = (x ^ (x >>> 13)) >>> 0
  return String(x % 1_000_000).padStart(6, '0')
}

function useNow(interval = 1000) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), interval)
    return () => window.clearInterval(timer)
  }, [interval])
  return now
}

/** Digits that roll into place when the code changes. */
function RollingCode({ code }: { code: string }) {
  return (
    <span className="landing-code" aria-label={code}>
      {code.split('').map((digit, index) => (
        <span key={`${index}-${digit}`} className="landing-digit" style={{ animationDelay: `${index * 45}ms` }}>
          {digit}
        </span>
      ))}
    </span>
  )
}

function DemoCard({ item, index, now }: { item: (typeof demo)[number]; index: number; now: number }) {
  const window = Math.floor(now / 30000)
  const left = 30 - (Math.floor(now / 1000) % 30)
  return (
    <div className="landing-card" style={{ '--card-hue': item.hue, '--i': index } as React.CSSProperties}>
      <div className="landing-card-top">
        <span className="landing-avatar">{item.service.slice(0, 2).toUpperCase()}</span>
        <span>
          <strong>{item.service}</strong>
          <small>{item.account}</small>
        </span>
      </div>
      <div className="landing-card-bottom">
        <RollingCode code={fakeCode(index + 1, window)} />
        <span className="landing-ring" style={{ '--progress': left / 30 } as React.CSSProperties}>
          <b>{left}</b>
        </span>
      </div>
    </div>
  )
}

/** The terminal of the demo: an agent asking for a code over SSH, typed again and again. */
function TerminalDemo({ now }: { now: number }) {
  const host = typeof window !== 'undefined' ? window.location.hostname : 'vault.example.org'
  const command = `ssh otp@${host} get github`
  const cycle = Math.floor(now / 250) % 48
  const typed = command.slice(0, Math.max(0, cycle - 4))
  const done = cycle - 4 >= command.length
  return (
    <div className="landing-terminal" aria-hidden="true">
      <div className="landing-terminal-bar">
        <i />
        <i />
        <i />
        <span>agent@laptop</span>
      </div>
      <pre>
        <span className="prompt">$ </span>
        {typed}
        {!done && <span className="caret" />}
        {done && (
          <>
            {'\n'}
            <span className="out">{fakeCode(1, Math.floor(now / 30000))}</span>
            {'\n'}
            <span className="prompt">$ </span>
            <span className="caret" />
          </>
        )}
      </pre>
    </div>
  )
}

/** The phone of the demo: the codes screen, with a tap that copies a code. */
function PhoneDemo({ now }: { now: number }) {
  const t = useT()
  const tapped = Math.floor(now / 1000) % 6 === 2
  return (
    <div className="landing-phone" aria-hidden="true">
      <div className="landing-phone-screen">
        <div className="landing-phone-head">
          <Logo size={20} />
          <span>2FAuth</span>
        </div>
        <div className="landing-phone-tabs">
          <b>{t('Codes')}</b>
          <span>{t('Cloud')}</span>
        </div>
        {demo.slice(0, 4).map((item, index) => (
          <div key={item.service} className={`landing-phone-row${tapped && index === 1 ? ' tapped' : ''}`} style={{ '--card-hue': item.hue } as React.CSSProperties}>
            <span className="landing-avatar small">{item.service.slice(0, 2).toUpperCase()}</span>
            <span className="grow">
              <strong>{item.service}</strong>
              <small>{index < 2 ? t('On this phone') : t('Synced')}</small>
            </span>
            <span className="mono">{fakeCode(index + 1, Math.floor(now / 30000)).replace(/(\d{3})(\d{3})/, '$1 $2')}</span>
          </div>
        ))}
        <div className={`landing-phone-toast${tapped ? ' show' : ''}`}>{t('Code copied')}</div>
      </div>
    </div>
  )
}

/** The page visitors see at the address of the server before they sign in. */
export default function Landing() {
  const t = useT()
  const config = useStore((state) => state.config)
  const now = useNow()
  const [release, setRelease] = useState<AndroidRelease | null>(null)

  useEffect(() => {
    api.androidLatest().then(setRelease).catch(() => setRelease(null))
  }, [])

  const appName = config?.appName ?? '2FAuth'
  const features = useMemo(
    () => [
      { icon: 'orbit', title: t('A 3D control panel'), text: t('Orbit, sphere, list or a free grid you arrange like a folder. Codes stay hidden until you look at them.') },
      { icon: 'lock', title: t('Three kinds of codes on the phone'), text: t('Kept only on the phone, synced with this server, or left in the cloud where the phone only receives short-lived codes.') },
      { icon: 'shield', title: t('Secrets stay yours'), text: t('Self-hosted, encrypted at rest, sessions protected against CSRF, passkeys and single sign-on.') },
      { icon: 'download', title: t('Bring your codes'), text: t('Imports Google Authenticator, Aegis, 2FAS, Bitwarden and more; exports encrypted backups.') },
      { icon: 'more', title: t('For scripts and AI agents'), text: t('Codes over SSH with read-only keys limited to chosen accounts, and a guide written for agents.') },
      { icon: 'users', title: t('Share with care'), text: t('Share an account with a colleague, see every code handed out in its log.') },
    ],
    [t],
  )

  return (
    <div className="landing">
      <header className="landing-nav">
        <div className="brand">
          <Logo />
          <span>{appName}</span>
        </div>
        <nav>
          <a href="#features">{t('Features')}</a>
          <a href="#android">{t('Android app')}</a>
          <Link to="/agents">{t('Agents')}</Link>
        </nav>
        <Link className="btn primary small" to="/login">
          {t('Sign in')}
        </Link>
      </header>

      <section className="landing-hero">
        <div className="landing-hero-text">
          <div className="landing-logo-wrap">
            <Logo className="landing-logo" />
          </div>
          <h1>{t('Your two-factor codes, on your own server.')}</h1>
          <p>
            {t(
              '{app} keeps the TOTP and HOTP secrets of your accounts and turns them into codes: in a 3D panel, in a native Android app that also works offline, and over SSH for your scripts.',
              { app: appName },
            )}
          </p>
          <div className="landing-cta">
            <Link className="btn primary" to="/login">
              {t('Open the panel')}
            </Link>
            {release && (
              <a className="btn" href={`${BASE}/download/android/latest.apk`}>
                <Icon name="download" size={15} /> {t('Android {version}', { version: release.version_name })}
              </a>
            )}
            <Link className="btn ghost" to="/agents">
              {t('Guide for agents')}
            </Link>
          </div>
        </div>
        <div className="landing-stage" aria-hidden="true">
          <div className="landing-orbit">
            {demo.map((item, index) => (
              <DemoCard key={item.service} item={item} index={index} now={now} />
            ))}
          </div>
        </div>
      </section>

      <section className="landing-section" id="features">
        <h2>{t('Everything a vault of codes needs')}</h2>
        <div className="landing-grid">
          {features.map((feature, index) => (
            <article key={feature.title} className="landing-feature" style={{ '--i': index } as React.CSSProperties}>
              <span className="landing-feature-icon">
                <Icon name={feature.icon as never} size={18} />
              </span>
              <h3>{feature.title}</h3>
              <p>{feature.text}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="landing-section landing-split" id="android">
        <PhoneDemo now={now} />
        <div>
          <h2>{t('The Android app')}</h2>
          <ul className="landing-list">
            <li>{t('Codes kept on the phone open instantly, even without a network.')}</li>
            <li>{t('New codes stay on the phone; syncing one to the server asks for your fingerprint.')}</li>
            <li>{t('Synced codes follow the changes made on the web; cloud codes never leave the server.')}</li>
            <li>{t('App lock, blocked screenshots, codes wiped from the clipboard.')}</li>
            <li>{t('Updates are published here and offered by the app itself.')}</li>
          </ul>
          {release ? (
            <a className="btn primary" href={`${BASE}/download/android/latest.apk`}>
              <Icon name="download" size={15} /> {t('Download {version}', { version: release.version_name })}
              <span className="hint">· {(release.size / 1024 / 1024).toFixed(1)} MB</span>
            </a>
          ) : (
            <p className="hint">{t('No version has been published on this server yet.')}</p>
          )}
        </div>
      </section>

      <section className="landing-section landing-split reverse">
        <div>
          <h2>{t('Codes for scripts and agents')}</h2>
          <p className="landing-muted">
            {t('Register the public key of a machine and choose what it may read. It asks for a code with a plain ssh command; nothing to install, and the key can only read.')}
          </p>
          <Link className="btn" to="/agents">
            {t('Read the agent guide')}
          </Link>
        </div>
        <TerminalDemo now={now} />
      </section>

      <footer className="landing-footer">
        <Logo size={22} />
        <span>
          {appName} · {t('Self-hosted two-factor authentication')}
        </span>
        <span className="spacer" />
        <Link to="/login">{t('Sign in')}</Link>
      </footer>
    </div>
  )
}
