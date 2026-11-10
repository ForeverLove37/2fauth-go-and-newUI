import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import type { ApiError, SshKey, SshKeysResponse } from '../api/types'
import { useT } from '../i18n'
import { accountLabel } from '../lib/format'
import { useLoaded } from '../lib/hooks'
import { useStore } from '../store/useStore'
import Icon from './Icon'
import Menu from './Menu'
import Section from './Section'

interface Draft {
  id: number | null
  name: string
  publicKey: string
  all: boolean
  accounts: number[]
  groups: number[]
  password: string
}

const emptyDraft: Draft = { id: null, name: '', publicKey: '', all: false, accounts: [], groups: [], password: '' }

/** The public keys allowed to read codes from the command line, and what each can read. */
export default function SshKeysPanel() {
  const t = useT()
  const toast = useStore((state) => state.toast)
  const accounts = useStore((state) => state.accounts)
  const groups = useStore((state) => state.groups)
  const { data: info, error: loadError, reload } = useLoaded<SshKeysResponse>('ssh-keys', () => api.sshKeys())
  const [draft, setDraft] = useState<Draft | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const usage = useMemo(() => {
    const port = info?.port && info.port !== '22' ? `-p ${info.port} ` : ''
    const target = `otp@${info?.host || window.location.hostname}`
    return [`ssh ${port}${target} list`, `ssh ${port}${target} get github`, `ssh ${port}${target} get github --json`].join('\n')
  }, [info?.port, info?.host])

  function toggle(list: number[], id: number) {
    return list.includes(id) ? list.filter((item) => item !== id) : [...list, id]
  }

  function edit(key: SshKey) {
    setError('')
    setDraft({
      id: key.id,
      name: key.name,
      publicKey: '',
      all: key.all_accounts,
      accounts: key.account_ids,
      groups: key.group_ids,
      password: '',
    })
  }

  async function save(event: React.FormEvent) {
    event.preventDefault()
    if (!draft) return
    setBusy(true)
    setError('')
    const scope = {
      name: draft.name.trim(),
      all_accounts: draft.all,
      account_ids: draft.accounts.join(','),
      group_ids: draft.groups.join(','),
      password: draft.password,
    }
    try {
      if (draft.id === null) await api.createSshKey({ ...scope, public_key: draft.publicKey.trim() })
      else await api.updateSshKey(draft.id, scope)
      toast('success', draft.id === null ? t('Key added') : t('Key updated'))
      setDraft(null)
      await reload()
    } catch (caught) {
      const typed = caught as ApiError
      setError(Object.values(typed.errors ?? {}).flat()[0] ?? typed.message)
    } finally {
      setBusy(false)
    }
  }

  async function remove(key: SshKey) {
    if (!window.confirm(t('Remove the key {name}? That machine loses access at once.', { name: key.name }))) return
    try {
      await api.deleteSshKey(key.id)
      toast('success', t('Key removed'))
      await reload()
    } catch (caught) {
      toast('error', (caught as ApiError).message)
    }
  }

  return (
    <Section
      id="security-ssh"
      title={t('SSH keys')}
      hint={t('Machines holding one of these keys can read codes from the command line, like git over SSH.')}
      defaultOpen
      summary={info ? String(info.keys.length) : undefined}
    >
      {!info && <p className="hint">{loadError || t('Loading…')}</p>}
      {info && !info.enabled && (
        <p className="hint" style={{ marginBottom: 12 }}>
          {t('The SSH service is off. Set SSH_LISTEN_ADDR in the .env file and restart to turn it on.')}
        </p>
      )}

      {info && info.keys.length > 0 ? (
        <table className="table">
          <thead>
            <tr>
              <th>{t('Key name')}</th>
              <th>{t('Read')}</th>
              <th>{t('Last used')}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {info.keys.map((key) => (
              <tr key={key.id}>
                <td>
                  <strong>{key.name}</strong>
                  <div className="hint mono small">{key.fingerprint}</div>
                </td>
                <td className="hint small">
                  {key.all_accounts
                    ? t('all accounts')
                    : t('{accounts} accounts, {groups} groups', { accounts: key.account_ids.length, groups: key.group_ids.length })}
                </td>
                <td className="hint small">
                  {key.last_used_at ? `${new Date(key.last_used_at * 1000).toLocaleString()} · ${key.last_used_ip}` : t('Never')}
                </td>
                <td>
                  <div className="flex" style={{ justifyContent: 'flex-end' }}>
                    <Menu
                      items={[
                        { label: t('Edit'), icon: 'sliders', onClick: () => edit(key) },
                        { label: t('Delete'), icon: 'trash', danger: true, onClick: () => void remove(key) },
                      ]}
                    />
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        info && <p className="hint">{t('No SSH key yet.')}</p>
      )}

      {!draft && (
        <button
          className="btn small primary"
          style={{ marginTop: 12 }}
          onClick={() => {
            setError('')
            setDraft(emptyDraft)
          }}
        >
          <Icon name="plus" size={14} /> {t('Add a key')}
        </button>
      )}

      {draft && (
        <form onSubmit={save} style={{ display: 'grid', gap: 12, marginTop: 14 }}>
          <div className="field">
            <label htmlFor="ssh-key-name">{t('Key name')}</label>
            <input
              id="ssh-key-name"
              className="input"
              maxLength={100}
              value={draft.name}
              onChange={(event) => setDraft({ ...draft, name: event.target.value })}
              placeholder="laptop"
            />
          </div>
          {draft.id === null && (
            <div className="field">
              <label htmlFor="ssh-public-key">{t('Public key')}</label>
              <textarea
                id="ssh-public-key"
                className="input mono"
                rows={3}
                spellCheck={false}
                value={draft.publicKey}
                onChange={(event) => setDraft({ ...draft, publicKey: event.target.value })}
                placeholder="ssh-ed25519 AAAA... me@laptop"
              />
              <p className="hint">{t('Paste the content of ~/.ssh/id_ed25519.pub. Never paste a private key.')}</p>
            </div>
          )}
          <div className="field">
            <label>{t('What this key can read')}</label>
            <div className="ssh-scope" role="group">
              <label>
                <input type="radio" name="ssh-scope" checked={!draft.all} onChange={() => setDraft({ ...draft, all: false })} />
                {t('Selected accounts and groups')}
              </label>
              <label>
                <input type="radio" name="ssh-scope" checked={draft.all} onChange={() => setDraft({ ...draft, all: true })} />
                {t('All my accounts, including future ones')}
              </label>
            </div>
            {!draft.all && (
              <div className="ssh-scope" style={{ marginTop: 8 }}>
                {groups.map((group) => (
                  <label key={`group-${group.id}`}>
                    <input
                      type="checkbox"
                      checked={draft.groups.includes(group.id)}
                      onChange={() => setDraft({ ...draft, groups: toggle(draft.groups, group.id) })}
                    />
                    {t('Group: {name} (includes accounts added later)', { name: group.name })}
                  </label>
                ))}
                {accounts.map((account) => (
                  <label key={account.id}>
                    <input
                      type="checkbox"
                      checked={draft.accounts.includes(account.id)}
                      onChange={() => setDraft({ ...draft, accounts: toggle(draft.accounts, account.id) })}
                    />
                    {accountLabel(account)}
                    {account.service && account.account ? <span className="hint">{account.account}</span> : null}
                  </label>
                ))}
              </div>
            )}
          </div>
          <div className="field" style={{ maxWidth: 320 }}>
            <label htmlFor="ssh-password">{t('Your password')}</label>
            <input
              id="ssh-password"
              className="input"
              type="password"
              autoComplete="current-password"
              value={draft.password}
              onChange={(event) => setDraft({ ...draft, password: event.target.value })}
            />
          </div>
          {error && <p className="hint" style={{ color: 'var(--danger)' }}>{error}</p>}
          <div className="flex">
            <button className="btn small primary" disabled={busy}>
              <Icon name="check" size={14} /> {draft.id === null ? t('Save key') : t('Save changes')}
            </button>
            <button type="button" className="btn small ghost" onClick={() => setDraft(null)}>
              {t('Cancel')}
            </button>
          </div>
        </form>
      )}

      {info?.enabled && (
        <div style={{ marginTop: 16 }}>
          <strong className="small">{t('Usage')}</strong>
          <div className="ssh-usage" style={{ marginTop: 6 }}>
            {usage}
          </div>
          <p className="hint small" style={{ marginTop: 8 }}>
            <Link to="/agents">{t('Guide for AI agents and scripts')} →</Link>
          </p>
          {info.host_fingerprint && (
            <p className="hint small" style={{ marginTop: 8 }}>
              {t('Host key fingerprint, to compare on the first connection:')} <span className="mono">{info.host_fingerprint}</span>
            </p>
          )}
        </div>
      )}
    </Section>
  )
}
