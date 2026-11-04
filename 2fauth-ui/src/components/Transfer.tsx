import { useMemo, useState } from 'react'
import { api, type ImportResult } from '../api/client'
import type { Account, ApiError, Group } from '../api/types'
import { useT } from '../i18n'
import { accountLabel, accountSubLabel, download, stamp } from '../lib/format'
import { useStore } from '../store/useStore'
import Icon from './Icon'

interface TransferProps {
  accounts: Account[]
  onClose: () => void
  onImported: () => Promise<void> | void
}

/** Import accounts from any supported export, or download a new export. */
export function ImportExportModal({ accounts, onClose, onImported }: TransferProps) {
  const t = useT()
  const toast = useStore((state) => state.toast)
  // Only the owner can export an account: a share only grants generating its codes.
  const exportable = accounts.filter((account) => !account.is_borrowed)
  const [payload, setPayload] = useState('')
  const [importPassphrase, setImportPassphrase] = useState('')
  const [needsPassphrase, setNeedsPassphrase] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [issues, setIssues] = useState<ImportResult['issues']>(null)
  // What is left out, rather than what is chosen: accounts imported while this dialog
  // is open are part of the export by default.
  const [excluded, setExcluded] = useState<number[]>([])
  const selected = exportable.map((account) => account.id).filter((id) => !excluded.includes(id))
  const [passphrase, setPassphrase] = useState('')
  const [confirmation, setConfirmation] = useState('')

  async function readFile(file: File) {
    setNeedsPassphrase(false)
    setImportPassphrase('')
    if (file.type.startsWith('image/')) {
      setBusy(true)
      try {
        const decoded = await api.decodeQrCode(file)
        setPayload(decoded.data)
        toast('success', t('QR code decoded'))
      } catch (caught) {
        setError((caught as ApiError).message)
      } finally {
        setBusy(false)
      }
      return
    }
    setPayload(await file.text())
  }

  async function runImport() {
    if (!payload.trim()) {
      setError(t('Paste an export or pick a file first'))
      return
    }
    setBusy(true)
    setError('')
    setIssues(null)
    try {
      const result = await api.importPayload(payload.trim(), importPassphrase)
      setNeedsPassphrase(false)
      toast(
        'success',
        t('{imported} accounts imported, {duplicates} already known, {invalid} not usable', {
          imported: result.imported,
          duplicates: result.duplicates,
          invalid: result.invalid,
        }),
      )
      await onImported()
      if (result.invalid > 0) {
        setIssues(result.issues)
        return
      }
      onClose()
    } catch (caught) {
      const typed = caught as ApiError
      if (typed.needsPassphrase) setNeedsPassphrase(true)
      setError(typed.needsPassphrase ? t('This file is encrypted: enter its passphrase') : typed.message)
    } finally {
      setBusy(false)
    }
  }

  async function exportEncrypted() {
    if (selected.length === 0) return
    setBusy(true)
    setError('')
    try {
      const file = await api.exportEncrypted(selected, passphrase)
      download(`2fauth_export_${stamp()}.encrypted.json`, JSON.stringify(file, null, 2), 'application/json')
      setPassphrase('')
      setConfirmation('')
      toast('success', t('Encrypted export downloaded'))
    } catch (caught) {
      setError((caught as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  async function exportJson() {
    if (selected.length === 0 || !window.confirm(t('This file will contain your secrets in clear text. Continue?'))) return
    const blob = await api.exportBlob(selected)
    download(`2fauth_export_${stamp()}.json`, blob as unknown as string, 'application/json')
    toast('success', t('Export downloaded'))
  }

  async function exportOtpauth() {
    if (selected.length === 0 || !window.confirm(t('This file will contain your secrets in clear text. Continue?'))) return
    const items = (await api.exportAccounts(selected, true)) as { uri: string }[]
    download(`2fauth_export_${stamp()}.txt`, items.map((item) => item.uri).join('\n'), 'text/plain')
    toast('success', t('otpauth export downloaded'))
  }

  const passphraseOk = passphrase.length >= 12 && passphrase === confirmation

  return (
    <div className="overlay" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div className="modal">
        <div className="modal-head">
          <h3>{t('Import / export')}</h3>
          <button className="btn icon ghost close" onClick={onClose} aria-label={t('Close')}>
            <Icon name="close" />
          </button>
        </div>
        <div className="modal-body stack">
          <div>
            <div className="field">
              <label htmlFor="import-payload">{t('Import')}</label>
              <textarea
                id="import-payload"
                value={payload}
                onChange={(event) => {
                  setPayload(event.target.value)
                  setNeedsPassphrase(false)
                }}
                placeholder={t(
                  'Paste an export or load a file: 2FAuth, Aegis, 2FAS, Google Authenticator, Bitwarden, andOTP, FreeOTP+, Authenticator Pro, Raivo, LastPass, Proton, CSV or a list of otpauth:// URIs',
                )}
              />
            </div>
            {needsPassphrase && (
              <div className="field">
                <label htmlFor="import-passphrase">{t('Passphrase of the file')}</label>
                <input
                  id="import-passphrase"
                  className="input"
                  type="password"
                  autoComplete="off"
                  autoFocus
                  value={importPassphrase}
                  onChange={(event) => setImportPassphrase(event.target.value)}
                  onKeyDown={(event) => event.key === 'Enter' && void runImport()}
                />
              </div>
            )}
            <div className="flex wrap">
              <label className="btn small" style={{ cursor: 'pointer' }}>
                <Icon name="upload" size={14} /> {t('Load file')}
                <input
                  type="file"
                  accept=".json,.txt,.csv,.2fas,.aes,image/*"
                  hidden
                  onChange={(event) => {
                    const file = event.target.files?.[0]
                    if (file) void readFile(file)
                  }}
                />
              </label>
              <button className="btn small primary" onClick={runImport} disabled={busy}>
                {busy ? t('Working…') : t('Import now')}
              </button>
            </div>
            {issues && issues.length > 0 && (
              <div style={{ marginTop: 10 }}>
                <p className="hint">{t('These entries could not be used:')}</p>
                <ul className="hint" style={{ margin: '4px 0 0 18px' }}>
                  {issues.map((issue) => (
                    <li key={issue.index}>
                      {issue.service || `#${issue.index + 1}`} — {issue.reason}
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>

          <div style={{ borderTop: '1px solid var(--border)', paddingTop: 16 }}>
            <div className="field">
              <label>
                {t('Export {selected} of {total} accounts', { selected: selected.length, total: exportable.length })}
              </label>
              <div className="flex wrap">
                {exportable.map((account) => (
                  <button
                    key={account.id}
                    className={`chip${selected.includes(account.id) ? ' active' : ''}`}
                    onClick={() =>
                      setExcluded((current) =>
                        current.includes(account.id)
                          ? current.filter((id) => id !== account.id)
                          : [...current, account.id],
                      )
                    }
                  >
                    {account.service || account.account}
                  </button>
                ))}
              </div>
              {exportable.length < accounts.length && (
                <span className="hint">{t('Accounts shared with you cannot be exported.')}</span>
              )}
            </div>
            <div className="row">
              <div className="field">
                <label htmlFor="export-passphrase">{t('Passphrase, 12 characters or more')}</label>
                <input
                  id="export-passphrase"
                  className="input"
                  type="password"
                  autoComplete="new-password"
                  value={passphrase}
                  onChange={(event) => setPassphrase(event.target.value)}
                />
              </div>
              <div className="field">
                <label htmlFor="export-confirmation">{t('Confirm')}</label>
                <input
                  id="export-confirmation"
                  className="input"
                  type="password"
                  autoComplete="new-password"
                  value={confirmation}
                  onChange={(event) => setConfirmation(event.target.value)}
                />
              </div>
            </div>
            <p className="hint">{t('Keep this passphrase safe: without it the file cannot be opened by anyone.')}</p>
            <div className="flex wrap" style={{ marginTop: 10 }}>
              <button className="btn small primary" onClick={exportEncrypted} disabled={busy || !passphraseOk || selected.length === 0}>
                <Icon name="lock" size={14} /> {t('Encrypted export')}
              </button>
              <button className="btn small ghost" onClick={exportJson} disabled={selected.length === 0}>
                <Icon name="download" size={14} /> {t('Unencrypted JSON')}
              </button>
              <button className="btn small ghost" onClick={exportOtpauth} disabled={selected.length === 0}>
                <Icon name="download" size={14} /> {t('otpauth list')}
              </button>
            </div>
          </div>

          {error && <p className="error-text">{error}</p>}
        </div>
      </div>
    </div>
  )
}

interface GroupsProps {
  groups: Group[]
  accounts: Account[]
  onClose: () => void
  onChanged: () => Promise<void> | void
  /** accounts have moved between groups */
  onAssigned: () => Promise<void> | void
}

/** Picks the accounts to add to a group, all at once. */
function AddToGroup({
  group,
  groups,
  accounts,
  onBack,
  onAssigned,
}: {
  group: Group
  groups: Group[]
  accounts: Account[]
  onBack: () => void
  onAssigned: GroupsProps['onAssigned']
}) {
  const t = useT()
  const toast = useStore((state) => state.toast)
  const [query, setQuery] = useState('')
  const [picked, setPicked] = useState<number[]>([])
  const [busy, setBusy] = useState(false)

  const names = useMemo(() => new Map(groups.map((item) => [item.id, item.name])), [groups])
  // Accounts shared with this user belong to someone else's groups.
  const candidates = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return accounts.filter(
      (account) =>
        !account.is_borrowed &&
        account.group_id !== group.id &&
        `${account.service ?? ''} ${account.account ?? ''}`.toLowerCase().includes(needle),
    )
  }, [accounts, group.id, query])
  const allPicked = candidates.length > 0 && candidates.every((account) => picked.includes(account.id))

  function toggle(id: number) {
    setPicked((current) => (current.includes(id) ? current.filter((item) => item !== id) : [...current, id]))
  }

  function toggleAll() {
    const shown = candidates.map((account) => account.id)
    setPicked((current) => (allPicked ? current.filter((id) => !shown.includes(id)) : [...new Set([...current, ...shown])]))
  }

  async function add() {
    setBusy(true)
    try {
      await api.assignGroup(group.id, picked)
      await onAssigned()
      toast('success', t('{count} accounts added to {name}', { count: picked.length, name: group.name }))
      onBack()
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="stack">
      <div className="flex">
        <input
          className="input grow"
          placeholder={t('Search accounts')}
          aria-label={t('Search accounts')}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
        <button className="btn small ghost" onClick={toggleAll} disabled={candidates.length === 0}>
          {allPicked ? t('Clear selection') : t('Select all')}
        </button>
      </div>

      {candidates.length === 0 ? (
        <p className="hint">{t('No accounts to add.')}</p>
      ) : (
        <div className="pick-list">
          {candidates.map((account) => {
            const now = account.group_id === null ? undefined : names.get(account.group_id)
            return (
              <label key={account.id} className="pick-row">
                <input type="checkbox" checked={picked.includes(account.id)} onChange={() => toggle(account.id)} />
                <span className="grow">
                  <strong>{accountLabel(account)}</strong>
                  <span className="hint">{accountSubLabel(account)}</span>
                </span>
                {now && <span className="hint">{t('Now in {name}', { name: now })}</span>}
              </label>
            )
          })}
        </div>
      )}

      <p className="hint">{t('An account is in one group at a time: adding it here takes it out of its current group.')}</p>
      <div className="flex" style={{ justifyContent: 'flex-end' }}>
        <button className="btn small ghost" onClick={onBack}>
          {t('Back')}
        </button>
        <button className="btn small primary" onClick={() => void add()} disabled={busy || picked.length === 0}>
          <Icon name="plus" size={14} /> {t('Add {count} accounts', { count: picked.length })}
        </button>
      </div>
    </div>
  )
}

/** Manage groups and their visibility in the chip bar. */
export function GroupsModal({ groups, accounts, onClose, onChanged, onAssigned }: GroupsProps) {
  const t = useT()
  const toast = useStore((state) => state.toast)
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const [addingTo, setAddingTo] = useState<number | null>(null)
  const adding = groups.find((group) => group.id === addingTo)

  async function create() {
    if (!name.trim()) return
    setBusy(true)
    try {
      await api.createGroup(name.trim())
      setName('')
      await onChanged()
      toast('success', t('Group created'))
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  async function rename(group: Group) {
    const next = window.prompt(t('Group name'), group.name)
    if (!next || next === group.name) return
    await api.updateGroup(group.id, next, group.show_in_chips)
    await onChanged()
  }

  async function toggleChips(group: Group) {
    await api.updateGroup(group.id, group.name, !group.show_in_chips)
    await onChanged()
  }

  async function remove(group: Group) {
    if (!window.confirm(t('Delete the group "{name}"? Accounts are kept.', { name: group.name }))) return
    await api.deleteGroup(group.id)
    await onChanged()
    toast('success', t('Group deleted'))
  }

  return (
    <div className="overlay" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div className="modal">
        <div className="modal-head">
          <h3>{adding ? t('Add accounts to {name}', { name: adding.name }) : t('Groups')}</h3>
          <button className="btn icon ghost close" onClick={onClose} aria-label={t('Close')}>
            <Icon name="close" />
          </button>
        </div>
        {adding ? (
          <div className="modal-body">
            <AddToGroup
              key={adding.id}
              group={adding}
              groups={groups}
              accounts={accounts}
              onBack={() => setAddingTo(null)}
              onAssigned={onAssigned}
            />
          </div>
        ) : (
        <div className="modal-body stack">
          <div className="flex">
            <input
              className="input grow"
              placeholder={t('New group name')}
              value={name}
              onChange={(event) => setName(event.target.value)}
              onKeyDown={(event) => event.key === 'Enter' && void create()}
            />
            <button className="btn primary" onClick={create} disabled={busy || !name.trim()}>
              <Icon name="plus" size={14} /> {t('Add')}
            </button>
          </div>

          {groups.length === 0 ? (
            <p className="hint">{t('No groups yet.')}</p>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>{t('Name')}</th>
                  <th>{t('Accounts')}</th>
                  <th>{t('Chip')}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {groups.map((group) => (
                  <tr key={group.id}>
                    <td>{group.name}</td>
                    <td className="mono">{group.twofaccounts_count}</td>
                    <td>
                      <button className={`toggle${group.show_in_chips ? ' on' : ''}`} onClick={() => void toggleChips(group)} />
                    </td>
                    <td>
                      <div className="flex" style={{ justifyContent: 'flex-end' }}>
                        <button className="btn small" onClick={() => setAddingTo(group.id)}>
                          <Icon name="plus" size={13} /> {t('Add accounts')}
                        </button>
                        <button className="btn small ghost" onClick={() => void rename(group)}>
                          {t('Rename')}
                        </button>
                        <button className="btn small danger" onClick={() => void remove(group)}>
                          <Icon name="trash" size={13} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
        )}
      </div>
    </div>
  )
}
