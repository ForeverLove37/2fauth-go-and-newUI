import { useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../api/client'
import type { Account } from '../api/types'
import AccountModal from '../components/AccountModal'
import { AccountCard } from '../components/AccountCard'
import { GroupsModal, ImportExportModal } from '../components/Transfer'
import Icon from '../components/Icon'
import { useT } from '../i18n'
import { accountLabel, accountSubLabel, copyText, displayCode, initials, remainingSeconds } from '../lib/format'
import { usePolling, useTick } from '../lib/hooks'
import Stage3D from '../stage/Stage3D'
import type { StageLayout } from '../stage/layouts'
import { useStore } from '../store/useStore'
import { useThemeStore } from '../theme/store'
import type { BackgroundStyle } from '../theme/types'

type ViewMode = StageLayout | 'grid' | 'list'

const MODE_KEY = '2fauth.mode.v1'
const ROTATE_KEY = '2fauth.autorotate.v1'

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

function loadMode(): ViewMode {
  const stored = localStorage.getItem(MODE_KEY) as ViewMode | null
  if (stored && modes.includes(stored)) return stored
  return 'orbit'
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
  const themeAutoRotate = useThemeStore((state) => state.theme.motion.autoRotate)
  const background = useThemeStore((state) => state.theme.background)
  const patchBackground = useThemeStore((state) => state.patchBackground)

  const now = useTick(250)
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
  const [draft, setDraft] = useState<number[] | null>(null)
  const dragRef = useRef<{ id: number; startX: number; startY: number; moved: boolean } | null>(null)
  const searchRef = useRef<HTMLInputElement>(null)

  const dots = Boolean(prefs.showOtpAsDot)
  const showNext = prefs.showNextOtp !== false

  useEffect(() => {
    localStorage.setItem(MODE_KEY, mode)
  }, [mode])

  useEffect(() => {
    localStorage.setItem(ROTATE_KEY, autoRotate ? '1' : '0')
  }, [autoRotate])

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

  // the grid is reorderable, so it renders from a draft order while dragging
  const ordered = useMemo(() => {
    if (!draft) return visible
    const byId = new Map(visible.map((account) => [account.id, account]))
    const result = draft.map((id) => byId.get(id)).filter((account): account is Account => Boolean(account))
    for (const account of visible) if (!draft.includes(account.id)) result.push(account)
    return result
  }, [draft, visible])

  const selected = visible.find((account) => account.id === selectedId) ?? visible[0] ?? null
  const canReorder = mode === 'grid' && groupFilter === 'all' && query.trim() === '' && visible.length > 1

  useEffect(() => {
    if (selected && selected.id !== selectedId) select(selected.id)
  }, [selected, selectedId, select])

  async function copy(account: Account | null) {
    if (!account?.otp?.password) return
    const ok = await copyText(account.otp.password)
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
      toast('success', t('New counter code {code}', { code: payload.password }))
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

  // drag to reorder the grid, like moving icons on a home screen
  function beginDrag(event: React.PointerEvent, account: Account) {
    if (!canReorder || (event.pointerType === 'mouse' && event.button !== 0)) return
    dragRef.current = { id: account.id, startX: event.clientX, startY: event.clientY, moved: false }
    setDraft(visible.map((item) => item.id))
  }

  useEffect(() => {
    if (!draft) return

    function onMove(event: PointerEvent) {
      const drag = dragRef.current
      if (!drag) return
      if (!drag.moved && Math.abs(event.clientX - drag.startX) + Math.abs(event.clientY - drag.startY) < 8) return
      drag.moved = true
      const target = document
        .elementFromPoint(event.clientX, event.clientY)
        ?.closest('[data-account-id]') as HTMLElement | null
      if (!target) return
      const targetId = Number(target.dataset.accountId)
      if (!targetId || targetId === drag.id) return
      setDraft((current) => {
        if (!current) return current
        const from = current.indexOf(drag.id)
        const to = current.indexOf(targetId)
        if (from < 0 || to < 0 || from === to) return current
        const next = [...current]
        next.splice(from, 1)
        next.splice(to, 0, drag.id)
        return next
      })
    }

    async function onUp() {
      const drag = dragRef.current
      dragRef.current = null
      const order = draft
      setDraft(null)
      if (!drag?.moved || !order) return
      try {
        await api.reorderAccounts(order)
        toast('success', t('New order saved'))
      } catch {
        toast('error', t('Could not save the new order'))
      } finally {
        void loadAccounts(true)
      }
    }

    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onUp)
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onUp)
    }
  }, [draft, loadAccounts, toast, t])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement
      if (target && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) {
        if (event.key === 'Escape') (target as HTMLInputElement).blur()
        return
      }
      if (event.metaKey || event.ctrlKey) return
      const index = visible.findIndex((account) => account.id === selected?.id)
      switch (event.key) {
        case 'ArrowRight':
          if (visible.length) select(visible[(index + 1 + visible.length) % visible.length].id)
          break
        case 'ArrowLeft':
          if (visible.length) select(visible[(index - 1 + visible.length) % visible.length].id)
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
      now={now}
      dots={dots}
      showNext={showNext}
      onSelect={() => select(account.id)}
      onCopy={(message) => toast('success', message)}
      onFavorite={() => void toggleFavorite(account.id)}
      onOpen={() => setEditing(account)}
    />
  )

  const is3D = mode === 'orbit' || mode === 'helix' || mode === 'wall' || mode === 'sphere'
  const dragging = draft !== null && dragRef.current?.moved === true

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
            count={visible.length}
            autoRotate={autoRotate}
            items={visible.map((account) => cardFor(account))}
          />
        ) : mode === 'grid' ? (
          <div className={`grid2d${canReorder ? ' reorderable' : ''}${dragging ? ' dragging' : ''}`}>
            {ordered.map((account) => (
              <div
                className={`grid2d-cell${draft?.includes(account.id) ? ' draggable' : ''}`}
                key={account.id}
                data-account-id={account.id}
                onPointerDown={(event) => beginDrag(event, account)}
              >
                {cardFor(account)}
              </div>
            ))}
          </div>
        ) : (
          <div className="list-mode">
            {visible.map((account) => (
              <div
                key={account.id}
                className={`account-row${account.id === selected?.id ? ' selected' : ''}`}
                onClick={() => select(account.id)}
                onDoubleClick={() => setEditing(account)}
              >
                <div className="card-avatar">
                  {account.icon ? <img src={api.iconUrl(account.icon)} alt="" /> : initials(account)}
                </div>
                <div className="grow">
                  <strong>{accountLabel(account)}</strong>
                  <div className="hint">{accountSubLabel(account)}</div>
                </div>
                <div className="otp-cell">{account.otp?.password ?? '——'}</div>
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
              <div className="card-avatar" style={{ width: 46, height: 46 }}>
                {selected.icon ? <img src={api.iconUrl(selected.icon)} alt="" /> : initials(selected)}
              </div>
              <div className="meta grow">
                <strong>{accountLabel(selected)}</strong>
                <span>
                  {accountSubLabel(selected)} ·{' '}
                  {selected.otp_type === 'hotp' ? t('counter') : `${remainingSeconds(selected, now)}s`}
                </span>
              </div>
              <button className={`focus-otp${dots ? ' dots' : ''}`} onClick={() => void copy(selected)} title={t('Click to copy')}>
                {displayCode(selected, false, true)}
              </button>
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
            {canReorder && <span>{t('Drag cards to reorder')}</span>}
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
        <GroupsModal groups={groups} onClose={() => setGroupsOpen(false)} onChanged={() => loadGroups()} />
      )}
    </div>
  )
}
