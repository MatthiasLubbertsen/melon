import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useQueries, useQuery } from '@tanstack/react-query'
import { ArrowRight, CalendarDays, CalendarRange, CalendarX, Clock, CornerDownLeft, ExternalLink, Hourglass, Loader2, NotebookPen, Search, SearchX, Sparkles, X } from 'lucide-react'
import { api, type Homework, type Lesson } from '@/lib/api'
import { addDays, defaultDay, formatDay, formatTime, isoWeek, monday, today, weekNumber } from '@/lib/dates'
import { useI18n } from '@/lib/i18n'
import { buildIndex, parse, search, type Chip, type Result } from '@/lib/search'
import { cn } from '@/lib/utils'

/** What a result does when picked; the schedule carries it out. */
export type SearchAction =
  | { type: 'focus'; lesson: Lesson }
  | { type: 'open'; lesson: Lesson }
  | { type: 'openRef'; date: string; instance: string }
  | { type: 'day'; date: string }

const host = (u: string) => {
  try {
    return new URL(u).host.replace(/^www\./, '')
  } catch {
    return u
  }
}

/** Monday of an ISO week like "2026-W48". */
const mondayOfWeek = (week: string) => {
  const [y, w] = week.split('-W').map(Number)
  return addDays(monday(`${y}-01-04`), (w - 1) * 7)
}

const reduced = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches

/**
 * The search button in the schedule's header. It grows into a search bar that
 * understands plain questions ("volgende les economie", "huiswerk voor
 * economie volgende week donderdag", "itslearning weekplanning 48") and
 * answers from the weeks around today, which it loads when it opens.
 */
