import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, BASE, type AndroidRelease } from '../api/client'
import Icon from '../components/Icon'
import Logo from '../components/Logo'
import { useT } from '../i18n'
import { brands } from '../lib/brands'
import { useStore } from '../store/useStore'

/** Where the source code lives. */
export const REPOSITORY = 'https://github.com/ForeverLove37/2fauth-go-and-newUI'

/** Services shown by the demo; the codes are made up and change like real ones. */
const demo = [
  { id: 12, service: 'GitHub', brand: 'github', account: 'octo@example.org', hue: 220 },
  { id: 7, service: 'Cloudflare', brand: 'cloudflare', account: 'ops@example.org', hue: 28 },
  { id: 31, service: 'Proton', brand: 'proton', account: 'me@proton.me', hue: 265 },
  { id: 4, service: 'Google', brand: 'google', account: 'me@gmail.com', hue: 215 },
  { id: 18, service: 'Discord', brand: 'discord', account: 'me#0001', hue: 235 },
  { id: 25, service: 'Tailscale', brand: 'tailscale', account: 'admin', hue: 200 },
]
type DemoItem = (typeof demo)[number]

/** A made-up six digit code for a demo account and a 30 s window. */
function fakeCode(seed: number, window: number): string {
  // Bitwise operators give signed numbers: >>> 0 keeps every step unsigned.
  let x = (seed * 2654435761 + window * 40503) >>> 0
  x = (x ^ (x >>> 15)) >>> 0
  x = Math.imul(x, 2246822507) >>> 0
  x = (x ^ (x >>> 13)) >>> 0
  return String(x % 1_000_000).padStart(6, '0')
}

const codeNow = (item: DemoItem) => fakeCode(item.id, Math.floor(Date.now() / 30000))
const spaced = (code: string) => code.replace(/(\d{3})(\d{3})/, '$1 $2')

function useNow(interval = 1000) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), interval)
    return () => window.clearInterval(timer)
  }, [interval])
  return now
}

const reducedMotion = () => typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

