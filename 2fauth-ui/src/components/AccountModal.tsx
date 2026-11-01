import { useEffect, useMemo, useState } from 'react'
import { api } from '../api/client'
import type { Account, ApiError, Group, ShareRecipient } from '../api/types'
import { useStore } from '../store/useStore'
import Icon from './Icon'

interface Props {
  account: Account | null
  groups: Group[]
  onClose: () => void
  onSaved: () => Promise<void> | void
}

const emptyForm = {
  otp_type: 'totp',
  service: '',
  account: '',
  secret: '',
  digits: 6,
  algorithm: 'sha1',
  period: 30,
  counter: 0,
  group_id: '',
}

type Form = typeof emptyForm

/** Create / edit an account, with URI import, QR preview, sharing and deletion. */
export default function AccountModal({ account, groups, onClose, onSaved }: Props) {
  const toast = useStore((state) => state.toast)
  const [form, setForm] = useState<Form>(emptyForm)
  const [uri, setUri] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [fields, setFields] = useState<Record<string, string[]>>({})
  const [qr, setQr] = useState('')
  const [shares, setShares] = useState<ShareRecipient[]>([])
  const [recipients, setRecipients] = useState<ShareRecipient[]>([])
  const [recipientFilter, setRecipientFilter] = useState('')
  const [selectedRecipients, setSelectedRecipients] = useState<number[]>([])
  const isEdit = Boolean(account)

  useEffect(() => {
    if (!account) {
      setForm(emptyForm)
      return
    }
    setForm({
      otp_type: account.otp_type,
      service: account.service ?? '',
      account: account.account ?? '',
      secret: account.secret ?? '',
      digits: account.digits,
      algorithm: account.algorithm,
      period: account.period ?? 30,
      counter: Number(account.counter ?? 0),
      group_id: account.group_id ? String(account.group_id) : '',
    })
    void loadExtras(account)
    // list responses hide the secret, so fetch the full record before editing
    if (!account.secret) {
      void api
        .account(account.id)
        .then((full) => {
          setForm((current) => ({ ...current, secret: full.secret ?? current.secret }))
        })
        .catch(() => {
          /* keep the form as is */
        })
    }
  }, [account])

  async function loadExtras(target: Account) {
    try {
      const qrcode = await api.qrcode(target.id)
      setQr(qrcode.qrcode)
    } catch {
      setQr('')
    }
    try {
      const response = await api.shares(target.id)
      setShares(response.specific_users ?? [])
      setRecipients(await api.recipients(target.id))
    } catch {
      setShares([])
      setRecipients([])
    }
  }

  function update<K extends keyof Form>(key: K, value: Form[K]) {
    setForm((current) => ({ ...current, [key]: value }))
  }

  async function applyUri() {
    if (!uri.trim()) return
    setError('')
    try {
      const preview = await api.preview(uri.trim())
      setForm((current) => ({
        ...current,
        service: preview.service ?? '',
        account: preview.account ?? '',
        secret: preview.secret ?? '',
        digits: preview.digits,
        algorithm: preview.algorithm,
        period: preview.period ?? 30,
        counter: Number(preview.counter ?? 0),
        otp_type: preview.otp_type,
      }))
      toast('success', 'URI parsed')
    } catch (caught) {
      setError((caught as ApiError).message)
    }
  }

  async function decodeQr(file: File) {
    setBusy(true)
    try {
      const response = await api.decodeQrCode(file)
      setUri(response.data)
      await applyUriWith(response.data)
    } catch (caught) {
      setError((caught as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  async function applyUriWith(value: string) {
    try {
      const preview = await api.preview(value)
      setForm((current) => ({
        ...current,
        service: preview.service ?? '',
        account: preview.account ?? '',
        secret: preview.secret ?? '',
        digits: preview.digits,
        algorithm: preview.algorithm,
        period: preview.period ?? 30,
        counter: Number(preview.counter ?? 0),
        otp_type: preview.otp_type,
      }))
    } catch (caught) {
      setError((caught as ApiError).message)
    }
  }

  async function uploadIcon(file: File) {
    setBusy(true)
    try {
      const response = await api.uploadIcon(file)
      toast('success', `Icon ${response.icon} uploaded`)
    } catch (caught) {
      setError((caught as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setFields({})

    const payload: Record<string, unknown> = {
      otp_type: form.otp_type,
      service: form.service,
      account: form.account,
      secret: form.secret,
      digits: Number(form.digits),
      algorithm: form.algorithm,
    }
    if (form.otp_type === 'hotp') payload.counter = Number(form.counter)
    else payload.period = Number(form.period)
    if (form.group_id !== '') payload.group_id = Number(form.group_id)

    try {
      if (account) await api.updateAccount(account.id, payload)
      else await api.createAccount(payload)
      toast('success', account ? 'Account updated' : 'Account added')
      await onSaved()
      onClose()
    } catch (caught) {
      const typed = caught as ApiError
      setError(typed.message)
      setFields(typed.errors ?? {})
    } finally {
      setBusy(false)
    }
  }

  async function toggleRecipient(id: number) {
    setSelectedRecipients((current) =>
      current.includes(id) ? current.filter((item) => item !== id) : [...current, id],
    )
  }

  async function saveShares() {
    if (!account) return
    setBusy(true)
    try {
      if (selectedRecipients.length === 0) await api.unshare(account.id)
      else await api.share(account.id, selectedRecipients)
      toast('success', 'Sharing updated')
      await loadExtras(account)
      setSelectedRecipients([])
    } catch (caught) {
      setError((caught as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  async function shareEveryone() {
    if (!account) return
    setBusy(true)
    try {
      await api.shareWithAll(account.id)
      toast('success', 'Shared with all users')
      await loadExtras(account)
    } catch (caught) {
      setError((caught as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  async function remove() {
    if (!account) return
    if (!window.confirm(`Delete ${account.service || account.account}?`)) return
    setBusy(true)
    try {
      await api.deleteAccount(account.id)
      toast('success', 'Account deleted')
      await onSaved()
      onClose()
    } catch (caught) {
      setError((caught as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  const filteredRecipients = useMemo(() => {
    const needle = recipientFilter.trim().toLowerCase()
    return recipients.filter((recipient) => !needle || recipient.name.toLowerCase().includes(needle))
  }, [recipients, recipientFilter])

  return (
    <div className="overlay" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div className="modal wide">
        <div className="modal-head">
          <h3>{isEdit ? 'Account details' : 'New account'}</h3>
          <button className="btn icon ghost close" onClick={onClose} aria-label="Close">
            <Icon name="close" />
          </button>
        </div>

        <form onSubmit={submit}>
          <div className="modal-body stack">
            {!isEdit && (
              <div className="panel" style={{ padding: 16 }}>
                <div className="field">
                  <label>Paste an otpauth:// URI</label>
                  <div className="flex">
                    <input
                      className="input grow mono"
                      placeholder="otpauth://totp/Issuer:account?secret=…"
                      value={uri}
                      onChange={(event) => setUri(event.target.value)}
                    />
                    <button type="button" className="btn" onClick={applyUri}>
                      Parse
                    </button>
                  </div>
                </div>
                <div className="flex wrap">
                  <label className="btn small ghost" style={{ cursor: 'pointer' }}>
                    <Icon name="qr" size={14} /> Scan image
                    <input
                      type="file"
                      accept="image/*"
                      hidden
                      onChange={(event) => {
                        const file = event.target.files?.[0]
                        if (file) void decodeQr(file)
                      }}
                    />
                  </label>
                  <span className="hint">A screenshot of the QR code works too.</span>
                </div>
              </div>
            )}

            <div className="row">
              <div className="field">
                <label>Type</label>
                <select className="select" value={form.otp_type} onChange={(event) => update('otp_type', event.target.value)}>
                  <option value="totp">TOTP (time based)</option>
                  <option value="hotp">HOTP (counter based)</option>
                  <option value="steamtotp">Steam</option>
                </select>
              </div>
              <div className="field">
                <label>Group</label>
                <select className="select" value={form.group_id} onChange={(event) => update('group_id', event.target.value)}>
                  <option value="">No group</option>
                  {groups.map((group) => (
                    <option key={group.id} value={group.id}>
                      {group.name}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="row">
              <div className="field">
                <label>Service</label>
                <input className="input" value={form.service} onChange={(event) => update('service', event.target.value)} />
                {fields.service && <span className="error-text">{fields.service[0]}</span>}
              </div>
              <div className="field">
                <label>Account</label>
                <input
                  className="input"
                  value={form.account}
                  onChange={(event) => update('account', event.target.value)}
                  required
                />
                {fields.account && <span className="error-text">{fields.account[0]}</span>}
              </div>
            </div>

            <div className="field">
              <label>Secret</label>
              <input
                className="input mono"
                value={form.secret}
                onChange={(event) => update('secret', event.target.value)}
                required
                spellCheck={false}
              />
              {fields.secret && <span className="error-text">{fields.secret[0]}</span>}
            </div>

            <div className="row">
              <div className="field">
                <label>Algorithm</label>
                <select className="select" value={form.algorithm} onChange={(event) => update('algorithm', event.target.value)}>
                  <option value="sha1">SHA1</option>
                  <option value="sha256">SHA256</option>
                  <option value="sha512">SHA512</option>
                  <option value="md5">MD5</option>
                </select>
              </div>
              <div className="field">
                <label>Digits</label>
                <input
                  className="input"
                  type="number"
                  min={5}
                  max={10}
                  value={form.digits}
                  onChange={(event) => update('digits', Number(event.target.value))}
                />
              </div>
              {form.otp_type === 'hotp' ? (
                <div className="field">
                  <label>Counter</label>
                  <input
                    className="input"
                    type="number"
                    min={0}
                    value={form.counter}
                    onChange={(event) => update('counter', Number(event.target.value))}
                  />
                </div>
              ) : (
                <div className="field">
                  <label>Period</label>
                  <input
                    className="input"
                    type="number"
                    min={1}
                    value={form.period}
                    onChange={(event) => update('period', Number(event.target.value))}
                  />
                </div>
              )}
            </div>

            <div className="flex wrap">
              <label className="btn small ghost" style={{ cursor: 'pointer' }}>
                <Icon name="upload" size={14} /> Upload icon
                <input
                  type="file"
                  accept="image/*"
                  hidden
                  onChange={(event) => {
                    const file = event.target.files?.[0]
                    if (file) void uploadIcon(file)
                  }}
                />
              </label>
              {isEdit && qr && (
                <div className="qr-preview" style={{ padding: 10 }}>
                  <img src={qr} alt="Account QR code" style={{ width: 130, height: 130 }} />
                </div>
              )}
            </div>

            {isEdit && account && (
              <div className="panel" style={{ padding: 16 }}>
                <div className="flex" style={{ marginBottom: 10 }}>
                  <strong>Sharing</strong>
                  <span className="hint right">
                    {account.is_borrowed ? `owned by ${account.borrowed_by}` : 'Share the OTP without the secret'}
                  </span>
                </div>
                <div className="flex wrap" style={{ marginBottom: 10 }}>
                  {shares.length === 0 && <span className="hint">Not shared yet.</span>}
                  {shares.map((recipient) => (
                    <span className="chip active" key={recipient.id}>
                      {recipient.name}
                    </span>
                  ))}
                </div>
                <div className="flex">
                  <input
                    className="input grow"
                    placeholder="Filter users…"
                    value={recipientFilter}
                    onChange={(event) => setRecipientFilter(event.target.value)}
                  />
                  <button type="button" className="btn small" onClick={shareEveryone} disabled={busy}>
                    Share with all
                  </button>
                </div>
                <div className="flex wrap" style={{ marginTop: 10 }}>
                  {filteredRecipients.map((recipient) => (
                    <button
                      type="button"
                      key={recipient.id}
                      className={`chip${selectedRecipients.includes(recipient.id) ? ' active' : ''}`}
                      onClick={() => void toggleRecipient(recipient.id)}
                    >
                      {recipient.name}
                    </button>
                  ))}
                </div>
                <button type="button" className="btn small" style={{ marginTop: 12 }} onClick={saveShares} disabled={busy}>
                  Save sharing
                </button>
              </div>
            )}

            {error && <p className="error-text">{error}</p>}
          </div>

          <div className="modal-foot">
            {isEdit && (
              <button type="button" className="btn danger" onClick={remove} disabled={busy}>
                <Icon name="trash" size={14} /> Delete
              </button>
            )}
            <button type="button" className="btn ghost" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" className="btn primary" disabled={busy}>
              {isEdit ? 'Save changes' : 'Add account'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
