import { Fragment, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Checkbox } from '@/components/ui/checkbox'
import { cn } from '@/lib/utils'

/**
 * A small Markdown renderer for Melon's content pages and for homework people
 * write themselves. It builds React elements (never HTML strings) and knows
 * just what those use: headings (#, ##, ###), paragraphs, lists (- and 1.),
 * checklists (- [ ] and - [x]), > quotes, ---, **bold**, *italic*,
 * ~~strikethrough~~, `code` and [links](/path or https://...).
 */

/** Only links that go somewhere harmless: no javascript: or data: URLs. */
function safeHref(href: string): string | null {
  if (href.startsWith('/') && !href.startsWith('//')) return href
  return /^(https?:|mailto:)/i.test(href) ? href : null
}

function inline(text: string, key = ''): ReactNode[] {
  const out: ReactNode[] = []
  const re = /\*\*(.+?)\*\*|\*(.+?)\*|~~(.+?)~~|`(.+?)`|\[(.+?)\]\((\S+?)\)/g
  let at = 0
  let i = 0
  for (const m of text.matchAll(re)) {
    if (m.index > at) out.push(text.slice(at, m.index))
    const k = `${key}-${i++}`
    if (m[1] !== undefined) out.push(<strong key={k}>{inline(m[1], k)}</strong>)
    else if (m[2] !== undefined) out.push(<em key={k}>{inline(m[2], k)}</em>)
    else if (m[3] !== undefined) out.push(<s key={k}>{inline(m[3], k)}</s>)
    else if (m[4] !== undefined) out.push(<code key={k}>{m[4]}</code>)
    else {
      const href = safeHref(m[6])
      out.push(
        href === null ? (
          <Fragment key={k}>{inline(m[5], k)}</Fragment>
        ) : href.startsWith('/') ? (
          <Link key={k} to={href}>
            {inline(m[5], k)}
          </Link>
        ) : (
          <a key={k} href={href} target="_blank" rel="noreferrer">
            {inline(m[5], k)}
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

const task = /^\[([ xX])\]\s+/
const listItem = /^(\s*[-*]|\s*\d+\.)\s/

/**
 * Ticks the n-th checklist item in a Markdown source on or off, for
 * checklists people tick right in the rendered text.
 */
export function toggleTask(source: string, n: number): string {
  let seen = -1
  return source.replace(/^(\s*(?:[-*]|\d+\.)\s+)\[([ xX])\]/gm, (all, lead: string, mark: string) => (++seen === n ? `${lead}[${mark === ' ' ? 'x' : ' '}]` : all))
}

/**
 * Whether every checklist item in a Markdown source is ticked, or null when
 * it has none. Homework with its own checklist is done when that is.
 */
export function tasksDone(source: string): boolean | null {
  const marks = [...source.matchAll(/^\s*(?:[-*]|\d+\.)\s+\[([ xX])\]\s/gm)].map((m) => m[1] !== ' ')
  return marks.length ? marks.every(Boolean) : null
}

/**
 * Renders Markdown. With onToggleTask, checklist items can be ticked; it gets
 * the item's index for toggleTask. headingIds is off for user text, so its
 * headings cannot clash with ids on the page.
 */
export function Markdown({ source, onToggleTask, headingIds = true }: { source: string; onToggleTask?: (n: number) => void; headingIds?: boolean }) {
  const blocks: ReactNode[] = []
  const lines = source.replace(/\r\n/g, '\n').split('\n')
  let tasks = 0
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
        <Tag key={k} id={headingIds ? slug(text) : undefined}>
          {inline(text, k)}
        </Tag>,
      )
      i++
    } else if (/^---+\s*$/.test(line)) {
      blocks.push(<hr key={k} />)
      i++
    } else if (listItem.test(line)) {
      const ordered = /^\s*\d+\./.test(line)
      const items: { text: string; nested: boolean }[] = []
      const base = lines[i].match(/^\s*/)![0].length
      while (i < lines.length && listItem.test(lines[i])) {
        // An item indented past the first one sits under the item above it.
        const nested = items.length > 0 && lines[i].match(/^\s*/)![0].length >= base + 2
        let text = lines[i].replace(/^\s*([-*]|\d+\.)\s+/, '')
        i++
        // Indented lines continue the item.
        while (i < lines.length && /^\s{2,}\S/.test(lines[i]) && !listItem.test(lines[i])) text += ' ' + lines[i++].trim()
        items.push({ text, nested })
      }
      const List = ordered ? 'ol' : 'ul'
      const isTasks = items.some((it) => task.test(it.text))
      blocks.push(
        <List key={k} data-tasks={isTasks || undefined}>
          {items.map(({ text: it, nested }, j) => {
            const m = it.match(task)
            if (!m) {
              return (
                <li key={j} data-nested={nested || undefined}>
                  {inline(it, `${k}-${j}`)}
                </li>
              )
            }
            const n = tasks++
            const checked = m[1] !== ' '
            return (
              <li key={j} data-task={checked ? 'done' : 'open'} data-nested={nested || undefined}>
                {/* Top-level checklist items get the big checkbox; the ones under them a smaller one. */}
                <label className={cn('inline-flex items-start', nested ? 'gap-2' : 'gap-3')}>
                  <Checkbox className={cn('border-muted-foreground/60', nested ? 'mt-1' : 'mt-0.5 size-5')} checked={checked} disabled={!onToggleTask} onCheckedChange={() => onToggleTask?.(n)} />
                  <span data-task-text>{inline(it.slice(m[0].length), `${k}-${j}`)}</span>
                </label>
              </li>
            )
          })}
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

/** Typography for Markdown inside the app (homework), sized for a drawer. */
export const markdownClass = [
  'grid gap-2 text-sm leading-relaxed break-words',
  '[&_h1]:text-lg [&_h1]:font-semibold [&_h2]:text-base [&_h2]:font-semibold [&_h3]:font-semibold',
  '[&_ul]:grid [&_ul]:list-disc [&_ul]:gap-1 [&_ul]:pl-5 [&_ol]:grid [&_ol]:list-decimal [&_ol]:gap-1 [&_ol]:pl-5',
  '[&_ul[data-tasks]]:list-none [&_ul[data-tasks]]:pl-0 [&_li[data-task=done]_[data-task-text]]:text-muted-foreground [&_li[data-task=done]_[data-task-text]]:line-through',
  // Items under another item move in; in a checklist, under the big checkbox's text.
  '[&_li[data-nested]]:ml-5 [&_ul[data-tasks]_li[data-nested]]:ml-8',
  '[&_ul[data-tasks]_li:not([data-task])]:ml-5 [&_ul[data-tasks]_li:not([data-task])]:list-disc [&_ul[data-tasks]_li[data-nested]:not([data-task])]:ml-12',
  '[&_li]:marker:text-muted-foreground [&_strong]:font-semibold [&_s]:text-muted-foreground',
  '[&_a]:font-medium [&_a]:text-honeydew-ink [&_a]:underline [&_a]:underline-offset-2',
  '[&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:text-[0.8125rem]',
  '[&_blockquote]:rounded-lg [&_blockquote]:border-l-4 [&_blockquote]:border-honeydew [&_blockquote]:bg-honeydew-tint [&_blockquote]:px-3 [&_blockquote]:py-2 [&_blockquote]:text-honeydew-ink',
  '[&_hr]:my-1',
].join(' ')
