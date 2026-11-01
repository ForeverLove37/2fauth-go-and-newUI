import { useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../api/client'
import type { Account } from '../api/types'
import AccountModal from '../components/AccountModal'
import OrbitRing from '../components/OrbitRing'
import { GroupsModal, ImportExportModal } from '../components/Transfer'
import Icon from '../components/Icon'
import { accountLabel, accountSubLabel, copyText, displayCode, initials, remainingSeconds } from '../lib/format'
import { usePolling, useTick } from '../lib/hooks'
import { useStore } from '../store/useStore'

/** The 3D account panel: orbit view, chip filters, keyboard shortcuts. */
export default function Dashboard() {
  const accounts = useStore((state) => state.accounts)
  const groups = useStore((state) => state.groups)
  const prefs = useStore((state) => state.prefs)
  const selectedId = useStore((state) => state.selectedId)
  const select = useStore((state) => state.select)
  const toast = useStore((state) => state.toast)
  const loadAccounts = useStore((state) => state.loadAccounts)
  const loadGroups = useStore((state) => state.loadGroups)
  const toggleFavorite = useStore((state) => state.toggleFavorite)

  const now = useTick(250)
  const [view, setView] = useState<'orbit' | 'list'>('orbit')
  const [groupFilter, setGroupFilter] = useState<number | 'all' | 'favorites'>('all')
  const [query, setQuery] = useState('')
  const [editing, setEditing] = useState<Account | null>(null)
  const [creating, setCreating] = useState(false)
  const [transferOpen, setTransferOpen] = useState(false)
  const [groupsOpen, setGroupsOpen] = useState(false)
  const searchRef = useRef<HTMLInputElement>(null)

  const dots = Boolean(prefs.showOtpAsDot)
  const showNext = prefs.showNextOtp !== false

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

  useEffect(() => {
    if (selected && selected.id !== selectedId) select(selected.id)
  }, [selected, selectedId, select])

  async function copy(account: Account | null) {
    if (!account?.otp?.password) return
    const ok = await copyText(account.otp.password)
    toast(ok ? 'success' : 'error', ok ? `${accountLabel(account)} code copied` : 'Copy failed')
  }

  async function hotp(account: Account | null) {
    if (!account || account.otp_type !== 'hotp') return
    try {
      const payload = await api.otp(account.id)
      await copyText(payload.password)
      toast('success', `New counter code ${payload.password}`)
      await loadAccounts(true)
    } catch {
      toast('error', 'Unable to generate a new code')
    }
  }

  // Keyboard shortcuts.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement
      if (target && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) {
        if (event.key === 'Escape') (target as HTMLInputElement).blur()
        return
      }
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

  const remaining = selected ? remainingSeconds(selected, now) : 0
  const countdownLabel = selected?.otp_type === 'hotp' ? 'counter' : `${remaining}s`

  return (
    <div className="stage">
      <div className="stage-inner">
        <div className="chips-bar">
          <button className={`chip${groupFilter === 'all' ? ' active' : ''}`} onClick={() => setGroupFilter('all')}>
            All <span className="count">{accounts.length}</span>
          </button>
          <button
            className={`chip${groupFilter === 'favorites' ? ' active' : ''}`}
            onClick={() => setGroupFilter('favorites')}
          >
            <Icon name="star" size={13} /> Pinned{' '}
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
            <div className="search" style={{ maxWidth: 260 }}>
              <Icon name="search" size={14} />
              <input
                ref={searchRef}
                placeholder="Search accounts"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
              />
            </div>
            <button
              className="btn icon ghost"
              title={view === 'orbit' ? 'Switch to list' : 'Switch to orbit'}
              onClick={() => setView(view === 'orbit' ? 'list' : 'orbit')}
            >
              <Icon name={view === 'orbit' ? 'grid' : 'orbit'} />
            </button>
          </div>
        </div>

        {view === 'orbit' ? (
          <OrbitRing
            accounts={visible}
            selectedId={selected?.id ?? null}
            now={now}
            dots={dots}
            showNext={showNext}
            onSelect={select}
            onCopy={(message) => toast('success', message)}
            onFavorite={(id) => void toggleFavorite(id)}
            onOpen={(id) => {
              const account = accounts.find((item) => item.id === id)
              if (account) setEditing(account)
            }}
          />
        ) : (
          <div className="list-mode">
            {visible.length === 0 && <div className="empty">No account matches this filter.</div>}
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
                  >
                    <Icon name={account.is_favorite ? 'star-filled' : 'star'} size={14} />
                  </button>
                  <button
                    className="icon-btn"
                    onClick={(event) => {
                      event.stopPropagation()
                      setEditing(account)
                    }}
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
                  {accountSubLabel(selected)} · {countdownLabel}
                </span>
              </div>
              <button
                className={`focus-otp${dots ? ' dots' : ''}`}
                onClick={() => void copy(selected)}
                title="Click to copy"
              >
                {displayCode(selected, false, true)}
              </button>
              <div className="flex">
                <button className="btn small primary" onClick={() => void copy(selected)}>
                  <Icon name="copy" size={14} /> Copy
                </button>
                {selected.otp_type === 'hotp' && (
                  <button className="btn small" onClick={() => void hotp(selected)}>
                    <Icon name="refresh" size={14} /> Next
                  </button>
                )}
                <button className="btn small" onClick={() => setCreating(true)}>
                  <Icon name="plus" size={14} /> New
                </button>
                <button className="btn small" onClick={() => setTransferOpen(true)}>
                  <Icon name="download" size={14} /> Import / export
                </button>
                <button className="btn small ghost" onClick={() => setGroupsOpen(true)}>
                  <Icon name="users" size={14} /> Groups
                </button>
              </div>
            </>
          ) : (
            <div className="flex grow">
              <span className="hint">No account yet — create one or import an export.</span>
              <div className="right flex">
                <button className="btn small primary" onClick={() => setCreating(true)}>
                  <Icon name="plus" size={14} /> New account
                </button>
                <button className="btn small" onClick={() => setTransferOpen(true)}>
                  <Icon name="upload" size={14} /> Import
                </button>
              </div>
            </div>
          )}
        </div>

        <div className="chips-bar" style={{ paddingBottom: 12, paddingTop: 0 }}>
          <span className="kbd-hint">
            <span>
              <kbd>←</kbd> <kbd>→</kbd> orbit
            </span>
            <span>
              <kbd>C</kbd> copy
            </span>
            <span>
              <kbd>F</kbd> pin
            </span>
            <span>
              <kbd>E</kbd> edit
            </span>
            <span>
              <kbd>N</kbd> new
            </span>
            <span>
              <kbd>/</kbd> search
            </span>
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