export function ScheduleSearch({ accountId, person, homework, onAction }: { accountId: string; person: string; homework: boolean; onAction: (a: SearchAction) => void }) {
  const { t, lang, locale } = useI18n()
  const [open, setOpen] = useState(false)
  const [armed, setArmed] = useState(false)
  const [text, setText] = useState('')
  const [active, setActive] = useState(0)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const barRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const closing = useRef(false)

  // The weeks around today, shared with the schedule itself so jumping there is instant too.
  const now = today()
  const around = useMemo(() => [-7, 0, 7, 14, 21, 28].map((d) => isoWeek(addDays(now, d))), [now])
  const [extra, setExtra] = useState<string[]>([])
  const weeks = useMemo(() => [...new Set([...around, ...extra])], [around, extra])
  const schedules = useQueries({
    queries: weeks.map((week) => ({
      queryKey: ['schedule', accountId, week, person, lang],
      queryFn: () => api.schedule(accountId, { week, person: person || undefined, lang }),
      enabled: armed,
      staleTime: 60_000,
    })),
  })
  const hw = useQuery({ queryKey: ['homework', accountId], queryFn: () => api.homework.list(accountId), staleTime: 30_000, enabled: armed && homework && !person })
  const stamp = schedules.map((s) => s.dataUpdatedAt).join()
  const lessons = useMemo(() => {
    const seen = new Set<string>()
    const out: Lesson[] = []
    for (const s of schedules) for (const l of s.data?.data.lessons ?? []) if (!seen.has(l.id)) {
        seen.add(l.id)
        out.push(l)
      }
    return out
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stamp])
  const index = useMemo(() => buildIndex(lessons), [lessons])
  const homeworkList: Homework[] = useMemo(() => (homework && !person ? (hw.data ?? []) : []), [hw.data, homework, person])

  const q = text.trim()
  const parsed = useMemo(() => (q ? parse(q, index, now, lang) : null), [q, index, now, lang])
  const results = useMemo(() => (parsed ? search(parsed, lessons, homeworkList, Date.now(), now) : []), [parsed, lessons, homeworkList, now])
  // A week the query names that is not loaded yet: fetch it.
  useEffect(() => {
    const missing = parsed?.weeks.filter((w) => !weeks.includes(w)) ?? []
    if (missing.length) setExtra((e) => [...new Set([...e, ...missing])].slice(-6))
  }, [parsed, weeks])
  const loading = schedules.some((s, i) => s.isPending && armed && (i < 3 || parsed?.weeks.includes(weeks[i])))
  useEffect(() => setActive(0), [q])

  // Opening: the button stretches into the bar (a clip-path from the button's box to the bar's).
  useLayoutEffect(() => {
    const bar = barRef.current
    const btn = buttonRef.current
    if (!open || !bar || !btn) return
    inputRef.current?.focus()
    if (reduced()) return
    const b = bar.getBoundingClientRect()
    const r = btn.getBoundingClientRect()
    bar.animate(
      [
        { clipPath: `inset(${r.top - b.top}px ${b.right - r.right}px ${b.bottom - r.bottom}px ${r.left - b.left}px round 999px)`, opacity: 0.6 },
        { clipPath: 'inset(0px 0px 0px 0px round 14px)', opacity: 1 },
      ],
      { duration: 280, easing: 'cubic-bezier(.2,.9,.25,1)' },
    )
  }, [open])

  const close = useCallback(() => {
    const bar = barRef.current
    const btn = buttonRef.current
    if (closing.current) return
    const done = () => {
      closing.current = false
      setOpen(false)
      setText('')
    }
    if (!bar || !btn || reduced()) return done()
    closing.current = true
    const b = bar.getBoundingClientRect()
    const r = btn.getBoundingClientRect()
    bar
      .animate(
        [
          { clipPath: 'inset(0px 0px 0px 0px round 14px)', opacity: 1 },
          { clipPath: `inset(${r.top - b.top}px ${b.right - r.right}px ${b.bottom - r.bottom}px ${r.left - b.left}px round 999px)`, opacity: 0.4 },
        ],
        { duration: 200, easing: 'cubic-bezier(.4,0,.6,1)' },
      )
      .finished.then(done, done)
  }, [])

  const show = () => {
    setArmed(true)
    setOpen(true)
  }

  // "/" or Ctrl/Cmd+K opens search from anywhere on the schedule.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const typing = e.target instanceof HTMLElement && (e.target.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName))
      if (((e.key === 'k' && (e.metaKey || e.ctrlKey)) || (e.key === '/' && !typing)) && !document.querySelector('[data-slot="drawer-popup"],[data-slot="dialog-content"]')) {
        e.preventDefault()
        setArmed(true)
        setOpen(true)
        inputRef.current?.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const run = (r: Result) => {
    switch (r.kind) {
      case 'plan':
        // Opened right here, in the click, so no popup blocker gets in the way.
        window.open(r.url, '_blank', 'noopener,noreferrer')
        break
      case 'lesson':
        onAction(r.open ? { type: 'open', lesson: r.lesson } : { type: 'focus', lesson: r.lesson })
        break
      case 'noPlan':
        onAction({ type: 'open', lesson: r.lesson })
        break
      case 'homework':
        onAction(r.lesson ? { type: 'open', lesson: r.lesson } : { type: 'openRef', date: r.homework.lesson!.date, instance: r.homework.lesson!.instance })
        break
      case 'day':
        onAction({ type: 'day', date: r.date })
        break
      case 'missing':
        return
      case 'week': {
        const m = mondayOfWeek(r.week)
        onAction({ type: 'day', date: isoWeek(defaultDay()) === r.week ? defaultDay() : m })
        break
      }
    }
    close()
  }

  const dayName = (d: string) => (d === now ? t('searchToday') : d === addDays(now, 1) ? t('searchTomorrow') : formatDay(d, locale, { weekday: 'short', day: 'numeric', month: 'short' }))
  const exampleSubject = index.subjects.find((s) => s.name.length > 3)?.name.toLowerCase() ?? (lang === 'nl' ? 'economie' : 'economics')
  const examples = [
    t('searchExNext', { subject: exampleSubject }),
    ...(homework && !person ? [t('searchExHomework', { subject: exampleSubject })] : []),
    t('searchExPlan', { n: weekNumber(now) }),
    t('searchExCancel'),
    t('searchExNow'),
  ]

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        aria-label={t('search')}
        title={`${t('search')} (/)`}
        onPointerEnter={() => setArmed(true)}
        onFocus={() => setArmed(true)}
        onClick={show}
        className={cn('inline-flex size-11 shrink-0 items-center justify-center rounded-lg transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none dark:hover:bg-muted/50', open && 'invisible')}
      >
        <MelonSearchIcon className="size-5" />
      </button>
      {open && (
        <div
          ref={barRef}
          className="absolute inset-y-0 right-0 left-0 z-30 flex items-center md:left-auto md:w-[min(38rem,100%)]"
          onBlur={(e) => {
            if (!e.currentTarget.contains(e.relatedTarget) && !q) close()
          }}
        >
          <div className="relative w-full">
            <label className="flex h-11 items-center gap-2 rounded-[14px] border border-honeydew bg-card pr-1 pl-3 shadow-sm ring-3 ring-honeydew/30">
              {loading && !results.length ? <Loader2 className="size-5 shrink-0 animate-spin text-muted-foreground" /> : <MelonSearchIcon className="size-5 shrink-0 text-honeydew-ink" />}
              <input
                ref={inputRef}
                type="search"
                role="combobox"
                aria-expanded="true"
                aria-controls="schedule-search-results"
                aria-activedescendant={results[active] ? `search-${results[active].key}` : undefined}
                value={text}
                onChange={(e) => setText(e.target.value)}
                onKeyDown={(e) => {
                  const keys: Record<string, () => void> = {
                    Escape: close,
                    ArrowDown: () => setActive((a) => Math.min(a + 1, results.length - 1)),
                    ArrowUp: () => setActive((a) => Math.max(a - 1, 0)),
                    Enter: () => results[active] && run(results[active]),
                  }
                  if (!keys[e.key]) return
                  e.preventDefault()
                  keys[e.key]()
                }}
                placeholder={t('searchPlaceholder')}
                aria-label={t('search')}
                autoComplete="off"
                autoCorrect="off"
                spellCheck={false}
                enterKeyHint="go"
                className="h-full min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-muted-foreground [&::-webkit-search-cancel-button]:hidden"
              />
              <button type="button" onClick={close} aria-label={t('searchClose')} className="grid size-9 shrink-0 place-items-center rounded-[10px] text-muted-foreground hover:bg-muted">
                <X className="size-4" />
              </button>
            </label>

            <div
              id="schedule-search-results"
              role="listbox"
              aria-label={t('search')}
              className="absolute inset-x-0 top-full mt-2 max-h-[min(70dvh,34rem)] overflow-y-auto overscroll-contain rounded-2xl border bg-popover p-1.5 text-popover-foreground shadow-xl animate-in fade-in-0 slide-in-from-top-1 duration-200 motion-reduce:animate-none"
            >
              {parsed && parsed.chips.length > 0 && (
                <div className="flex flex-wrap items-center gap-1 px-2 pt-1 pb-2" aria-live="polite">
                  <Sparkles className="mr-0.5 size-3.5 text-honeydew-ink" aria-hidden="true" />
                  {parsed.chips.map((c, i) => (
                    <ChipView key={i} chip={c} dayName={dayName} />
                  ))}
                </div>
              )}
              {!q && (
                <div className="grid gap-1 p-1">
                  <p className="px-1.5 pt-0.5 pb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">{t('searchTry')}</p>
                  {examples.map((ex) => (
                    <button
                      key={ex}
                      type="button"
                      onClick={() => (setText(ex), inputRef.current?.focus())}
                      className="flex items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
                    >
                      <Search className="size-4 shrink-0 text-muted-foreground" />
                      <span className="flex-1">{ex}</span>
                      <ArrowRight className="size-4 text-muted-foreground" />
                    </button>
                  ))}
                </div>
              )}
              {q && results.length === 0 && (
                <p className="flex items-center gap-2 px-3 py-4 text-sm text-muted-foreground">
                  {loading ? <Loader2 className="size-4 animate-spin" /> : <Search className="size-4" />}
                  {loading ? t('searchLoading') : t('searchNothing')}
                </p>
              )}
              {results.length > 0 && (
                <ul className="grid gap-0.5">
                  {results.map((r, i) => (
                    <li key={r.key} id={`search-${r.key}`} role="option" aria-selected={i === active} style={{ animationDelay: `${i * 28}ms` }} className="animate-in fade-in-0 slide-in-from-top-1 fill-mode-both duration-200 motion-reduce:animate-none">
                      <button
                        type="button"
                        tabIndex={-1}
                        onPointerMove={() => setActive(i)}
                        onClick={() => run(r)}
                        className={cn('flex w-full items-center gap-3 rounded-xl px-2 py-2 text-left transition-colors focus-visible:outline-none', i === active ? 'bg-accent text-accent-foreground' : 'hover:bg-muted/60')}
                      >
                        <ResultView r={r} dayName={dayName} active={i === active} />
                      </button>
                    </li>
                  ))}
                </ul>
              )}
              {q && results.length > 0 && <p className="hidden px-2.5 pt-1.5 pb-0.5 text-xs text-muted-foreground md:block">{t('searchKeys')}</p>}
            </div>
          </div>
        </div>
      )}
    </>
  )
}

