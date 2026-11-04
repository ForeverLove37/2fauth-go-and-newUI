import { useState, type ReactNode } from 'react'
import Icon from './Icon'

const STORAGE_KEY = '2fauth.sections.v1'

function remembered(): Record<string, boolean> {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}') as Record<string, boolean>
  } catch {
    return {}
  }
}

function remember(id: string, open: boolean) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ ...remembered(), [id]: open }))
  } catch {
    /* the section simply falls back to its default next time */
  }
}

interface SectionProps {
  /** names the remembered state */
  id: string
  title: string
  hint?: string
  /** current state, readable without opening the section */
  summary?: string
  /** controls kept in the head, shown while the section is open */
  actions?: ReactNode
  defaultOpen?: boolean
  children: ReactNode
}

/** A panel that folds down to its head. Its content mounts on the first opening and stays. */
export default function Section({ id, title, hint, summary, actions, defaultOpen = false, children }: SectionProps) {
  const [open, setOpen] = useState(() => remembered()[id] ?? defaultOpen)
  const [mounted, setMounted] = useState(open)

  function toggle() {
    remember(id, !open)
    setOpen(!open)
    setMounted(true)
  }

  return (
    <section className={`panel section${open ? ' open' : ''}`}>
      <div className="section-head">
        <button type="button" className="section-toggle" aria-expanded={open} onClick={toggle}>
          <span className="grow">
            <h2>{title}</h2>
            {open && hint && <p>{hint}</p>}
          </span>
          {!open && summary && <span className="section-summary">{summary}</span>}
          <span className="section-chevron">
            <Icon name="chevron" size={15} />
          </span>
        </button>
        {open && actions && <div className="section-actions">{actions}</div>}
      </div>
      {mounted && (
        <div className="panel-body" hidden={!open}>
          {children}
        </div>
      )}
    </section>
  )
}