/** The mark of a service on a white tile, like the icons of the panel and the app. */
function BrandTile({ item, size = 34 }: { item: DemoItem; size?: number }) {
  const brand = brands[item.brand]
  return (
    <span className="landing-brand" style={{ width: size, height: size, borderRadius: size * 0.29 }}>
      {brand ? (
        <svg viewBox="0 0 24 24" width={size * 0.6} height={size * 0.6} aria-hidden="true">
          <path d={brand.path} fill={brand.color} />
        </svg>
      ) : (
        <b>{item.service.slice(0, 2).toUpperCase()}</b>
      )}
    </span>
  )
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

function Ring({ left, small }: { left: number; small?: boolean }) {
  return (
    <span className={`landing-ring${small ? ' small' : ''}`} style={{ '--progress': left / 30 } as React.CSSProperties}>
      <b>{left}</b>
    </span>
  )
}

function DemoCard({ item, index, now, copied }: { item: DemoItem; index: number; now: number; copied: boolean }) {
  const t = useT()
  const left = 30 - (Math.floor(now / 1000) % 30)
  return (
    <div className={`landing-card${copied ? ' copied' : ''}`} data-index={index} style={{ '--card-hue': item.hue, '--i': index } as React.CSSProperties}>
      <div className="landing-card-top">
        <BrandTile item={item} />
        <span className="grow">
          <strong>{item.service}</strong>
          <small>{item.account}</small>
        </span>
        <span className="landing-card-copied">
          <Icon name="check" size={12} /> {t('Copied')}
        </span>
      </div>
      <div className="landing-card-bottom">
        <RollingCode code={fakeCode(item.id, Math.floor(now / 30000))} />
        <Ring left={left} />
      </div>
    </div>
  )
}

const AUTO_SPEED = -360 / 26 // degrees per second, the old CSS orbit
const REST_TILT = -12

/**
 * The orbit of cards. It turns by itself; a visitor can drag it (it keeps the
 * momentum), tilt it, turn it with the arrow keys, and click a card to copy its code.
 */
function DemoOrbit({ now }: { now: number }) {
  const t = useT()
  const stageRef = useRef<HTMLDivElement>(null)
  const orbitRef = useRef<HTMLDivElement>(null)
  const motion = useRef({ angle: 0, tilt: REST_TILT, velocity: AUTO_SPEED, dragging: false, touched: 0 })
  const drag = useRef({ x: 0, y: 0, moved: 0, time: 0, id: -1 })
  const [copied, setCopied] = useState<number | null>(null)
  const [used, setUsed] = useState(false)
  const [grabbing, setGrabbing] = useState(false)

  useEffect(() => {
    const stage = stageRef.current
    const orbit = orbitRef.current
    if (!stage || !orbit) return
    const still = reducedMotion()
    let frame = 0
    let last = performance.now()
    let visible = true
    const observer = new IntersectionObserver(([entry]) => {
      visible = entry.isIntersecting
    })
    observer.observe(stage)

    const step = (time: number) => {
      const dt = Math.min(0.05, (time - last) / 1000)
      last = time
      const m = motion.current
      if (visible && !document.hidden) {
        if (!m.dragging) {
          const idle = time - m.touched > 2500
          // Momentum fades into the steady turn of the orbit.
          const target = still ? 0 : AUTO_SPEED
          m.velocity += (target - m.velocity) * Math.min(1, dt * (idle ? 1.6 : 2.4))
          m.angle += m.velocity * dt
          if (idle) m.tilt += (REST_TILT - m.tilt) * Math.min(1, dt * 2)
        }
        orbit.style.transform = `rotateX(${m.tilt.toFixed(2)}deg) rotateY(${m.angle.toFixed(2)}deg)`
      }
      frame = requestAnimationFrame(step)
    }
    frame = requestAnimationFrame(step)
    return () => {
      cancelAnimationFrame(frame)
      observer.disconnect()
    }
  }, [])

  const copy = useCallback((index: number) => {
    setCopied(index)
    window.setTimeout(() => setCopied((current) => (current === index ? null : current)), 1400)
  }, [])

  const onPointerDown = (event: React.PointerEvent) => {
    if (event.button !== 0) return
    drag.current = { x: event.clientX, y: event.clientY, moved: 0, time: performance.now(), id: event.pointerId }
    motion.current.dragging = true
    motion.current.velocity = 0
    motion.current.touched = performance.now()
    stageRef.current?.setPointerCapture(event.pointerId)
    setGrabbing(true)
    setUsed(true)
  }
  const onPointerMove = (event: React.PointerEvent) => {
    const d = drag.current
    const m = motion.current
    if (!m.dragging || event.pointerId !== d.id) return
    const now = performance.now()
    const dx = event.clientX - d.x
    const dy = event.clientY - d.y
    const dt = Math.max(1, now - d.time) / 1000
    m.angle += dx * 0.4
    m.tilt = Math.max(-38, Math.min(26, m.tilt - dy * 0.22))
    // The speed of the last moves, smoothed: what the orbit keeps when it is let go.
    m.velocity = m.velocity * 0.6 + ((dx * 0.4) / dt) * 0.4
    m.touched = now
    d.moved += Math.abs(dx) + Math.abs(dy)
    d.x = event.clientX
    d.y = event.clientY
    d.time = now
  }
  const onPointerCancel = (event: React.PointerEvent) => {
    // The browser took the gesture (a scroll of the page): not a click.
    if (event.pointerId !== drag.current.id) return
    motion.current.dragging = false
    motion.current.touched = performance.now()
    setGrabbing(false)
  }
  const onPointerUp = (event: React.PointerEvent) => {
    const m = motion.current
    if (!m.dragging || event.pointerId !== drag.current.id) return
    m.dragging = false
    m.touched = performance.now()
    // No move since the last sample: the pointer stopped before it was released.
    if (performance.now() - drag.current.time > 80) m.velocity = 0
    m.velocity = Math.max(-720, Math.min(720, m.velocity))
    setGrabbing(false)
    if (drag.current.moved < 6) {
      // A click: the card under the pointer (the capture hides it from the event target).
      stageRef.current?.releasePointerCapture(event.pointerId)
      const card = document.elementFromPoint(event.clientX, event.clientY)?.closest<HTMLElement>('.landing-card')
      if (card?.dataset.index) copy(Number(card.dataset.index))
    }
  }
  const onKeyDown = (event: React.KeyboardEvent) => {
    const m = motion.current
    if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
      event.preventDefault()
      m.velocity = event.key === 'ArrowLeft' ? 260 : -260
      m.touched = performance.now()
      setUsed(true)
    } else if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
      event.preventDefault()
      m.tilt = Math.max(-38, Math.min(26, m.tilt + (event.key === 'ArrowUp' ? -6 : 6)))
      m.touched = performance.now()
      setUsed(true)
    } else if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      // The card that faces the visitor.
      const front = ((Math.round(-m.angle / 60) % 6) + 6) % 6
      copy(front)
    }
  }

  return (
    <div className="landing-stage-wrap">
      <div
        ref={stageRef}
        className={`landing-stage${grabbing ? ' grabbing' : ''}`}
        tabIndex={0}
        role="group"
        aria-label={t('Demo: drag to turn the cards, click one to copy its code')}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerCancel}
        onKeyDown={onKeyDown}
      >
        <div className="landing-orbit-scale">
          <div ref={orbitRef} className="landing-orbit">
            {demo.map((item, index) => (
              <DemoCard key={item.service} item={item} index={index} now={now} copied={copied === index} />
            ))}
          </div>
        </div>
      </div>
      <p className={`landing-hint${used ? ' faded' : ''}`}>
        <Icon name="orbit" size={14} /> {t('Drag to turn the cards · click one to copy its code')}
      </p>
    </div>
  )
}

