import { Fragment, type ReactNode } from 'react'
import { Link } from 'react-router-dom'

/**
 * A small Markdown renderer for Melon's own content pages. It builds React
 * elements (never HTML strings) and knows just what those pages use:
 * headings (#, ##, ###), paragraphs, lists (- and 1.), > quotes, ---,
 * **bold**, *italic*, `code` and [links](/path or https://...).
 */

function inline(text: string, key = ''): ReactNode[] {
  const out: ReactNode[] = []
  const re = /\*\*(.+?)\*\*|\*(.+?)\*|`(.+?)`|\[(.+?)\]\((\S+?)\)/g
  let at = 0
  let i = 0
  for (const m of text.matchAll(re)) {
    if (m.index > at) out.push(text.slice(at, m.index))
    const k = `${key}-${i++}`
    if (m[1] !== undefined) out.push(<strong key={k}>{inline(m[1], k)}</strong>)
    else if (m[2] !== undefined) out.push(<em key={k}>{inline(m[2], k)}</em>)
    else if (m[3] !== undefined) out.push(<code key={k}>{m[3]}</code>)
    else {
      const href = m[5]
      out.push(
        href.startsWith('/') ? (
          <Link key={k} to={href}>
            {inline(m[4], k)}
          </Link>
        ) : (
          <a key={k} href={href} target="_blank" rel="noreferrer">
            {inline(m[4], k)}
          </a>
        ),
      )
    }
    at = m.index + m[0].length
  }
  if (at < text.length) out.push(text.slice(at))
  return out
}

/** Turns a heading into an id for in-page links: "Wat ziet een admin?" → "wat-ziet-een-admin". */
const slug = (s: string) =>
  s
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')

export function Markdown({ source }: { source: string }) {
  const blocks: ReactNode[] = []
  const lines = source.replace(/\r\n/g, '\n').split('\n')
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    const k = `b${blocks.length}`
    if (line.trim() === '') {
      i++
    } else if (/^#{1,3}\s/.test(line)) {
      const level = line.match(/^#+/)![0].length
      const text = line.slice(level).trim()
      const Tag = (['h1', 'h2', 'h3'] as const)[level - 1]
      blocks.push(
        <Tag key={k} id={slug(text)}>
          {inline(text, k)}
        </Tag>,
      )
      i++
    } else if (/^---+\s*$/.test(line)) {
      blocks.push(<hr key={k} />)
      i++
    } else if (/^(\s*[-*]|\s*\d+\.)\s/.test(line)) {
      const ordered = /^\s*\d+\./.test(line)
      const items: string[] = []
      while (i < lines.length && /^(\s*[-*]|\s*\d+\.)\s/.test(lines[i])) {
        let item = lines[i].replace(/^\s*([-*]|\d+\.)\s+/, '')
        i++
        // Indented lines continue the item.
        while (i < lines.length && /^\s{2,}\S/.test(lines[i]) && !/^(\s*[-*]|\s*\d+\.)\s/.test(lines[i])) item += ' ' + lines[i++].trim()
        items.push(item)
      }
      const List = ordered ? 'ol' : 'ul'
      blocks.push(
        <List key={k}>
          {items.map((it, j) => (
            <li key={j}>{inline(it, `${k}-${j}`)}</li>
          ))}
        </List>,
      )
    } else if (line.startsWith('>')) {
      const quote: string[] = []
      while (i < lines.length && lines[i].startsWith('>')) quote.push(lines[i++].replace(/^>\s?/, ''))
      blocks.push(<blockquote key={k}>{inline(quote.join(' '), k)}</blockquote>)
    } else {
      const para: string[] = []
      while (i < lines.length && lines[i].trim() !== '' && !/^(#{1,3}\s|---+\s*$|\s*[-*]\s|\s*\d+\.\s|>)/.test(lines[i])) para.push(lines[i++].trim())
      blocks.push(<p key={k}>{inline(para.join(' '), k)}</p>)
    }
  }
  return <Fragment>{blocks}</Fragment>
}