/** Melon's magnifier: a melon slice for a lens, its rind for the rim. */
export function MelonSearchIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className} aria-hidden="true">
      <circle cx="10.5" cy="10.5" r="6.75" />
      <path d="M6.6 11.2a4 4 0 0 0 7.8 0" stroke="var(--watermelon)" strokeWidth={2.2} />
      <circle cx="9" cy="12.4" r=".55" fill="currentColor" stroke="none" />
      <circle cx="12" cy="12.4" r=".55" fill="currentColor" stroke="none" />
      <path d="m20 20-4.4-4.4" />
    </svg>
  )
}

function ChipView({ chip, dayName }: { chip: Chip; dayName: (d: string) => string }) {
  const { t } = useI18n()
  const base = 'inline-flex h-6 items-center rounded-full px-2 text-xs font-medium'
  switch (chip.k) {
    case 'subject':
      return <span className={cn(base, 'bg-honeydew-tint text-honeydew-ink')}>{chip.name}</span>
    case 'teacher':
      return <span className={cn(base, 'bg-honeydew-tint text-honeydew-ink')}>{chip.name}</span>
    case 'room':
      return <span className={cn(base, 'bg-secondary')}>{t('searchRoom', { room: chip.name })}</span>
    case 'intent':
      return <span className={cn(base, 'bg-cantaloupe-tint text-cantaloupe-ink')}>{t(`searchIntent_${chip.intent}`)}</span>
    case 'when':
      return <span className={cn(base, 'bg-secondary')}>{t(`searchWhen_${chip.when}`)}</span>
    case 'date':
      return <span className={cn(base, 'bg-secondary')}>{dayName(chip.date)}</span>
    case 'week':
      return <span className={cn(base, 'bg-secondary')}>{t('searchWeek', { n: Number(chip.week.slice(-2)) })}</span>
    case 'period':
      return <span className={cn(base, 'bg-secondary')}>{t('searchPeriod', { n: chip.n })}</span>
    case 'text':
      return <span className={cn(base, 'border border-dashed text-muted-foreground')}>“{chip.text}”</span>
  }
}

