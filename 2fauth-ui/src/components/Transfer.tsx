import { useState } from 'react'
import { api } from '../api/client'
import type { Account, ApiError, Group } from '../api/types'
import { download, stamp } from '../lib/format'
import { useStore } from '../store/useStore'
import Icon from './Icon'

interface TransferProps {
  accounts: Account[]
  onClose: () => void
  onImported: () => Promise<void> | void
}

/** Import accounts from any supported export, or download a new export. */
export function ImportExportModal({ accounts, onClose, onImported }: TransferProps) {
  const toast = useStore((state) => state.toast)
  const [payload, setPayload] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [progress, setProgress] = useState('')
  const [selected, setSelected] = useState<number[]>(accounts.map((account) => account.id))

  async function readFile(file: File) {
    if (file.type.startsWith('image/')) {
      setBusy(true)
      try {
        const decoded = await api.decodeQrCode(file)
        setPayload(decoded.data)
        toast('success', 'QR code decoded')
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
      setError('Paste an export or pick a file first')
      return
    }
    setBusy(true)
    setError('')
    try {
      const migrated = await api.importAccounts(payload.trim())
      const fresh = migrated.filter((account) => account.id < 0 && account.id !== -1)
      const duplicates = migrated.filter((account) => account.id === -1)
      setProgress(`Found ${migrated.length} accounts (${duplicates.length} already known) — saving…`)

      let saved = 0
      for (const account of fresh) {
        await api.createAccount({
          otp_type: account.otp_type,
          service: account.service ?? '',
          account: account.account ?? '',
          secret: account.secret,
          digits: account.digits,
          algorithm: account.algorithm,
          period: account.period ?? undefined,
          counter: account.counter ?? undefined,
        })
        saved += 1
        setProgress(`Saved ${saved}/${fresh.length}…`)
      }
      toast('success', `${saved} accounts imported`)
      await onImported()
      onClose()
    } catch (caught) {
      setError((caught as ApiError).message)
    } finally {
      setBusy(false)
      setProgress('')
    }
  }

  async function exportJson() {
    if (selected.length === 0) return
    const blob = await api.exportBlob(selected)
    download(`2fauth_export_${stamp()}.json`, blob as unknown as string, 'application/json')
    toast('success', 'Export downloaded')
  }

  async function exportOtpauth() {
    if (selected.length === 0) return
    const items = (await api.exportAccounts(selected, true)) as { uri: string }[]
    download(`2fauth_export_${stamp()}.txt`, items.map((item) => item.uri).join('\n'), 'text/plain')
    toast('success', 'otpauth export downloaded')
  }

  return (
    <div className="overlay" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div className="modal">
        <div className="modal-head">
          <h3>Import / export</h3>
          <button className="btn icon ghost close" onClick={onClose} aria-label="Close">
            <Icon name="close" />
          </button>
        </div>
        <div className="modal-body stack">
          <div>
            <div className="field">
              <label>Import</label>
              <textarea
                value={payload}
                onChange={(event) => setPayload(event.target.value)}
                placeholder="Paste a 2FAuth JSON export, an Aegis/2FAS JSON, a list of otpauth:// URIs or an otpauth-migration URI"
              />
            </div>
            <div className="flex wrap">
              <label className="btn small" style={{ cursor: 'pointer' }}>
                <Icon name="upload" size={14} /> Load file
                <input
                  type="file"
                  accept=".json,.txt,.csv,image/*"
                  hidden
                  onChange={(event) => {
                    const file = event.target.files?.[0]
                    if (file) void readFile(file)
                  }}
                />
              </label>
              <button className="btn small primary" onClick={runImport} disabled={busy}>
                Import now
              </button>
              {progress && <span className="hint">{progress}</span>}
            </div>
          </div>

          <div style={{ borderTop: '1px solid var(--border)', paddingTop: 16 }}>
            <div className="field">
              <label>Export {selected.length} of {accounts.length} accounts</label>
              <div className="flex wrap">
                {accounts.map((account) => (
                  <button
                    key={account.id}
                    className={`chip${selected.includes(account.id) ? ' active' : ''}`}
                    onClick={() =>
                      setSelected((current) =>
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
            </div>
            <div className="flex wrap">
              <button className="btn small" onClick={exportJson} disabled={selected.length === 0}>
                <Icon name="download" size={14} /> 2FAuth JSON
              </button>
              <button className="btn small" onClick={exportOtpauth} disabled={selected.length === 0}>
                <Icon name="download" size={14} /> otpauth list
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
  onClose: () => void
  onChanged: () => Promise<void> | void
}

/** Manage groups and their visibility in the chip bar. */
export function GroupsModal({ groups, onClose, onChanged }: GroupsProps) {
  const toast = useStore((state) => state.toast)
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)

  async function create() {
    if (!name.trim()) return
    setBusy(true)
    try {
      await api.createGroup(name.trim())
      setName('')
      await onChanged()
      toast('success', 'Group created')
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  async function rename(group: Group) {
    const next = window.prompt('Group name', group.name)
    if (!next || next === group.name) return
    await api.updateGroup(group.id, next, group.show_in_chips)
    await onChanged()
  }

  async function toggleChips(group: Group) {
    await api.updateGroup(group.id, group.name, !group.show_in_chips)
    await onChanged()
  }

  async function remove(group: Group) {
    if (!window.confirm(`Delete the group "${group.name}"? Accounts are kept.`)) return
    await api.deleteGroup(group.id)
    await onChanged()
    toast('success', 'Group deleted')
  }

  return (
    <div className="overlay" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div className="modal">
        <div className="modal-head">
          <h3>Groups</h3>
          <button className="btn icon ghost close" onClick={onClose} aria-label="Close">
            <Icon name="close" />
          </button>
        </div>
        <div className="modal-body stack">
          <div className="flex">
            <input
              className="input grow"
              placeholder="New group name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              onKeyDown={(event) => event.key === 'Enter' && void create()}
            />
            <button className="btn primary" onClick={create} disabled={busy || !name.trim()}>
              <Icon name="plus" size={14} /> Add
            </button>
          </div>

          {groups.length === 0 ? (
            <p className="hint">No groups yet.</p>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Accounts</th>
                  <th>Chip</th>
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
                        <button className="btn small ghost" onClick={() => void rename(group)}>
                          Rename
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
      </div>
    </div>
  )
}
