import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { BASE } from '../api/client'
import Icon from '../components/Icon'
import { useT } from '../i18n'
import { copyText } from '../lib/format'
import { inline, parseMarkdown, slug, type Block } from '../lib/markdown'
import { useStore } from '../store/useStore'

/** Where the server publishes the guide, as an absolute address an agent can fetch. */
function guideUrl(): string {
  return `${window.location.origin}${BASE}/agents.md`
}

function CodeBlock({ block }: { block: Extract<Block, { kind: 'code' }> }) {
  const t = useT()
  const toast = useStore((state) => state.toast)
  return (
    <div className="doc-code">
      <div className="doc-code-head">
        <span>{block.lang || 'text'}</span>
        <button
          className="btn ghost small"
          onClick={async () => {
            const ok = await copyText(block.text)
            toast(ok ? 'success' : 'error', t(ok ? 'Copied' : 'Copy failed'))
          }}
        >
          <Icon name="copy" size={13} /> {t('Copy')}
        </button>
      </div>
      <pre>
        <code>{block.text}</code>
      </pre>
    </div>
  )
}

function render(block: Block, index: number) {
  switch (block.kind) {
    case 'heading': {
      const Tag = (`h${Math.min(block.level + 1, 4)}`) as 'h2' | 'h3' | 'h4'
      return block.level === 1 ? null : (
        <Tag key={index} id={slug(block.text)}>
          {inline(block.text)}
        </Tag>
      )
    }
    case 'paragraph':
      return <p key={index}>{inline(block.text)}</p>
    case 'list':
      return (
        <ul key={index}>
          {block.items.map((item, itemIndex) => (
            <li key={itemIndex}>{inline(item)}</li>
          ))}
        </ul>
      )
    case 'table':
      return (
        <div className="doc-table" key={index}>
          <table className="table">
            <thead>
              <tr>{block.head.map((cell, cellIndex) => <th key={cellIndex}>{inline(cell)}</th>)}</tr>
            </thead>
            <tbody>
              {block.rows.map((row, rowIndex) => (
                <tr key={rowIndex}>{row.map((cell, cellIndex) => <td key={cellIndex}>{inline(cell)}</td>)}</tr>
              ))}
            </tbody>
          </table>
        </div>
      )
    case 'code':
      return <CodeBlock key={index} block={block} />
  }
}

/**
 * The guide for AI agents, as people read it. The server writes it in Markdown at
 * /agents.md with the real addresses of this instance; this page only presents it.
 */
export default function Agents() {
  const t = useT()
  const toast = useStore((state) => state.toast)
  const user = useStore((state) => state.user)
  const [source, setSource] = useState<string | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let alive = true
    fetch(`${BASE}/agents.md`, { headers: { Accept: 'text/markdown' } })
      .then(async (response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`)
        const text = await response.text()
        if (alive) setSource(text)
      })
      .catch((reason: Error) => alive && setError(reason.message))
    return () => {
      alive = false
    }
  }, [])

  const blocks = useMemo(() => (source ? parseMarkdown(source) : []), [source])
  const title = blocks.find((block) => block.kind === 'heading' && block.level === 1)
  const sections = blocks.filter((block): block is Extract<Block, { kind: 'heading' }> => block.kind === 'heading' && block.level === 2)
  const prompt = t('Read {url} and follow it to get one-time passwords from my 2FAuth server.', { url: guideUrl() })

  async function copy(text: string, message: string) {
    const ok = await copyText(text)
    toast(ok ? 'success' : 'error', ok ? message : t('Copy failed'))
  }

  return (
    <div className="doc-page">
      <header className="doc-top">
        <Link to={user ? '/settings?tab=security' : '/login'} className="brand">
          <div className="brand-mark">2F</div>
          <div>
            {t('Agent guide')}
            <small>{t('One-time passwords for AI agents and scripts')}</small>
          </div>
        </Link>
        <div className="spacer" />
        <a className="btn small ghost" href={`${BASE}/agents.md`} target="_blank" rel="noreferrer">
          <Icon name="download" size={14} /> agents.md
        </a>
      </header>

      <main className="doc">
        <section className="panel doc-intro">
          <p>
            {t(
              'Give this address to an agent: it explains, with the real addresses of this server, how to get a code over SSH or with a token. The agent still needs a key or a token that you grant it.',
            )}
          </p>
          <div className="doc-prompt mono">{prompt}</div>
          <div className="flex wrap">
            <button className="btn primary small" onClick={() => void copy(prompt, t('Instruction copied'))}>
              <Icon name="copy" size={14} /> {t('Copy an instruction for the agent')}
            </button>
            <button className="btn small" onClick={() => void copy(guideUrl(), t('Address copied'))}>
              <Icon name="copy" size={14} /> {t('Copy the address')}
            </button>
            {user && (
              <Link className="btn small ghost" to="/settings?tab=security">
                <Icon name="shield" size={14} /> {t('Manage SSH keys and tokens')}
              </Link>
            )}
          </div>
        </section>

        {!source && <p className="hint">{error || t('Loading…')}</p>}
        {source && (
          <article className="panel doc-body">
            {title && title.kind === 'heading' && <h1>{inline(title.text)}</h1>}
            {sections.length > 2 && (
              <nav className="doc-toc">
                {sections.map((section) => (
                  <a key={section.text} href={`#${slug(section.text)}`}>
                    {section.text}
                  </a>
                ))}
              </nav>
            )}
            {blocks.map(render)}
          </article>
        )}
      </main>
    </div>
  )
}
