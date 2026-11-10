import { Fragment, type ReactNode } from 'react'

/**
 * Renders the small Markdown subset the server's documents use: headings, paragraphs,
 * lists, tables and fenced code. Text is never parsed as HTML, so a document cannot
 * inject markup into the page.
 */

export type Block =
  | { kind: 'heading'; level: number; text: string }
  | { kind: 'paragraph'; text: string }
  | { kind: 'list'; items: string[] }
  | { kind: 'table'; head: string[]; rows: string[][] }
  | { kind: 'code'; lang: string; text: string }

function cells(line: string): string[] {
  return line.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map((cell) => cell.trim())
}

export function parseMarkdown(source: string): Block[] {
  const lines = source.replace(/\r\n/g, '\n').split('\n')
  const blocks: Block[] = []
  let index = 0
  while (index < lines.length) {
    const line = lines[index]
    const fence = line.match(/^```(\w*)\s*$/)
    if (fence) {
      const body: string[] = []
      index += 1
      while (index < lines.length && !/^```\s*$/.test(lines[index])) body.push(lines[index++])
      index += 1
      blocks.push({ kind: 'code', lang: fence[1], text: body.join('\n') })
      continue
    }
    const heading = line.match(/^(#{1,4})\s+(.*)$/)
    if (heading) {
      blocks.push({ kind: 'heading', level: heading[1].length, text: heading[2] })
      index += 1
      continue
    }
    if (/^\s*\|/.test(line) && /^\s*\|[\s:|-]+\|\s*$/.test(lines[index + 1] ?? '')) {
      const head = cells(line)
      const rows: string[][] = []
      index += 2
      while (index < lines.length && /^\s*\|/.test(lines[index])) rows.push(cells(lines[index++]))
      blocks.push({ kind: 'table', head, rows })
      continue
    }
    if (/^- /.test(line)) {
      const items: string[] = []
      while (index < lines.length && (/^- /.test(lines[index]) || (/^\s{2,}\S/.test(lines[index]) && items.length > 0))) {
        if (/^- /.test(lines[index])) items.push(lines[index].slice(2))
        else items[items.length - 1] += ' ' + lines[index].trim()
        index += 1
      }
      blocks.push({ kind: 'list', items })
      continue
    }
    if (line.trim() === '') {
      index += 1
      continue
    }
    const text: string[] = []
    while (index < lines.length && lines[index].trim() !== '' && !/^(```|#{1,4}\s|- |\s*\|)/.test(lines[index])) {
      text.push(lines[index++].trim())
    }
    blocks.push({ kind: 'paragraph', text: text.join(' ') })
  }
  return blocks
}

/** `code`, **bold** and bare links. */
export function inline(text: string): ReactNode {
  const parts = text.split(/(`[^`]+`|\*\*[^*]+\*\*|https?:\/\/[^\s)|,]+[^\s)|,.:;])/g)
  return parts.map((part, index) => {
    if (part.startsWith('`') && part.endsWith('`') && part.length > 1) return <code key={index}>{part.slice(1, -1)}</code>
    if (part.startsWith('**') && part.endsWith('**') && part.length > 3) return <strong key={index}>{part.slice(2, -2)}</strong>
    if (/^https?:\/\//.test(part)) {
      return (
        <a key={index} href={part} target="_blank" rel="noreferrer noopener">
          {part}
        </a>
      )
    }
    return <Fragment key={index}>{part}</Fragment>
  })
}

/** A stable anchor for a heading: "2. Over SSH" → "over-ssh". */
export function slug(text: string): string {
  return text
    .toLowerCase()
    .replace(/^[\d.\s]+/, '')
    .replace(/[^\p{L}\p{N}]+/gu, '-')
    .replace(/^-|-$/g, '')
}
