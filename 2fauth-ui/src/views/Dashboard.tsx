import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../api/client'
import type { Account } from '../api/types'
import AccountModal from '../components/AccountModal'
import { AccountCard, CardAvatar, LiveCode } from '../components/AccountCard'
import FreeGrid, { type ArrangeKind, type FreeGridHandle } from '../components/FreeGrid'
import Menu from '../components/Menu'
import { GroupsModal, ImportExportModal } from '../components/Transfer'
import Icon from '../components/Icon'
import { useT } from '../i18n'
import { accountLabel, accountSubLabel, copyText, currentCode, maskedCode, remainingSeconds } from '../lib/format'
import { usePolling, useSecond, useTimedReveal } from '../lib/hooks'
import Stage3D from '../stage/Stage3D'
import { hasRadius, type StageLayout } from '../stage/layouts'
import { useStore } from '../store/useStore'
import { useThemeStore } from '../theme/store'
import type { BackgroundStyle } from '../theme/types'

type ViewMode = StageLayout | 'grid' | 'list'

const MODE_KEY = '2fauth.mode.v1'
const ROTATE_KEY = '2fauth.autorotate.v1'
const RADIUS_KEY = '2fauth.radius.v1'
const GRID_KEY = '2fauth.grid.v1'

const modeMeta: { id: ViewMode; label: string; icon: 'orbit' | 'grid' | 'share' | 'qr' | 'sliders' | 'eye' | 'copy' }[] = [
  { id: 'orbit', label: 'Orbit', icon: 'orbit' },
  { id: 'helix', label: 'Helix', icon: 'share' },
  { id: 'wall', label: 'Wall', icon: 'grid' },
  { id: 'sphere', label: 'Sphere', icon: 'eye' },
  { id: 'grid', label: 'Grid', icon: 'sliders' },
  { id: 'list', label: 'List', icon: 'copy' },
]

const modes: ViewMode[] = ['orbit', 'helix', 'wall', 'sphere', 'grid', 'list']

const sceneryOrder: BackgroundStyle[] = ['grid', 'stars', 'nebula', 'waves', 'aurora', 'bokeh', 'minimal']
const sceneryLabel: Record<BackgroundStyle, string> = {
  grid: 'Grid',
  stars: 'Starfield',
  nebula: 'Nebula',
  waves: 'Waves',
  aurora: 'Aurora',
  bokeh: 'Bokeh',
  minimal: 'Minimal',
}

/** A code outside a card: hidden like the others, shown when the mouse moves onto it. */
function InlineCode({
  account,
  masked,
  className,
  title,
  onClick,
}: {
  account: Account
  masked: boolean
  className: string
  title?: string
  onClick?: (event: React.MouseEvent) => void
}) {
  const { revealed, handlers } = useTimedReveal()
  const hidden = masked && !revealed
  return (
    <button className={`${className}${hidden ? ' dots' : ''}`} onClick={onClick} title={title} {...handlers}>
      {hidden ? maskedCode(account) : <LiveCode account={account} />}
    </button>
  )
}

/** Seconds left of the selected account. */
function FocusCountdown({ account }: { account: Account }) {
  const t = useT()
  const now = useSecond()
  return <>{account.otp_type === 'hotp' ? t('counter') : `${remainingSeconds(account, now)}s`}</>
}

