import { useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'
import { Bold, Code, Eye, Heading, Italic, Link2, List, ListChecks, ListOrdered, Strikethrough, type LucideIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useI18n, type Key } from '@/lib/i18n'
import { Markdown, markdownClass } from '@/lib/markdown'
import { cn } from '@/lib/utils'

type Edit = { value: string; start: number; end: number }

/** Wraps the selection in before/after, or takes the wrapping off again. */
function wrap(e: Edit, before: string, after: string, placeholder: string): Edit {
  const { value, start, end } = e
  const picked = value.slice(start, end)
  if (value.slice(start - before.length, start) === before && value.slice(end, end + after.length) === after) {
    return { value: value.slice(0, start - before.length) + picked + value.slice(end + after.length), start: start - before.length, end: end - before.length }
  }
  const text = picked || placeholder
  return { value: value.slice(0, start) + before + text + after + value.slice(end), start: start + before.length, end: start + before.length + text.length }
}

/**
 * Puts a prefix in front of every selected line, or takes it off when every
 * line already has it. prefix gets the line's index, for numbered lists.
 */
function prefixLines(e: Edit, prefix: (i: number) => string, has: RegExp): Edit {
  const { value } = e
  const from = value.lastIndexOf('\n', e.start - 1) + 1
  const nl = value.indexOf('\n', e.end)
  const to = nl === -1 ? value.length : nl
  const lines = value.slice(from, to).split('\n')
  const remove = lines.every((l) => has.test(l))
  const next = lines.map((l, i) => (remove ? l.replace(has, '') : prefix(i) + l.replace(/^(\s*([-*]|\d+\.)\s+(\[[ xX]\]\s+)?|#{1,3}\s+)/, ''))).join('\n')
  return { value: value.slice(0, from) + next + value.slice(to), start: from, end: from + next.length }
}

type Tool = { icon: LucideIcon; label: Key; key?: string; hidden?: boolean; run: (e: Edit, t: (k: Key) => string) => Edit }

const tools: Tool[][] = [
  [
    { icon: Bold, label: 'mdBold', key: 'b', run: (e) => wrap(e, '**', '**', '') },
    { icon: Italic, label: 'mdItalic', key: 'i', run: (e) => wrap(e, '*', '*', '') },
    { icon: Strikethrough, label: 'mdStrike', key: 'd', hidden: true, run: (e) => wrap(e, '~~', '~~', '') },
    { icon: Heading, label: 'mdHeading', run: (e) => prefixLines(e, () => '### ', /^###\s+/) },
  ],
  [
    { icon: List, label: 'mdList', run: (e) => prefixLines(e, () => '- ', /^- (?!\[[ xX]\])/) },
    { icon: ListOrdered, label: 'mdNumbered', run: (e) => prefixLines(e, (i) => `${i + 1}. `, /^\d+\.\s+/) },
    { icon: ListChecks, label: 'mdTask', run: (e) => prefixLines(e, () => '- [ ] ', /^- \[[ xX]\]\s+/) },
  ],
  [
    { icon: Link2, label: 'mdLink', key: 'k', run: (e, t) => (e.start === e.end ? wrap(e, '[', '](https://)', t('mdLinkText')) : wrap(e, '[', '](https://)', '')) },
    { icon: Code, label: 'mdCode', run: (e) => wrap(e, '`', '`', '') },
  ],
]

/**
 * A Markdown text field with formatting buttons and a preview, in Melon's
 * own look. The buttons write plain Markdown, so the text stays readable
 * wherever it ends up (the API, the calendar feed).
 */
export function MarkdownEditor({
  value,
  onChange,
  placeholder,
  maxLength,
  autoFocus,
  label,
  onSubmit,
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  maxLength?: number
  autoFocus?: boolean
  label: string
  /** Ctrl/Cmd+Enter. */
  onSubmit?: () => void
}) {
  const { t } = useI18n()
  const ref = useRef<HTMLTextAreaElement>(null)
  const [preview, setPreview] = useState(false)
  // Where the cursor goes once React has written a value the buttons made.
  const selection = useRef<{ start: number; end: number } | null>(null)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el || !selection.current) return
    el.focus()
    el.setSelectionRange(selection.current.start, selection.current.end)
    selection.current = null
  })
  const write = (v: string, start: number, end = start) => {
    selection.current = { start, end }
    onChange(v)
  }

  const apply = (tool: Tool) => {
    const el = ref.current
    if (!el) return
    const next = tool.run({ value, start: el.selectionStart, end: el.selectionEnd }, t)
    write(next.value, next.start, next.end)
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (!(e.metaKey || e.ctrlKey)) {
      // Enter on a list line starts the next item; on an empty item it ends the list.
      if (e.key === 'Enter' && !e.shiftKey) {
        const el = e.currentTarget
        if (el.selectionStart !== el.selectionEnd) return
        const from = value.lastIndexOf('\n', el.selectionStart - 1) + 1
        const line = value.slice(from, el.selectionStart)
        const m = line.match(/^(\s*)([-*]|(\d+)\.)\s+(\[[ xX]\]\s+)?/)
        if (!m) return
        e.preventDefault()
        if (line.trim() === m[0].trim()) {
          write(value.slice(0, from) + value.slice(el.selectionStart), from)
          return
        }
        const bullet = m[3] ? `${Number(m[3]) + 1}.` : m[2]
        const insert = `\n${m[1]}${bullet} ${m[4] ? '[ ] ' : ''}`
        const at = el.selectionStart
        write(value.slice(0, at) + insert + value.slice(at), at + insert.length)
      }
      return
    }
    if (e.key === 'Enter' && onSubmit) {
      e.preventDefault()
      onSubmit()
      return
    }
    const tool = tools.flat().find((x) => x.key === e.key.toLowerCase() && (x.key !== 'd' || e.shiftKey))
    if (tool) {
      e.preventDefault()
      apply(tool)
    }
  }

  return (
    <div className="overflow-hidden rounded-xl border border-input bg-card focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50 dark:bg-input/30">
      <div role="toolbar" aria-label={label} className="flex items-center gap-0.5 border-b bg-muted/40 px-1 py-1">
        {tools.map((group, g) => (
          <div key={g} className={cn('flex items-center gap-0.5', g > 0 && 'ml-0.5 border-l pl-1')}>
            {group
              .filter((tool) => !tool.hidden)
              .map((tool) => (
                <Button
                  key={tool.label}
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className="size-8 text-muted-foreground hover:text-foreground"
                  aria-label={t(tool.label)}
                  title={tool.key ? `${t(tool.label)} (Ctrl+${tool.key.toUpperCase()})` : t(tool.label)}
                  disabled={preview}
                  // Keep the selection in the text field.
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => apply(tool)}
                >
                  <tool.icon />
                </Button>
              ))}
          </div>
        ))}
        <span className="flex-1" />
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className={cn('size-8 text-muted-foreground hover:text-foreground', preview && 'bg-primary text-primary-foreground hover:bg-primary/80 hover:text-primary-foreground')}
          aria-label={t('mdPreview')}
          title={t('mdPreview')}
          aria-pressed={preview}
          onClick={() => setPreview((p) => !p)}
        >
          <Eye />
        </Button>
      </div>
      {preview ? (
        <div className={cn(markdownClass, 'min-h-32 px-3 py-2.5')}>{value.trim() ? <Markdown source={value} headingIds={false} /> : <p className="text-muted-foreground">{t('mdEmpty')}</p>}</div>
      ) : (
        <textarea
          ref={ref}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={onKeyDown}
          maxLength={maxLength}
          autoFocus={autoFocus}
          placeholder={placeholder}
          aria-label={label}
          className="block min-h-32 w-full resize-y bg-transparent px-3 py-2.5 text-base outline-none placeholder:text-muted-foreground/60 md:text-sm"
        />
      )}
    </div>
  )
}