function Tile({ tone, children }: { tone: 'honeydew' | 'cantaloupe' | 'watermelon' | 'muted'; children: ReactNode }) {
  const tones = { honeydew: 'bg-honeydew-tint text-honeydew-ink', cantaloupe: 'bg-cantaloupe-tint text-cantaloupe-ink', watermelon: 'bg-watermelon-tint text-watermelon-ink', muted: 'bg-secondary text-muted-foreground' }
  return <span className={cn('grid size-10 shrink-0 place-items-center rounded-xl [&>svg]:size-5', tones[tone])}>{children}</span>
}

function ResultView({ r, dayName, active }: { r: Result; dayName: (d: string) => string; active: boolean }) {
  const { t, locale } = useI18n()
  const when = (l: Lesson) =>
    [dayName(l.date), l.period ? t('searchPeriod', { n: l.period.from }) : null, `${formatTime(l.start, locale)}–${formatTime(l.end, locale)}`, l.locations.join(', ') || null].filter(Boolean).join(' · ')
  const hint = (text: string, icon: ReactNode) => (
    <span className={cn('hidden shrink-0 items-center gap-1 text-xs text-muted-foreground sm:flex', !active && 'sm:hidden')}>
      {text}
      {icon}
    </span>
  )
  const enter = <CornerDownLeft className="size-3.5" />
  switch (r.kind) {
    case 'lesson': {
      const l = r.lesson
      const cancelled = l.status === 'cancelled'
      const tone = cancelled ? 'watermelon' : l.status === 'changed' || l.type === 'detention' ? 'cantaloupe' : 'honeydew'
      return (
        <>
          <Tile tone={tone}>{l.type === 'detention' ? <Hourglass /> : cancelled ? <CalendarX /> : r.open ? <NotebookPen /> : <Clock />}</Tile>
          <span className="grid min-w-0 flex-1 gap-0.5">
            <span className="flex min-w-0 items-center gap-2">
              <span className={cn('truncate font-medium', cancelled && 'line-through decoration-watermelon decoration-2')}>{l.title}</span>
              {r.why && <span className="shrink-0 rounded-full bg-honeydew px-1.5 py-px text-[0.7rem] font-semibold text-primary-foreground">{t(r.why === 'next' ? 'searchNext' : r.why === 'prev' ? 'searchPrev' : 'searchNow')}</span>}
              {cancelled && <span className="shrink-0 rounded-full bg-watermelon-tint px-1.5 py-px text-[0.7rem] font-semibold text-watermelon-ink">{t('cancelled')}</span>}
            </span>
            <span className="truncate text-sm text-muted-foreground first-letter:uppercase">{when(l)}</span>
          </span>
          {hint(r.open ? t('searchActionOpen') : t('searchActionShow'), enter)}
        </>
      )
    }
    case 'homework': {
      const h = r.homework
      const l = h.lesson!
      return (
        <>
          <Tile tone="cantaloupe">
            <NotebookPen />
          </Tile>
          <span className="grid min-w-0 flex-1 gap-0.5">
            <span className="truncate font-medium">{h.text.replace(/^\s*([-*]|\d+\.)\s+(\[[ xX]\]\s+)?/gm, '').replace(/[*_`#>]/g, '').split('\n').filter(Boolean).join(' · ')}</span>
            <span className="truncate text-sm text-muted-foreground first-letter:uppercase">
              {t('searchHomeworkFor', { lesson: l.title })} · {dayName(l.date)}
            </span>
          </span>
          {hint(t('searchActionOpen'), enter)}
        </>
      )
    }
    case 'plan':
      return (
        <>
          <Tile tone="honeydew">
            <CalendarRange />
          </Tile>
          <span className="grid min-w-0 flex-1 gap-0.5">
            <span className="truncate font-medium">{t('searchPlan', { subject: r.lesson.subjects[0]?.name ?? r.lesson.title })}</span>
            <span className="truncate text-sm text-muted-foreground">{t('searchPlanWeek', { n: weekNumber(r.lesson.date), host: host(r.url) })}</span>
          </span>
          <span className={cn('flex shrink-0 items-center gap-1 text-xs text-muted-foreground')}>
            <span className={cn('hidden sm:inline', !active && 'sm:hidden')}>{t('searchActionTab')}</span>
            <ExternalLink className="size-4" />
          </span>
        </>
      )
    case 'noPlan':
      return (
        <>
          <Tile tone="muted">
            <CalendarRange />
          </Tile>
          <span className="grid min-w-0 flex-1 gap-0.5">
            <span className="truncate font-medium">{t('searchNoPlan', { subject: r.lesson.subjects[0]?.name ?? r.lesson.title })}</span>
            <span className="truncate text-sm text-muted-foreground">{t('searchNoPlanHint', { n: weekNumber(r.lesson.date) })}</span>
          </span>
          {hint(t('searchActionOpen'), enter)}
        </>
      )
    case 'day':
      return (
        <>
          <Tile tone="honeydew">
            <CalendarDays />
          </Tile>
          <span className="min-w-0 flex-1 truncate font-medium first-letter:uppercase">{t('searchGoDay', { day: formatDay(r.date, locale, { weekday: 'long', day: 'numeric', month: 'long' }) })}</span>
          {hint(t('searchActionShow'), enter)}
        </>
      )
    case 'missing':
      return (
        <>
          <Tile tone="muted">
            <SearchX />
          </Tile>
          <span className="grid min-w-0 flex-1 gap-0.5">
            <span className="truncate font-medium">{t('searchMissing', { subject: r.name[0].toUpperCase() + r.name.slice(1) })}</span>
            <span className="text-sm text-muted-foreground">{t('searchMissingHint')}</span>
          </span>
        </>
      )
    case 'week':
      return (
        <>
          <Tile tone="honeydew">
            <CalendarDays />
          </Tile>
          <span className="grid min-w-0 flex-1 gap-0.5">
            <span className="truncate font-medium">{t('searchGoWeek', { n: Number(r.week.slice(-2)) })}</span>
            <span className="truncate text-sm text-muted-foreground">
              {formatDay(mondayOfWeek(r.week), locale, { day: 'numeric', month: 'short' })} – {formatDay(addDays(mondayOfWeek(r.week), 4), locale, { day: 'numeric', month: 'short' })}
            </span>
          </span>
          {hint(t('searchActionShow'), enter)}
        </>
      )
  }
}