/** The rate the browser is really drawing at. */
function FpsMeter() {
  const [fps, setFps] = useState(0)
  useEffect(() => {
    let frame = 0
    let count = 0
    let start = performance.now()
    const tick = (now: number) => {
      count += 1
      if (now - start >= 500) {
        setFps(Math.round((count * 1000) / (now - start)))
        count = 0
        start = now
      }
      frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [])
  return <span className="fps-meter mono">{fps} FPS</span>
}

function loadMode(): ViewMode {
  const stored = localStorage.getItem(MODE_KEY) as ViewMode | null
  if (stored && modes.includes(stored)) return stored
  return 'orbit'
}

function loadRadii(): Record<string, number> {
  try {
    return JSON.parse(localStorage.getItem(RADIUS_KEY) ?? '{}') as Record<string, number>
  } catch {
    return {}
  }
}

/** The account panel: four 3D layouts plus 2D views sharing the same cards. */
export default function Dashboard() {
  const t = useT()
  const accounts = useStore((state) => state.accounts)
  const groups = useStore((state) => state.groups)
  const prefs = useStore((state) => state.prefs)
  const selectedId = useStore((state) => state.selectedId)
  const select = useStore((state) => state.select)
  const toast = useStore((state) => state.toast)
  const loadAccounts = useStore((state) => state.loadAccounts)
  const loadGroups = useStore((state) => state.loadGroups)
  const toggleFavorite = useStore((state) => state.toggleFavorite)
  const locked = useStore((state) => state.locked)
  const userId = useStore((state) => state.user?.id)
  const setPreference = useStore((state) => state.setPreference)
  const themeAutoRotate = useThemeStore((state) => state.theme.motion.autoRotate)
  const motion = useThemeStore((state) => state.theme.motion)
  const background = useThemeStore((state) => state.theme.background)
  const patchBackground = useThemeStore((state) => state.patchBackground)

  const [mode, setMode] = useState<ViewMode>(() => loadMode())
  const [autoRotate, setAutoRotate] = useState<boolean>(() => {
    const stored = localStorage.getItem(ROTATE_KEY)
    return stored === null ? themeAutoRotate : stored === '1'
  })
  const [groupFilter, setGroupFilter] = useState<number | 'all' | 'favorites'>('all')
  const [query, setQuery] = useState('')
  const [editing, setEditing] = useState<Account | null>(null)
  const [creating, setCreating] = useState(false)
  const [transferOpen, setTransferOpen] = useState(false)
  const [groupsOpen, setGroupsOpen] = useState(false)
  // the card enlarged in the 3D modes
  const [focusedId, setFocusedId] = useState<number | null>(null)
  const [radii, setRadii] = useState(loadRadii)
  const searchRef = useRef<HTMLInputElement>(null)
  const gridRef = useRef<FreeGridHandle>(null)

  // Hidden unless the user chose otherwise, also while the preferences are loading.
  const dots = prefs.showOtpAsDot !== false
  const showNext = prefs.showNextOtp !== false
  const showIcons = prefs.showAccountsIcons !== false

  useEffect(() => {
    localStorage.setItem(MODE_KEY, mode)
  }, [mode])

  useEffect(() => {
    localStorage.setItem(ROTATE_KEY, autoRotate ? '1' : '0')
  }, [autoRotate])

  useEffect(() => {
    localStorage.setItem(RADIUS_KEY, JSON.stringify(radii))
  }, [radii])

  usePolling(() => {
    void loadAccounts(true).catch(() => {
      /* the session guard already handles expired sessions */
    })
  }, 5000)

  useEffect(() => {
    void loadGroups()
  }, [loadGroups])

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return accounts.filter((account) => {
      if (groupFilter === 'favorites' && !account.is_favorite) return false
      if (typeof groupFilter === 'number' && account.group_id !== groupFilter) return false
      if (!needle) return true
      return `${account.service ?? ''} ${account.account ?? ''}`.toLowerCase().includes(needle)
    })
  }, [accounts, groupFilter, query])

  const selected = visible.find((account) => account.id === selectedId) ?? visible[0] ?? null
  // A filtered view has gaps in it: its cards are shown one after the other instead.
  const canArrange = mode === 'grid' && groupFilter === 'all' && query.trim() === ''
  const radius = hasRadius(mode) ? (radii[mode] ?? 1) : 1

  useEffect(() => {
    if (selected && selected.id !== selectedId) select(selected.id)
  }, [selected, selectedId, select])

  const is3D = mode === 'orbit' || mode === 'helix' || mode === 'wall' || mode === 'sphere'

  // A focused card that is filtered away, or a change of mode, ends the focus.
  useEffect(() => {
    if (focusedId !== null && (!is3D || !visible.some((account) => account.id === focusedId))) setFocusedId(null)
  }, [focusedId, is3D, visible])

  async function copy(account: Account | null) {
    const code = account ? currentCode(account, Date.now()) : ''
    if (!account || !code) return
    const ok = await copyText(code)
    toast(
      ok ? 'success' : 'error',
      ok ? t('Code copied for {name}', { name: accountLabel(account) }) : t('Could not copy the code'),
    )
  }

  async function hotp(account: Account | null) {
    if (!account || account.otp_type !== 'hotp') return
    try {
      const payload = await api.otp(account.id)
      await copyText(payload.password)
      toast('success', dots ? t('New counter code copied') : t('New counter code {code}', { code: payload.password }))
      await loadAccounts(true)
    } catch {
      toast('error', t('Could not generate a new code'))
    }
  }

  function cycleScenery() {
    const current = background?.style ?? 'grid'
    const next = sceneryOrder[(sceneryOrder.indexOf(current) + 1) % sceneryOrder.length]
    patchBackground({ style: next })
  }

  const reorder = useCallback(
    async (ids: number[]) => {
      // The new order holds on screen while it is being saved.
      const byId = new Map(useStore.getState().accounts.map((account) => [account.id, account]))
      useStore.setState({ accounts: ids.map((id) => byId.get(id)).filter((account): account is Account => Boolean(account)) })
      try {
        await api.reorderAccounts(ids)
        toast('success', t('New order saved'))
      } catch {
        toast('error', t('Could not save the new order'))
      } finally {
        void loadAccounts(true)
      }
    },
    [loadAccounts, toast, t],
  )

  const onCardSelect = useCallback(
    (id: number) => {
      select(id)
      setFocusedId((current) => current ?? id)
    },
    [select],
  )
  const onCardCopy = useCallback((message: string) => toast('success', message), [toast])
  const onCardFavorite = useCallback((id: number) => void toggleFavorite(id), [toggleFavorite])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement
      if (target && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) {
        if (event.key === 'Escape') (target as HTMLInputElement).blur()
        return
      }
      if (event.metaKey || event.ctrlKey) return
      const index = visible.findIndex((account) => account.id === selected?.id)
      const step = (delta: number) => {
        if (!visible.length) return
        const next = visible[(index + delta + visible.length) % visible.length].id
        select(next)
        if (focusedId !== null) setFocusedId(next)
      }
      switch (event.key) {
        case 'ArrowRight':
          step(1)
          break
        case 'ArrowLeft':
          step(-1)
          break
        case 'Escape':
          setFocusedId(null)
          break
        case 'c':
          void copy(selected)
          break
        case 'f':
          if (selected) void toggleFavorite(selected.id)
          break
        case 'e':
          if (selected) setEditing(selected)
          break
        case 'h':
          void hotp(selected)
          break
        case 'n':
          setCreating(true)
          break
        case 'r':
          setAutoRotate((value) => !value)
          break
        case '/':
          event.preventDefault()
          searchRef.current?.focus()
          break
        case 'g':
          setGroupsOpen(true)
          break
        default:
          break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  const cardFor = (account: Account) => (
    <AccountCard
      key={account.id}
      account={account}
      selected={account.id === selected?.id}
      dots={dots}
      showNext={showNext}
      showIcon={showIcons}
      onSelect={is3D ? onCardSelect : select}
      onCopy={onCardCopy}
      onFavorite={onCardFavorite}
      onOpen={setEditing}
    />
  )

  const focusIndex = focusedId === null ? -1 : visible.findIndex((account) => account.id === focusedId)

  return (
    <div className="stage">
      <div className="stage-inner">
        <div className="chips-bar">
          <button className={`chip${groupFilter === 'all' ? ' active' : ''}`} onClick={() => setGroupFilter('all')}>
            {t('All')} <span className="count">{accounts.length}</span>
          </button>
          <button
            className={`chip${groupFilter === 'favorites' ? ' active' : ''}`}
            onClick={() => setGroupFilter('favorites')}
          >
            <Icon name="star" size={13} /> {t('Pinned')}{' '}
            <span className="count">{accounts.filter((account) => account.is_favorite).length}</span>
          </button>
          {groups
            .filter((group) => group.show_in_chips)
            .map((group) => (
              <button
                key={group.id}
                className={`chip${groupFilter === group.id ? ' active' : ''}`}
                onClick={() => setGroupFilter(group.id)}
              >
                {group.name} <span className="count">{group.twofaccounts_count}</span>
              </button>
            ))}

          <div className="right flex">
            <div className="search" style={{ maxWidth: 220 }}>
              <Icon name="search" size={14} />
              <input
                ref={searchRef}
                placeholder={t('Search accounts')}
                value={query}
                onChange={(event) => setQuery(event.target.value)}
              />
            </div>
          </div>
        </div>

        <div className="view-bar">
          <div className="segmented" role="tablist" aria-label={t('Display mode')}>
            {modeMeta.map((item) => (
              <button
                key={item.id}
                role="tab"
                aria-selected={mode === item.id}
                className={mode === item.id ? 'active' : ''}
                onClick={() => setMode(item.id)}
                title={t(item.label)}
              >
                <Icon name={item.icon} size={13} />
                <span>{t(item.label)}</span>
              </button>
            ))}
          </div>

          {is3D && (
            <button
              className={`chip${autoRotate ? ' active' : ''}`}
              onClick={() => setAutoRotate((value) => !value)}
              title={t('Auto-rotate (R)')}
            >
              <Icon name="refresh" size={13} /> {t('Auto-rotate')}
            </button>
          )}

          <button className="chip" onClick={cycleScenery} title={t('Change the 3D scenery')}>
            <Icon name="orbit" size={13} /> {t('Scenery')}: {t(sceneryLabel[background?.style ?? 'grid'])}
          </button>

          <button
            className={`chip${dots ? '' : ' active'}`}
            disabled={locked.includes('showOtpAsDot')}
            aria-pressed={!dots}
            onClick={() => void setPreference('showOtpAsDot', !dots)}
            title={dots ? t('Show every code in clear text') : t('Hide every code again')}
          >
            <Icon name={dots ? 'eye' : 'eye-off'} size={13} /> {dots ? t('Show all codes') : t('Hide all codes')}
          </button>

          {canArrange && (
            <Menu
              label={t('Arrange')}
              items={(
                [
                  ['group', t('By group')],
                  ['name', t('By service name')],
                  ['nearest', t('To the nearest free position')],
                ] as [ArrangeKind, string][]
              ).map(([kind, label]) => ({ label, onClick: () => gridRef.current?.arrange(kind) }))}
            />
          )}

          {is3D && motion.showFps && <FpsMeter />}

          <div className="right flex">
            <button className="btn small ghost" onClick={() => setCreating(true)}>
              <Icon name="plus" size={14} /> {t('New account')}
            </button>
            <button className="btn small ghost" onClick={() => setTransferOpen(true)}>
              <Icon name="download" size={14} /> {t('Import / export')}
            </button>
            <button className="btn small ghost" onClick={() => setGroupsOpen(true)}>
              <Icon name="users" size={14} /> {t('Groups')}
            </button>
          </div>
        </div>

        {visible.length === 0 ? (
          <div className="empty">
            <strong>{t('No matching accounts')}</strong>
            <span>{t('Try another group or clear the search, or create and import accounts.')}</span>
          </div>
        ) : is3D ? (
          <Stage3D
            layout={mode}
            radius={radius}
            count={visible.length}
            autoRotate={autoRotate}
            fps={motion.fps ?? 0}
            glass={motion.glass ?? false}
            focusIndex={focusIndex >= 0 ? focusIndex : null}
            controls={
              hasRadius(mode) && (
                <label className="stage-radius" title={t('Double-click to restore the radius')} onDoubleClick={() => setRadii((current) => ({ ...current, [mode]: 1 }))}>
                  <span>{t('Radius')}</span>
                  <input
                    type="range"
                    min={0.5}
                    max={2.5}
                    step={0.05}
                    value={radius}
                    aria-label={t('Radius')}
                    onChange={(event) => {
                      const value = Number(event.target.value)
                      setRadii((current) => ({ ...current, [mode]: value }))
                    }}
                  />
                  <span className="mono">{Math.round(radius * 100)}%</span>
                </label>
              )
            }
            onBackgroundClick={() => setFocusedId(null)}
            items={visible.map((account) => cardFor(account))}
          />
        ) : mode === 'grid' ? (
          <FreeGrid
            ref={gridRef}
            key={userId}
            accounts={visible}
            groups={groups}
            enabled={canArrange}
            animate={motion.enabled}
            storageKey={`${GRID_KEY}.${userId ?? 0}`}
            renderCard={cardFor}
            onArranged={reorder}
          />
        ) : (
          <div className="list-mode">
            {visible.map((account) => (
              <div
                key={account.id}
                className={`account-row${account.id === selected?.id ? ' selected' : ''}`}
                onClick={() => select(account.id)}
                onDoubleClick={() => setEditing(account)}
              >
                <CardAvatar account={account} showIcon={showIcons} />
                <div className="grow">
                  <strong>{accountLabel(account)}</strong>
                  <div className="hint">{accountSubLabel(account)}</div>
                </div>
                <InlineCode
                  account={account}
                  masked={dots}
                  className="otp-cell"
                  title={t('Click to copy')}
                  onClick={(event) => {
                    event.stopPropagation()
                    void copy(account)
                  }}
                />
                <div className="flex">
                  <button
                    className={`icon-btn${account.is_favorite ? ' on' : ''}`}
                    onClick={(event) => {
                      event.stopPropagation()
                      void toggleFavorite(account.id)
                    }}
                    title={t('Pin')}
                  >
                    <Icon name={account.is_favorite ? 'star-filled' : 'star'} size={14} />
                  </button>
                  <button
                    className="icon-btn"
                    onClick={(event) => {
                      event.stopPropagation()
                      setEditing(account)
                    }}
                    title={t('Details')}
                  >
                    <Icon name="sliders" size={14} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}

        <div className="focus-bar">
          {selected ? (
            <>
              <CardAvatar account={selected} showIcon={showIcons} size={46} />
              <div className="meta grow">
                <strong>{accountLabel(selected)}</strong>
                <span>
                  {accountSubLabel(selected)} · <FocusCountdown account={selected} />
                </span>
              </div>
              <InlineCode
                key={selected.id}
                account={selected}
                masked={dots}
                className="focus-otp"
                title={t('Click to copy')}
                onClick={() => void copy(selected)}
              />
              <div className="flex">
                <button className="btn small primary" onClick={() => void copy(selected)}>
                  <Icon name="copy" size={14} /> {t('Copy')}
                </button>
                {selected.otp_type === 'hotp' && (
                  <button className="btn small" onClick={() => void hotp(selected)}>
                    <Icon name="refresh" size={14} /> {t('Next')}
                  </button>
                )}
                <button className="btn small ghost" onClick={() => setEditing(selected)}>
                  <Icon name="sliders" size={14} /> {t('Details')}
                </button>
              </div>
            </>
          ) : (
            <span className="hint">{t('No accounts yet.')}</span>
          )}
        </div>

        <div className="chips-bar" style={{ paddingBottom: 10, paddingTop: 0 }}>
          <span className="kbd-hint">
            <span>
              <kbd>←</kbd> <kbd>→</kbd> {t('Switch account')}
            </span>
            <span>
              <kbd>C</kbd> {t('Copy')}
            </span>
            <span>
              <kbd>R</kbd> {t('Auto-rotate')}
            </span>
            <span>
              <kbd>F</kbd> {t('Pin')}
            </span>
            <span>
              <kbd>E</kbd> {t('Details')}
            </span>
            <span>
              <kbd>/</kbd> {t('Search')}
            </span>
            {canArrange && <span>{t('Drag a card anywhere; drag its text to select it')}</span>}
          </span>
        </div>
      </div>

      {(creating || editing) && (
        <AccountModal
          account={editing}
          groups={groups}
          onClose={() => {
            setCreating(false)
            setEditing(null)
          }}
          onSaved={() => loadAccounts(true)}
        />
      )}

      {transferOpen && (
        <ImportExportModal
          accounts={accounts}
          onClose={() => setTransferOpen(false)}
          onImported={() => loadAccounts(true)}
        />
      )}

      {groupsOpen && (
        <GroupsModal
          groups={groups}
          accounts={accounts}
          onClose={() => setGroupsOpen(false)}
          onChanged={() => loadGroups()}
          onAssigned={() => Promise.all([loadGroups(), loadAccounts(true)]).then(() => undefined)}
        />
      )}
    </div>
  )
}
