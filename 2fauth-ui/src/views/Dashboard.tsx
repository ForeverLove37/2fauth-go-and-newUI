import { useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../api/client'
import type { Account } from '../api/types'
import AccountModal from '../components/AccountModal'
import { AccountCard } from '../components/AccountCard'
import { GroupsModal, ImportExportModal } from '../components/Transfer'
import Icon from '../components/Icon'
import { accountLabel, accountSubLabel, copyText, displayCode, initials, remainingSeconds } from '../lib/format'
import { usePolling, useTick } from '../lib/hooks'
import Stage3D from '../stage/Stage3D'
import type { StageLayout } from '../stage/layouts'
import { useStore } from '../store/useStore'
import { useThemeStore } from '../theme/store'

type ViewMode = StageLayout | 'hybrid' | 'grid' | 'list'

const MODE_KEY = '2fauth.mode.v1'
const ROTATE_KEY = '2fauth.autorotate.v1'

const modeMeta: { id: ViewMode; label: string; icon: 'orbit' | 'grid' | 'share' | 'qr' | 'sliders' | 'eye' | 'copy' }[] = [
  { id: 'orbit', label: '环绕', icon: 'orbit' },
  { id: 'helix', label: '螺旋', icon: 'share' },
  { id: 'wall', label: '卡墙', icon: 'grid' },
  { id: 'sphere', label: '球面', icon: 'eye' },
  { id: 'hybrid', label: '混合', icon: 'qr' },
  { id: 'grid', label: '网格', icon: 'sliders' },
  { id: 'list', label: '列表', icon: 'copy' },
]

const modes: ViewMode[] = ['orbit', 'helix', 'wall', 'sphere', 'hybrid', 'grid', 'list']

function loadMode(): ViewMode {
  const stored = localStorage.getItem(MODE_KEY) as ViewMode | null
  if (stored && modes.includes(stored)) return stored
  return 'orbit'
}

/** The account panel: several 3D layouts plus hybrid and 2D views sharing the same cards. */
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
  const themeAutoRotate = useThemeStore((state) => state.theme.motion.autoRotate)

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

  const selected = visible.find((account) => account.id === selectedId) ?? visible[0] ?? null

  useEffect(() => {
    if (selected && selected.id !== selectedId) select(selected.id)
  }, [selected, selectedId, select])

  async function copy(account: Account | null) {
    if (!account?.otp?.password) return
    const ok = await copyText(account.otp.password)
    toast(ok ? 'success' : 'error', ok ? `已复制 ${accountLabel(account)} 验证码` : '复制失败')
  }

  async function hotp(account: Account | null) {
    if (!account || account.otp_type !== 'hotp') return
    try {
      const payload = await api.otp(account.id)
      await copyText(payload.password)
      toast('success', `新计数器验证码 ${payload.password}`)
      await loadAccounts(true)
    } catch {
      toast('error', '无法生成新验证码')
    }
  }

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

  return (
    <div className="stage">
      <div className="stage-inner">
        <div className="chips-bar">
          <button className={`chip${groupFilter === 'all' ? ' active' : ''}`} onClick={() => setGroupFilter('all')}>
            全部 <span className="count">{accounts.length}</span>
          </button>
          <button
            className={`chip${groupFilter === 'favorites' ? ' active' : ''}`}
            onClick={() => setGroupFilter('favorites')}
          >
            <Icon name="star" size={13} /> 置顶{' '}
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
                placeholder="搜索账号"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
              />
            </div>
          </div>
        </div>

        <div className="view-bar">
          <div className="segmented" role="tablist" aria-label="显示模式">
            {modeMeta.map((item) => (
              <button
                key={item.id}
                role="tab"
                aria-selected={mode === item.id}
                className={mode === item.id ? 'active' : ''}
                onClick={() => setMode(item.id)}
                title={item.id === 'hybrid' ? '3D 舞台 + 2D 网格并列' : undefined}
              >
                <Icon name={item.icon} size={13} />
                <span>{item.label}</span>
              </button>
            ))}
          </div>

          {(is3D || mode === 'hybrid') && (
            <button
              className={`chip${autoRotate ? ' active' : ''}`}
              onClick={() => setAutoRotate((value) => !value)}
              title="自动旋转 (R)"
            >
              <Icon name="refresh" size={13} /> 自动旋转
            </button>
          )}

          <div className="right flex">
            <button className="btn small ghost" onClick={() => setCreating(true)}>
              <Icon name="plus" size={14} /> 新账号
            </button>
            <button className="btn small ghost" onClick={() => setTransferOpen(true)}>
              <Icon name="download" size={14} /> 导入/导出
            </button>
            <button className="btn small ghost" onClick={() => setGroupsOpen(true)}>
              <Icon name="users" size={14} /> 分组
            </button>
          </div>
        </div>

        {visible.length === 0 ? (
          <div className="empty">
            <strong>没有匹配的账号</strong>
            <span>换个分组或清空搜索，也可以新建账号或导入导出文件。</span>
          </div>
        ) : is3D ? (
          <Stage3D
            layout={mode}
            count={visible.length}
            autoRotate={autoRotate}
            items={visible.map((account) => cardFor(account))}
          />
        ) : mode === 'hybrid' ? (
          <div className="hybrid">
            <div className="hybrid-stage">
              <Stage3D
                layout="wall"
                count={visible.length}
                autoRotate={autoRotate}
                dim={0.25}
                items={visible.map((account) => cardFor(account))}
              />
            </div>
            <div className="hybrid-grid">
              {visible.map((account) => (
                <div className="grid2d-cell" key={account.id}>
                  {cardFor(account)}
                </div>
              ))}
            </div>
          </div>
        ) : mode === 'grid' ? (
          <div className="grid2d">
            {visible.map((account) => (
              <div className="grid2d-cell" key={account.id}>
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
                  {accountSubLabel(selected)} ·{' '}
                  {selected.otp_type === 'hotp' ? 'counter' : `${remainingSeconds(selected, now)}s`}
                </span>
              </div>
              <button className={`focus-otp${dots ? ' dots' : ''}`} onClick={() => void copy(selected)} title="点击复制">
                {displayCode(selected, false, true)}
              </button>
              <div className="flex">
                <button className="btn small primary" onClick={() => void copy(selected)}>
                  <Icon name="copy" size={14} /> 复制
                </button>
                {selected.otp_type === 'hotp' && (
                  <button className="btn small" onClick={() => void hotp(selected)}>
                    <Icon name="refresh" size={14} /> 下一个
                  </button>
                )}
                <button className="btn small ghost" onClick={() => setEditing(selected)}>
                  <Icon name="sliders" size={14} /> 详情
                </button>
              </div>
            </>
          ) : (
            <span className="hint">还没有账号。</span>
          )}
        </div>

        <div className="chips-bar" style={{ paddingBottom: 10, paddingTop: 0 }}>
          <span className="kbd-hint">
            <span>
              <kbd>←</kbd> <kbd>→</kbd> 切换账号
            </span>
            <span>
              <kbd>C</kbd> 复制
            </span>
            <span>
              <kbd>R</kbd> 自动旋转
            </span>
            <span>
              <kbd>F</kbd> 置顶
            </span>
            <span>
              <kbd>E</kbd> 详情
            </span>
            <span>
              <kbd>/</kbd> 搜索
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