/* ------------------------------------------------------------------ the phone */

type PhoneTab = 'codes' | 'cloud'
type Toast = { id: number; title: string; detail: string }

const phoneCodes = demo.slice(0, 4).map((item, index) => ({ item, kind: index < 2 ? ('local' as const) : ('synced' as const) }))
const phoneCloud = demo.slice(4)

/**
 * The phone of the demo, a working miniature of the Android app: switch tabs, tap a
 * code to copy it, sync, add a code. It plays by itself until a visitor touches it.
 */
function PhoneDemo({ now }: { now: number }) {
  const t = useT()
  const [tab, setTab] = useState<PhoneTab>('codes')
  const [toast, setToast] = useState<Toast | null>(null)
  const [pressed, setPressed] = useState<string | null>(null)
  const [syncing, setSyncing] = useState(false)
  const [syncedAt, setSyncedAt] = useState(() => Date.now() - 4 * 60_000)
  const [sheet, setSheet] = useState(false)
  const [loading, setLoading] = useState<string | null>(null)
  const [touched, setTouched] = useState(false)
  const toastId = useRef(0)
  const timers = useRef<number[]>([])

  useEffect(() => () => timers.current.forEach((id) => window.clearTimeout(id)), [])
  const later = (fn: () => void, ms: number) => {
    timers.current.push(window.setTimeout(fn, ms))
  }

  const say = useCallback((title: string, detail: string) => {
    const id = ++toastId.current
    setToast({ id, title, detail })
    timers.current.push(window.setTimeout(() => setToast((current) => (current?.id === id ? null : current)), 1800))
  }, [])

  const copy = useCallback(
    (item: DemoItem) => {
      setPressed(item.service)
      timers.current.push(window.setTimeout(() => setPressed((current) => (current === item.service ? null : current)), 260))
      const left = 30 - (Math.floor(Date.now() / 1000) % 30)
      say(item.service, left <= 5 ? t('Code copied — it expires in {n} s', { n: left }) : t('Code copied'))
    },
    [say, t],
  )

  // Until the visitor plays with it, the phone shows itself: a tap every few seconds.
  useEffect(() => {
    if (touched) return
    const timer = window.setInterval(() => copy(phoneCodes[Math.floor(Date.now() / 6000) % 2].item), 6000)
    return () => window.clearInterval(timer)
  }, [touched, copy])

  const touch = () => setTouched(true)
  const window30 = Math.floor(now / 30000)
  const left = 30 - (Math.floor(now / 1000) % 30)
  const minutes = Math.floor((now - syncedAt) / 60_000)

  const tapCloud = (item: DemoItem) => {
    touch()
    setLoading(item.service)
    later(() => {
      setLoading(null)
      copy(item)
    }, 450)
  }

  return (
    <div className="landing-phone" onPointerDown={touch}>
      <div className="landing-phone-screen">
        <div className={`landing-phone-toast${toast ? ' show' : ''}`} role="status" aria-live="polite">
          {toast && (
            <>
              <span className="tick">
                <Icon name="check" size={12} />
              </span>
              <span>
                <b>{toast.title}</b>
                <small>{toast.detail}</small>
              </span>
            </>
          )}
        </div>

        <div className="landing-phone-head">
          <Logo size={20} />
          <span className="grow">2FAuth</span>
          <button
            type="button"
            className={`landing-phone-icon${syncing ? ' spin' : ''}`}
            aria-label={t('Sync')}
            onClick={() => {
              if (syncing) return
              setSyncing(true)
              later(() => {
                setSyncing(false)
                setSyncedAt(Date.now())
                say(t('Synced'), t('Everything is up to date'))
              }, 1100)
            }}
          >
            <Icon name="refresh" size={14} />
          </button>
        </div>
        <div className="landing-phone-sync">
          {syncing ? t('Syncing…') : minutes < 1 ? t('Synced just now') : t('Synced {n} min ago', { n: minutes })}
        </div>

        <div className="landing-phone-tabs" role="tablist">
          {(['codes', 'cloud'] as const).map((id) => (
            <button key={id} type="button" role="tab" aria-selected={tab === id} className={tab === id ? 'on' : ''} onClick={() => setTab(id)}>
              {id === 'codes' ? t('Codes') : t('Cloud')}
            </button>
          ))}
        </div>

        <div className="landing-phone-list" key={tab}>
          {tab === 'codes'
            ? phoneCodes.map(({ item, kind }, index) => (
                <button
                  type="button"
                  key={item.service}
                  className={`landing-phone-row${pressed === item.service ? ' tapped' : ''}`}
                  style={{ '--card-hue': item.hue, '--i': index } as React.CSSProperties}
                  onClick={() => {
                    touch()
                    copy(item)
                  }}
                >
                  <BrandTile item={item} size={28} />
                  <span className="grow">
                    <strong>{item.service}</strong>
                    <small>{kind === 'local' ? t('On this phone') : t('Synced')}</small>
                  </span>
                  <span className="mono">{spaced(fakeCode(item.id, window30))}</span>
                  <Ring left={left} small />
                </button>
              ))
            : phoneCloud.map((item, index) => (
                <button
                  type="button"
                  key={item.service}
                  className={`landing-phone-row cloud${pressed === item.service ? ' tapped' : ''}`}
                  style={{ '--card-hue': item.hue, '--i': index } as React.CSSProperties}
                  onClick={() => tapCloud(item)}
                >
                  <BrandTile item={item} size={28} />
                  <span className="grow">
                    <strong>{item.service}</strong>
                    <small>{t('Secret stays on the server')}</small>
                  </span>
                  <span className="mono">{loading === item.service ? '··· ···' : spaced(fakeCode(item.id, window30))}</span>
                </button>
              ))}
          {tab === 'cloud' && <p className="landing-phone-note">{t('The phone only receives short-lived codes for these.')}</p>}
        </div>

        <button type="button" className="landing-phone-fab" aria-label={t('Add a code')} onClick={() => setSheet(true)}>
          <Icon name="plus" size={18} />
        </button>

        <div className={`landing-phone-sheet${sheet ? ' open' : ''}`} aria-hidden={!sheet}>
          <div className="landing-phone-scrim" onClick={() => setSheet(false)} />
          <div className="landing-phone-panel">
            <i className="handle" />
            <strong>{t('Save a new code as')}</strong>
            {[
              { key: 'local', title: t('On this phone'), text: t('Default. The secret never leaves the phone.') },
              { key: 'synced', title: t('Synced'), text: t('Kept on the phone and the server; asks for your fingerprint.') },
              { key: 'cloud', title: t('Cloud'), text: t('Kept on the server; the phone only gets codes.') },
            ].map((option) => (
              <button
                type="button"
                key={option.key}
                tabIndex={sheet ? 0 : -1}
                onClick={() => {
                  setSheet(false)
                  say(option.title, t('Demo only: nothing was saved'))
                }}
              >
                <b>{option.title}</b>
                <small>{option.text}</small>
              </button>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

/* --------------------------------------------------------------- the terminal */

type Line = { kind: 'cmd' | 'out' | 'err' | 'json'; text: string }

/** What the SSH command of the server answers, for the demo accounts. */
function runCommand(input: string): Line[] {
  const words = input.trim().split(/\s+/).filter(Boolean)
  // "ssh [options] otp@host <command>" or the command alone
  if (words[0] === 'ssh') {
    const at = words.findIndex((word) => word.includes('@'))
    words.splice(0, at >= 0 ? at + 1 : 1)
  }
  const json = words.includes('--json')
  const args = words.filter((word) => word !== '--json')
  const [command = '', ...rest] = args
  if (!command) return []
  const out = (text: string): Line => ({ kind: 'out', text })
  switch (command) {
    case 'clear':
      return [{ kind: 'out', text: '\u0000clear' }]
    case 'help':
    case '--help':
    case '-h':
      return [
        'Commands:',
        '  get <account> [--json]   print the current code of an account',
        '  list [--json]            list the accounts this key can read',
        '  whoami                   show this key and what it can read',
        '  help                     show this message',
      ].map(out)
    case 'whoami':
      return ['agent (agent@example.org)', 'key: laptop SHA256:q3J8…Xw', `reads: all accounts, ${demo.length} right now`].map(out)
    case 'list':
    case 'ls':
      if (json) return [{ kind: 'json', text: JSON.stringify(demo.map((d) => ({ id: d.id, service: d.service, account: d.account, otp_type: 'totp' }))) }]
      return demo.map((d) => out(`${String(d.id).padEnd(4)}${d.service.padEnd(12)}${d.account.padEnd(18)}totp`))
  }
  const query = (command === 'get' ? rest : args).join(' ').toLowerCase().replace(/^#/, '')
  if (!query) return [{ kind: 'err', text: 'Usage: get <account> [--json]' }]
  const hits = demo.filter((d) => String(d.id) === query || `${d.service}:${d.account}`.toLowerCase().includes(query))
  if (hits.length === 0) return [{ kind: 'err', text: `No account matches "${query}" for this key.` }]
  if (hits.length > 1) return [{ kind: 'err', text: `"${query}" matches several accounts, use an id:` }, ...hits.map((d) => ({ kind: 'err' as const, text: `  ${d.id}\t${d.service}\t${d.account}` }))]
  const hit = hits[0]
  const code = codeNow(hit)
  if (!json) return [out(code)]
  const expires = 30 - (Math.floor(Date.now() / 1000) % 30)
  return [{ kind: 'json', text: JSON.stringify({ password: code, otp_type: 'totp', period: 30, id: hit.id, service: hit.service, account: hit.account, expires_in: expires }) }]
}

/**
 * The terminal of the demo: an agent asking for codes over SSH, typed key by key
 * like a person (or an agent) would. A click hands the keyboard to the visitor.
 */
function TerminalDemo() {
  const t = useT()
  const host = typeof window !== 'undefined' ? window.location.hostname : 'vault.example.org'
  const prefix = `ssh otp@${host}`
  const script = useMemo(() => [`${prefix} get github`, `${prefix} list`, `${prefix} get cloudflare --json`], [prefix])
  const [lines, setLines] = useState<Line[]>([])
  const [typed, setTyped] = useState('')
  const [typing, setTyping] = useState(false)
  const [live, setLive] = useState(false)
  const [input, setInput] = useState('')
  const [history, setHistory] = useState<string[]>([])
  const [, setRecall] = useState(-1)
  const inputRef = useRef<HTMLInputElement>(null)
  const bodyRef = useRef<HTMLPreElement>(null)

  const run = useCallback(
    (command: string) => {
      const answer = runCommand(command)
      setLines((current) => {
        if (answer[0]?.text === '\u0000clear') return []
        return [...current, { kind: 'cmd' as const, text: command }, ...answer].slice(-40)
      })
    },
    [],
  )

  // The script, played until the visitor takes the keyboard.
  useEffect(() => {
    if (live) return
    let cancelled = false
    let timer = 0
    const wait = (ms: number) =>
      new Promise<void>((resolve) => {
        timer = window.setTimeout(resolve, ms)
      })
    const still = reducedMotion()
    ;(async () => {
      for (let round = 0; !cancelled; round++) {
        setLines([])
        await wait(500)
        for (const command of script) {
          if (cancelled) return
          setTyping(true)
          if (still) {
            setTyped(command)
          } else {
            for (let i = 1; i <= command.length && !cancelled; i++) {
              setTyped(command.slice(0, i))
              const char = command[i - 1]
              // A steady rhythm with small human irregularities; a breath after a word.
              await wait(char === ' ' ? 90 + Math.random() * 70 : 38 + Math.random() * 42)
            }
          }
          if (cancelled) return
          setTyping(false)
          await wait(380)
          if (cancelled) return
          setTyped('')
          run(command)
          await wait(command.includes('list') ? 2600 : 2100)
        }
        await wait(1200)
      }
    })()
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [live, script, run])

  useEffect(() => {
    const body = bodyRef.current
    if (body) body.scrollTop = body.scrollHeight
  }, [lines, typed, input])

  const takeOver = () => {
    if (!live) {
      setLive(true)
      setTyped('')
      setTyping(false)
      setLines([{ kind: 'out', text: t('Your turn: try "help", "list" or "get github".') }])
    }
    inputRef.current?.focus({ preventScroll: true })
  }

  const onKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      event.preventDefault()
      const command = input.trim()
      setInput('')
      setRecall(-1)
      if (!command) {
        setLines((current) => [...current, { kind: 'cmd', text: '' }])
        return
      }
      setHistory((current) => [...current, command].slice(-20))
      run(command)
    } else if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
      event.preventDefault()
      setRecall((current) => {
        const next = event.key === 'ArrowUp' ? (current < 0 ? history.length - 1 : Math.max(0, current - 1)) : current < 0 ? -1 : current + 1
        if (next < 0 || next >= history.length) {
          setInput('')
          return -1
        }
        setInput(history[next])
        return next
      })
    } else if (event.key === 'l' && event.ctrlKey) {
      event.preventDefault()
      setLines([])
    }
  }

  const current = live ? input : typed
  return (
    <div className={`landing-terminal${live ? ' live' : ''}`} onClick={takeOver}>
      <div className="landing-terminal-bar">
        <i />
        <i />
        <i />
        <span>agent@laptop — ssh</span>
        <span className="spacer" />
        {!live && <em>{t('Click to type')}</em>}
      </div>
      <pre ref={bodyRef} aria-live="polite">
        {lines.map((line, index) =>
          line.kind === 'cmd' ? (
            <div key={index}>
              <span className="prompt">$ </span>
              {line.text}
            </div>
          ) : (
            <div key={index} className={`${line.kind} appear`}>
              {line.text}
            </div>
          ),
        )}
        <div>
          <span className="prompt">$ </span>
          {current}
          <span className={`caret${typing || (live && input) ? ' solid' : ''}`} />
        </div>
      </pre>
      <input
        ref={inputRef}
        className="landing-terminal-input"
        value={input}
        onChange={(event) => setInput(event.target.value)}
        onKeyDown={onKeyDown}
        aria-label={t('Command')}
        autoComplete="off"
        autoCapitalize="off"
        spellCheck={false}
        tabIndex={live ? 0 : -1}
      />
    </div>
  )
}

/* ------------------------------------------------------------------- the page */

export function GitHubMark({ size = 16 }: { size?: number }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} aria-hidden="true">
      <path d={brands.github.path} fill="currentColor" />
    </svg>
  )
}

/** The links at the bottom of the public pages. */
export function LegalFooter({ appName }: { appName: string }) {
  const t = useT()
  return (
    <footer className="landing-footer">
      <Logo size={22} />
      <span>
        {appName} · {t('Self-hosted two-factor authentication')}
      </span>
      <span className="spacer" />
      <nav>
        <a href={REPOSITORY} target="_blank" rel="noreferrer">
          <GitHubMark size={14} /> GitHub
        </a>
        <Link to="/terms">{t('Terms')}</Link>
        <Link to="/privacy">{t('Privacy')}</Link>
        <Link to="/agents">{t('Agents')}</Link>
        <Link to="/login">{t('Sign in')}</Link>
      </nav>
    </footer>
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
          <a href={REPOSITORY} target="_blank" rel="noreferrer" aria-label="GitHub">
            <GitHubMark />
          </a>
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
        <DemoOrbit now={now} />
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
        <div className="landing-phone-wrap">
          <PhoneDemo now={now} />
          <p className="landing-hint">{t('Try it: tap a code, switch to Cloud, press + or sync')}</p>
        </div>
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
          <div className="landing-cta">
            <Link className="btn" to="/agents">
              {t('Read the agent guide')}
            </Link>
            <a className="btn ghost" href={REPOSITORY} target="_blank" rel="noreferrer">
              <GitHubMark size={15} /> {t('Source code')}
            </a>
          </div>
        </div>
        <TerminalDemo />
      </section>

      <LegalFooter appName={appName} />
    </div>
  )
}
