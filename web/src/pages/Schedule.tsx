import { Fragment, useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import { ArrowDown, CalendarCheck, ChevronLeft, ChevronRight, CloudOff, Megaphone, Plus, RefreshCw, Settings, TriangleAlert, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { AccountMenu } from '@/components/AccountMenu'
import { Banner } from '@/components/bits'
import { CodeLookup } from '@/components/CodeLookup'
import { DetentionDialog, type PickedPeriods } from '@/components/DetentionForm'
import { HomeworkMenu } from '@/components/Homework'
import { LessonCard, LessonDrawer, LessonGroup, SplitCard } from '@/components/Lesson'
import { detentionSlots, isDetention, layoutWeek, lineOf, spanOf } from '@/lib/layoutWeek'
import { LinkDialog } from '@/components/LinkForm'
import { Logo } from '@/components/Logo'
import { PeopleSearch } from '@/components/PeopleSearch'
import { api, type Detention, type Lesson, type Person } from '@/lib/api'
import { addDays, defaultDay, formatDay, formatTime, isoWeek, monday, today, weekNumber } from '@/lib/dates'
import { useNow, usePullToRefresh } from '@/lib/hooks'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import { toast } from 'sonner'

/** Puts a cell on the desktop week grid at the column and lines its CSS variables name. */
const place = 'md:[grid-column:var(--c)] md:[grid-row:var(--r)/span_var(--s)] md:pb-3'

// The day being looked at outlives this page: switching to another child
// remounts it, and so does a trip to the admin dashboard or settings. It is
// kept for the browser tab, and only for the calendar day it was picked on,
// so tomorrow starts at today again.
/** Minutes since midnight as 13:05. */
const hhmm = (m: number) => `${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`

const DAY_KEY = 'melon.day'

function readDay(): string {
  try {
    const saved = JSON.parse(sessionStorage.getItem(DAY_KEY) ?? 'null') as { day?: string; on?: string } | null
    if (saved?.on === today() && typeof saved.day === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(saved.day)) return saved.day
  } catch {
    /* storage blocked or garbage */
  }
  return defaultDay()
}

/**
 * A link that opens one lesson (the calendar feed has one on every event):
 * /?account=acc_…&day=2026-11-25&lesson=<instance>. Only for the account it
 * names, which Home switches to first.
 */
function readLessonLink(accountId: string): { day: string; lesson: string } | null {
  const q = new URLSearchParams(window.location.search)
  const day = q.get('day') ?? ''
  if (q.get('account') !== accountId || !/^\d{4}-\d{2}-\d{2}$/.test(day)) return null
  return { day, lesson: q.get('lesson') ?? '' }
}

function useRememberedDay(initial?: string) {
  const [day, setDay] = useState(() => initial ?? readDay())
  useEffect(() => {
    try {
      sessionStorage.setItem(DAY_KEY, JSON.stringify({ day, on: today() }))
    } catch {
      /* private mode */
    }
  }, [day])
  return [day, setDay] as const
}

export function Schedule() {
  const { t, lang, locale, plural, error } = useI18n()
  const { active, reload } = useSession()
  const account = active!
  const qc = useQueryClient()

  const [link, setLink] = useState(() => readLessonLink(account.id))
  const navigate = useNavigate()
  const [day, setDay] = useRememberedDay(link?.day)
  const [person, setPerson] = useState<Person | null>(null)
  const [openLesson, setOpenLesson] = useState<Lesson | null>(null)
  const [relinking, setRelinking] = useState(false)
  const [manual, setManual] = useState(false)
  // Adding a detention: the dialog, or the dialog stepped aside to pick periods on the schedule.
  const [adding, setAdding] = useState<'no' | 'form' | 'picking'>('no')
  const [detentionSession, setDetentionSession] = useState(0)
  const [picked, setPicked] = useState<PickedPeriods | null>(null)
  const [draft, setDraft] = useState<PickedPeriods | null>(null)
  // The detention being changed, with its lesson for the subject name; null when adding.
  const [editing, setEditing] = useState<{ detention: Detention; lesson: Lesson } | null>(null)
  const picking = adding === 'picking'
  const now = useNow(15_000)

  // Someone else's schedule belongs to the account it was found with.
  useEffect(() => setPerson(null), [account.id])

  const week = isoWeek(day)
  const key = useMemo(() => ['schedule', account.id, week, person?.code ?? '', lang], [account.id, week, person, lang])
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.schedule(account.id, { week, person: person?.code, lang }),
    // Fresh beats fast: always refetch on focus and keep polling while open.
    staleTime: 0,
    refetchOnWindowFocus: 'always',
    refetchOnReconnect: 'always',
    refetchInterval: 60_000,
    placeholderData: keepPreviousData,
    retry: (count, err) => count < 2 && [0, 502, 503].includes((err as { status?: number }).status ?? -1),
  })
  const announcements = useQuery({
    queryKey: ['announcements', account.id],
    queryFn: () => api.announcements(account.id),
    staleTime: 2 * 60_000,
    enabled: !person,
  })

  // A manual refresh asks the server to skip its short cache.
  const refresh = useCallback(async () => {
    setManual(true)
    try {
      const fresh = await api.schedule(account.id, { week, person: person?.code, lang, fresh: true })
      qc.setQueryData(key, fresh)
    } catch {
      await query.refetch()
    } finally {
      setManual(false)
    }
  }, [account.id, week, person, lang, key, qc, query])
  const ptr = usePullToRefresh(refresh)

  const mon = monday(day)
  const todayStr = today()
  const lessons = useMemo(() => query.data?.data.lessons ?? [], [query.data])
  const byDay = useMemo(() => {
    const map = new Map<string, Lesson[]>()
    for (const l of lessons) map.set(l.date, [...(map.get(l.date) ?? []), l])
    return map
  }, [lessons])
  // Monday to Friday, plus weekend days only when something is on them.
  const days = useMemo(() => [0, 1, 2, 3, 4, 5, 6].map((i) => addDays(mon, i)).filter((d, i) => i < 5 || byDay.has(d) || d === day), [mon, byDay, day])
  const loadedWeek = query.data ? isoWeek(query.data.data.from) === week : false
  const loading = query.isPending || (query.isPlaceholderData && !loadedWeek)

  // Open the lesson a link points at once its week is in, then tidy the URL.
  useEffect(() => {
    if (!link || !loadedWeek || query.isFetching) return
    const l = lessons.find((x) => x.instance === link.lesson || x.id === link.lesson)
    if (l) setOpenLesson(l)
    setLink(null)
    navigate('/', { replace: true })
  }, [link, loadedWeek, query.isFetching, lessons, navigate])

  const dayLessons = byDay.get(day) ?? []
  const dayChanges = dayLessons.filter((l) => l.status !== 'scheduled')
  const fetchedAt = query.data?.meta.fetchedAt
  const stale = query.data?.meta.stale
  const expired = (query.error as { code?: string } | null)?.code === 'account_link_expired'

  const age = fetchedAt ? Math.max(0, Math.round((now - new Date(fetchedAt).getTime()) / 1000)) : null
  const ago = age === null ? '' : age < 10 ? t('justNow') : age < 60 ? t('secondsAgo', { n: Math.floor(age / 10) * 10 }) : age < 3600 ? t('minutesAgo', { n: Math.floor(age / 60) }) : t('atTime', { time: formatTime(fetchedAt!, locale) })

  // Swipe left/right to change day on touch screens.
  const swipe = useRef<{ x: number; y: number } | null>(null)
  const onTouchStart = (e: React.TouchEvent) => (swipe.current = { x: e.touches[0].clientX, y: e.touches[0].clientY })
  const onTouchEnd = (e: React.TouchEvent) => {
    const s = swipe.current
    swipe.current = null
    if (!s || held.current) return
    const dx = e.changedTouches[0].clientX - s.x
    const dy = e.changedTouches[0].clientY - s.y
    if (Math.abs(dx) < 70 || Math.abs(dx) < Math.abs(dy) * 2) return
    const i = days.indexOf(day)
    if (dx < 0) setDay(i < days.length - 1 ? days[i + 1] : addDays(mon, 7))
    else setDay(i > 0 ? days[i - 1] : addDays(mon, -3))
  }

  const isNow = (l: Lesson) => new Date(l.start).getTime() <= now && now < new Date(l.end).getTime()
  const layout = useMemo(() => layoutWeek(byDay, days), [byDay, days])
  // Subjects a detention can be linked to: the ones on this week's schedule.
  const weekSubjects = useMemo(() => {
    const seen = new Map<string, string>()
    for (const l of lessons) if (!isDetention(l)) for (const s of l.subjects) if (!seen.has(s.code.toLowerCase())) seen.set(s.code.toLowerCase(), s.name)
    return [...seen].map(([code, name]) => ({ code, name })).sort((a, b) => a.name.localeCompare(b.name, locale))
  }, [lessons, locale])
  // Changing a detention: load it as stored (the lesson only shows what it turned into), then open the form.
  const editDetention = useCallback(
    async (l: Lesson) => {
      try {
        const list = await api.detentions(account.id, { from: l.date, to: l.date })
        const d = list.find((x) => x.id === l.id)
        if (!d) throw new Error('missing')
        setEditing({ detention: d, lesson: l })
        setOpenLesson(null)
        setPicked(null)
        setDetentionSession((n) => n + 1)
        setAdding('form')
      } catch {
        toast.error(t('detentionLoadFailed'))
      }
    },
    [account.id, t],
  )
  // A detention's own subject stays pickable even when it is not on this week's schedule.
  const editSubjects = useMemo(() => {
    const own = editing?.lesson.subjects[0]
    if (!own || weekSubjects.some((s) => s.code === own.code.toLowerCase())) return weekSubjects
    return [...weekSubjects, { code: own.code.toLowerCase(), name: own.name }]
  }, [editing, weekSubjects])
  // Picking a detention's periods, within one day. With a mouse: press and drag. On a touch screen the
  // page has to keep scrolling and swiping to other days, so a tap picks a period (a second tap stretches
  // it to there) and a long press starts a drag, like moving an event in a calendar app.
  const slots = useMemo(() => detentionSlots(layout.times), [layout.times])
  const anchor = useRef<PickedPeriods | null>(null)
  /** A long press is dragging: the page holds still and a sideways move is no swipe. */
  const held = useRef(false)
  const mainRef = useRef<HTMLElement>(null)
  const periodsAt = (x: number, y: number): PickedPeriods | null => {
    const el = document.elementFromPoint(x, y)?.closest<HTMLElement>('[data-pick-day]')
    return el ? { date: el.dataset.pickDay!, from: Number(el.dataset.pickFrom), to: Number(el.dataset.pickTo) } : null
  }
  const stretch = (x: number, y: number) => {
    const a = anchor.current
    const at = a && periodsAt(x, y)
    if (!a || !at || at.date !== a.date) return
    setDraft({ date: a.date, from: Math.min(a.from, at.from), to: Math.max(a.to, at.to) })
  }
  useEffect(() => {
    const stop = () => (anchor.current = null)
    window.addEventListener('pointerup', stop)
    return () => window.removeEventListener('pointerup', stop)
  }, [])
  useEffect(() => {
    const main = mainRef.current
    if (!picking || !main) return
    let timer = 0
    let start: { x: number; y: number; at: PickedPeriods } | null = null
    const down = (e: TouchEvent) => {
      const t = e.touches[0]
      const at = e.touches.length === 1 ? periodsAt(t.clientX, t.clientY) : null
      if (!at) return
      start = { x: t.clientX, y: t.clientY, at }
      timer = window.setTimeout(() => {
        held.current = true
        anchor.current = at
        setDraft(at)
        navigator.vibrate?.(10)
      }, 350)
    }
    const move = (e: TouchEvent) => {
      if (!start) return
      const t = e.touches[0]
      if (held.current) {
        e.preventDefault()
        stretch(t.clientX, t.clientY)
      } else if (Math.hypot(t.clientX - start.x, t.clientY - start.y) > 8) {
        // Scrolling or swiping, not picking.
        window.clearTimeout(timer)
        start = null
      }
    }
    const up = (e: TouchEvent) => {
      window.clearTimeout(timer)
      if (start && !held.current && e.type === 'touchend') {
        const at = start.at
        setDraft((d) => (!d || d.date !== at.date || (at.from >= d.from && at.to <= d.to) ? at : { date: at.date, from: Math.min(d.from, at.from), to: Math.max(d.to, at.to) }))
      }
      start = null
      anchor.current = null
      // After the swipe handler has seen it.
      window.setTimeout(() => (held.current = false))
    }
    const menu = (e: Event) => e.preventDefault()
    main.addEventListener('touchstart', down, { passive: true })
    main.addEventListener('touchmove', move, { passive: false })
    main.addEventListener('touchend', up)
    main.addEventListener('touchcancel', up)
    main.addEventListener('contextmenu', menu)
    return () => {
      window.clearTimeout(timer)
      main.removeEventListener('touchstart', down)
      main.removeEventListener('touchmove', move)
      main.removeEventListener('touchend', up)
      main.removeEventListener('touchcancel', up)
      main.removeEventListener('contextmenu', menu)
    }
  }, [picking]) // eslint-disable-line react-hooks/exhaustive-deps
  const onPickDown = (e: React.PointerEvent) => {
    const at = picking && e.pointerType !== 'touch' && periodsAt(e.clientX, e.clientY)
    if (!at) return
    e.preventDefault()
    anchor.current = at
    setDraft(at)
  }
  const onPickMove = (e: React.PointerEvent) => {
    if (e.pointerType !== 'touch') stretch(e.clientX, e.clientY)
  }
  /** What makes a cell of the schedule pickable while picking, and shows it when it is picked. */
  const pickable = (date: string, from: number, to: number) =>
    picking
      ? {
          'data-pick-day': date,
          'data-pick-from': from,
          'data-pick-to': to,
          className: cn('relative cursor-pointer select-none [-webkit-touch-callout:none] [&_*]:pointer-events-none', draft && draft.date === date && from <= draft.to && to >= draft.from && 'after:absolute after:inset-0 after:rounded-xl after:bg-cantaloupe/25 after:ring-2 after:ring-cantaloupe md:after:bottom-3'),
        }
      : { className: '' }
  const lessonGroup = (g: Lesson[]) =>
    g.length === 1 ? <LessonCard key={g[0].id} lesson={g[0]} isNow={isNow(g[0])} onOpen={() => setOpenLesson(g[0])} /> : <LessonGroup key={g[0].id} lessons={g} isNow={isNow} onOpen={setOpenLesson} />
  const busy = manual || ptr.refreshing || (query.isFetching && !query.isPending)

  return (
    <div className="mx-auto min-h-dvh w-full max-w-6xl 2xl:max-w-[96rem]">
      {/* Pull-to-refresh indicator */}
      <div
        aria-hidden={ptr.pull === 0 && !ptr.refreshing}
        className="pointer-events-none fixed inset-x-0 top-0 z-40 flex justify-center"
        style={{ transform: `translateY(${ptr.refreshing ? 16 : Math.min(ptr.pull, 1.2) * ptr.threshold - 48}px)`, transition: ptr.pull ? 'none' : 'transform .2s' }}
      >
        <span className="flex items-center gap-2 rounded-full border bg-popover px-3.5 py-2 text-sm shadow-md">
          {ptr.refreshing ? <RefreshCw className="size-4 animate-spin" /> : <ArrowDown className={cn('size-4 transition-transform', ptr.pull >= 1 && 'rotate-180')} />}
          {ptr.refreshing ? t('refreshing') : ptr.pull >= 1 ? t('releaseToRefresh') : t('pullToRefresh')}
        </span>
      </div>

      <header className="sticky top-0 z-20 border-b bg-background/90 px-3 pt-[max(0.5rem,env(safe-area-inset-top))] backdrop-blur md:px-6">
        <div className="flex items-center gap-1">
          <Logo className="mr-1 hidden text-2xl sm:inline-flex" withName />
          <div className="min-w-0 shrink sm:ml-3">
            <AccountMenu />
          </div>
          <button type="button" onClick={refresh} disabled={busy} className="flex min-h-9 shrink-0 items-center gap-1.5 rounded-full px-2 text-xs text-muted-foreground hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none" aria-label={t('refresh')}>
            <span aria-live="polite" className="max-w-[9rem] truncate sm:max-w-none">{busy ? t('refreshing') : fetchedAt ? t('updated', { ago }) : ''}</span>
            <RefreshCw className={cn('size-3.5', busy && 'animate-spin')} />
          </button>
          <span className="flex-1" />
          {/* Parents have no homework of their own. */}
          {!person && account.role !== 'parent' && <HomeworkMenu accountId={account.id} onPick={setDay} />}
          <Button variant="ghost" size="icon" className="size-11 shrink-0" aria-label={t('settings')} render={<Link to="/settings" />}>
            <Settings className="size-5" />
          </Button>
        </div>

        {/* Week navigation and day picker */}
        <div className="mt-1 flex items-center gap-1">
          <Button variant="ghost" size="icon" className="size-10" aria-label={t('previousWeek')} onClick={() => setDay(addDays(day, -7))}>
            <ChevronLeft className="size-5" />
          </Button>
          <span className="min-w-20 text-center text-sm font-medium tabular-nums">{t('week', { n: weekNumber(day) })}</span>
          <Button variant="ghost" size="icon" className="size-10" aria-label={t('nextWeek')} onClick={() => setDay(addDays(day, 7))}>
            <ChevronRight className="size-5" />
          </Button>
          {/* Right next to the arrows, so the way back is where you were clicking. */}
          {day !== defaultDay() && (
            <Button variant="outline" className="ml-1 h-9 rounded-full border-honeydew px-3.5" onClick={() => setDay(defaultDay())}>
              <CalendarCheck />
              {t('today')}
            </Button>
          )}
          {/* Detentions belong to your own schedule, not to someone you look up. */}
          {!person && (
            <Button variant="outline" size="icon" className="ml-auto size-9 rounded-full" aria-label={t('addDetention')} title={t('addDetention')} onClick={() => (setEditing(null), setDetentionSession((n) => n + 1), setPicked(null), setAdding('form'))}>
              <Plus />
            </Button>
          )}
        </div>
        <nav aria-label={t('week', { n: weekNumber(day) })} className="-mx-1 flex gap-1 pt-1 pb-2 md:hidden">
          {days.map((d) => {
            const ls = byDay.get(d) ?? []
            const cancelled = ls.some((l) => l.status === 'cancelled')
            const changed = ls.some((l) => l.status === 'changed' || isDetention(l))
            const selected = d === day
            return (
              <button
                key={d}
                type="button"
                onClick={() => setDay(d)}
                aria-current={selected ? 'date' : undefined}
                data-day={d}
                className={cn(
                  'relative flex h-14 min-w-0 flex-1 flex-col items-center justify-center rounded-xl text-sm transition-colors focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
                  selected ? 'bg-primary font-semibold text-primary-foreground' : 'hover:bg-muted',
                  d === todayStr && !selected && 'ring-1 ring-honeydew ring-inset',
                )}
              >
                <span className={cn('text-xs', !selected && 'text-muted-foreground')}>{formatDay(d, locale, { weekday: 'short' })}</span>
                <span className="text-base leading-tight tabular-nums">{formatDay(d, locale, { day: 'numeric' })}</span>
                {(cancelled || changed) && (
                  <span className="absolute top-1.5 right-1.5 flex gap-0.5">
                    {changed && <span className="size-2 rounded-full bg-cantaloupe ring-2 ring-background" />}
                    {cancelled && <span className="size-2 rounded-full bg-watermelon ring-2 ring-background" />}
                    <span className="sr-only">{plural('changesDay', ls.filter((l) => l.status !== 'scheduled').length)}</span>
                  </span>
                )}
              </button>
            )
          })}
        </nav>
        {/* Desktop day headers live in the sticky header, so the days stay visible while scrolling the week. */}
        <div aria-hidden="true" className="hidden gap-3 pt-1 pb-2 md:grid md:grid-flow-col md:auto-cols-fr">
          {days.map((d) => (
            <div key={d} className={cn('flex min-w-0 items-baseline gap-1.5 rounded-lg px-2 py-1 text-sm font-medium', d === todayStr && 'bg-primary text-primary-foreground')}>
              <span className="first-letter:uppercase">{formatDay(d, locale, { weekday: 'short' })}</span>
              <span className={cn('truncate', d !== todayStr && 'text-muted-foreground')}>{formatDay(d, locale, { day: 'numeric', month: 'short' })}</span>
            </div>
          ))}
        </div>
      </header>

      <main ref={mainRef} className={cn('grid gap-3 px-3 pt-3 pb-[max(2rem,env(safe-area-inset-bottom))] md:px-6', picking && 'pb-28')} onTouchStart={onTouchStart} onTouchEnd={onTouchEnd} onPointerDown={onPickDown} onPointerMove={onPickMove}>
        {account.canViewOthers && <PeopleSearch accountId={account.id} onPick={setPerson} />}
        {!account.canViewOthers && account.role === 'employee' && <CodeLookup onPick={setPerson} />}

        {person && (
          <Banner
            tone="honeydew"
            title={t('viewing', { name: person.name })}
            action={
              <Button variant="ghost" size="icon" className="-my-1.5 -mr-1.5 size-9 hover:bg-honeydew/30" aria-label={t('backToMine')} onClick={() => setPerson(null)}>
                <X />
              </Button>
            }
          />
        )}

        {expired && (
          <Banner tone="watermelon" icon={<TriangleAlert />} title={t('relinkTitle')} action={<Button variant="destructive" className="h-9" onClick={() => setRelinking(true)}>{t('relink')}</Button>}>
            {t('relinkText')}
          </Banner>
        )}
        {query.isError && !expired && (
          <Banner tone="watermelon" icon={<CloudOff />} title={error(query.error)} action={<Button variant="destructive" className="h-9" onClick={() => { void reload(); void query.refetch() }}>{t('retry')}</Button>} />
        )}
        {stale && fetchedAt && (
          <Banner tone="cantaloupe" icon={<CloudOff />} title={t('staleTitle')}>
            {t('staleText', { time: formatTime(fetchedAt, locale) })}
          </Banner>
        )}

        {/* Changes can't be missed: they are counted here, dotted in the day picker and edged on each card. */}
        {!loading && dayChanges.length > 0 && (
          <div className="md:hidden">
            <Banner
              tone={dayChanges.some((l) => l.status === 'cancelled') ? 'watermelon' : 'cantaloupe'}
              icon={<TriangleAlert />}
              title={plural(day === todayStr ? 'changesToday' : 'changesDay', dayChanges.length)}
              below={
                <ul className="grid gap-0.5">
                  {dayChanges.map((l) => (
                    <li key={l.id} className="truncate">
                      {l.period && <span className="font-medium">{t('period', { n: l.period.from })} </span>}
                      {l.title}: {l.status === 'cancelled' ? t('cancelled').toLowerCase() : l.changes.map((c) => t(`change_${c}`).toLowerCase()).join(', ')}
                    </li>
                  ))}
                </ul>
              }
            />
          </div>
        )}

        {!person && announcements.data && announcements.data.length > 0 && (
          <div className="grid gap-2">
            {announcements.data.map((a) => (
              <Banner key={a.id} tone="honeydew" icon={<Megaphone />} title={a.title || t('announcements')}>
                {a.text}
              </Banner>
            ))}
          </div>
        )}

        <h1 className="px-1 text-sm font-medium first-letter:uppercase md:hidden">{formatDay(day, locale, { weekday: 'long', day: 'numeric', month: 'long' })}</h1>

        {/* One grid for the week on desktop: a column per day and a row per period, so the hours line up across days. */}
        {/* Period rows are 1fr and break rows auto: in a grid without a set height, every fr row grows to the tallest one, so all periods get the same height. */}
        <div
          style={{ '--cols': days.length, '--rows': ['auto', ...Array.from({ length: layout.rows }, (_, k) => (k % 2 === 0 ? '1fr' : 'auto'))].join(' ') } as CSSProperties}
          className={cn('grid gap-4 md:grid-cols-[repeat(var(--cols),minmax(0,1fr))] md:grid-rows-[var(--rows)] md:gap-x-3 md:gap-y-0', query.isPlaceholderData && 'opacity-60')}
        >
          {days.map((d, i) => {
            const ls = byDay.get(d) ?? []
            const dl = layout.days.get(d)!
            // Line 1 holds the day headers; period rows start below them.
            const at = (line: number, span = 1) => ({ '--c': i + 1, '--r': line + 1, '--s': span }) as CSSProperties
            return (
              <section key={d} aria-label={formatDay(d, locale, { weekday: 'long', day: 'numeric', month: 'long' })} className={cn('min-w-0 gap-2 md:contents', d === day ? 'grid' : 'hidden')}>
                {loading ? (
                  [0, 1, 2, 3, 4].map((k) => (
                    <div key={k} style={at(2 * k + 1)} className={place}>
                      <Skeleton className="h-[4.75rem] rounded-xl" />
                    </div>
                  ))
                ) : ls.length === 0 && !picking ? (
                  !query.isError && (
                    <div style={at(1, layout.rows)} className={place}>
                      <div className="grid place-items-center gap-1 rounded-xl border border-dashed px-4 py-10 text-center md:py-6">
                        <Logo className="text-3xl opacity-80" />
                        <p className="mt-2 font-medium">{t('noLessons')}</p>
                        <p className="text-sm text-muted-foreground md:hidden">{t('noLessonsHint')}</p>
                      </div>
                    </div>
                  )
                ) : (
                  <>
                    {dl.cells.map((c) => {
                      const last = c.kind === 'free' ? c.period : c.to
                      const pause = dl.breaks.find((b) => b.after === last)
                      return (
                        <Fragment key={c.kind === 'free' ? `free-${c.period}` : `${c.from}-${(c.groups[0]?.[0] ?? c.detentions[0]).id}`}>
                          {c.kind === 'free' ? (
                            <div style={at(lineOf(layout, c.period))} {...pickable(d, c.period, c.period)} className={cn(place, pickable(d, c.period, c.period).className)}>
                              <div className="flex h-full min-h-11 items-center justify-center gap-2 rounded-xl border border-dashed border-muted-foreground/30 px-4 py-2 text-center text-sm text-muted-foreground">
                                <span className="font-medium tabular-nums">{t('period', { n: c.period })}</span>
                                <span>{t('free')}</span>
                              </div>
                            </div>
                          ) : (
                            <div style={at(lineOf(layout, c.from), spanOf(c.from, c.to))} {...pickable(d, c.from, c.to)} className={cn(place, pickable(d, c.from, c.to).className, 'grid gap-2', c.groups.length + c.detentions.length > 1 && !(c.groups.length === 1 && c.groups[0].length === 1) && 'content-start')}>
                              {c.detentions.length > 0 && c.groups.length === 1 && c.groups[0].length === 1 ? (
                                // One lesson with a detention on top of it: split the card, keep the period and time.
                                <SplitCard lesson={c.groups[0][0]} detentions={c.detentions} isNow={isNow(c.groups[0][0])} onOpen={setOpenLesson} />
                              ) : (
                                <>
                                  {c.groups.map((g) => lessonGroup(g))}
                                  {c.detentions.map((d) => (
                                    <LessonCard key={d.id} lesson={d} isNow={isNow(d)} onOpen={() => setOpenLesson(d)} />
                                  ))}
                                </>
                              )}
                            </div>
                          )}
                          {/* A break sits on the grid line between two periods. */}
                          {pause && (
                            <div style={at(lineOf(layout, last) + 1)} className={cn(place, 'flex items-center gap-2 px-1 text-xs leading-none text-muted-foreground')}>
                              <span aria-hidden="true" className="h-px flex-1 bg-border" />
                              {t('breakFor', { n: pause.minutes })}
                              <span aria-hidden="true" className="h-px flex-1 bg-border" />
                            </div>
                          )}
                        </Fragment>
                      )
                    })}
                    {/* While picking, the periods after the day's last one, so detention after school can be picked too. */}
                    {picking &&
                      [...slots.keys()]
                        .filter((p) => p >= layout.first && p > Math.max(layout.first - 1, ...dl.cells.map((c) => (c.kind === 'free' ? c.period : c.to))))
                        .sort((a, b) => a - b)
                        .map((p) => (
                          <div key={`pick-${p}`} style={at(lineOf(layout, p))} {...pickable(d, p, p)} className={cn(place, pickable(d, p, p).className)}>
                            <div className="flex h-full min-h-11 items-center justify-center gap-2 rounded-xl border border-dashed border-muted-foreground/30 px-4 py-2 text-center text-sm text-muted-foreground">
                              <span className="font-medium tabular-nums">{t('period', { n: p })}</span>
                              <span className="tabular-nums">
                                {hhmm(slots.get(p)!.start)} – {hhmm(slots.get(p)!.end)}
                              </span>
                            </div>
                          </div>
                        ))}
                    {dl.unplaced.map((g, k) => (
                      <div key={g[0].id} style={at(layout.rows + 1 + k)} className={place}>
                        {lessonGroup(g)}
                      </div>
                    ))}
                  </>
                )}
              </section>
            )
          })}
        </div>
      </main>

      {/* The open lesson follows the schedule, so homework added in the drawer shows up in it. */}
      <LessonDrawer lesson={openLesson && (lessons.find((l) => l.id === openLesson.id) ?? openLesson)} accountId={account.id} own={!person} homework={account.role !== 'parent'} onClose={() => setOpenLesson(null)} onEditDetention={editDetention} />
      <DetentionDialog
        open={adding === 'form'}
        onOpenChange={(o) => setAdding(o ? 'form' : 'no')}
        accountId={account.id}
        day={day}
        times={layout.times}
        subjects={editSubjects}
        onAdded={setDay}
        session={detentionSession}
        picked={picked}
        onPick={() => (setDraft(null), setAdding('picking'))}
        editing={editing?.detention ?? null}
      />
      {picking && (
        <div role="region" aria-label={t('addDetention')} className="fixed inset-x-0 bottom-0 z-40 border-t bg-card/95 px-4 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] shadow-lg backdrop-blur animate-in fade-in-0 slide-in-from-bottom-2 motion-reduce:animate-none">
          <div className="mx-auto flex max-w-lg items-center gap-2">
            <p className="flex-1 text-sm" aria-live="polite">
              {draft ? (
                <>
                  <span className="font-medium first-letter:uppercase">{formatDay(draft.date, locale, { weekday: 'short', day: 'numeric', month: 'short' })}</span>
                  {' · '}
                  {draft.from === draft.to ? t('period', { n: draft.from }) : `${t('period', { n: draft.from })} – ${t('period', { n: draft.to })}`}
                </>
              ) : (
                t('detentionPickBar')
              )}
            </p>
            <Button variant="ghost" className="h-10" onClick={() => setAdding('form')}>
              {t('cancel')}
            </Button>
            <Button className="h-10" disabled={!draft} onClick={() => (setPicked(draft), setAdding('form'))}>
              {t('detentionPickDone')}
            </Button>
          </div>
        </div>
      )}
      <LinkDialog open={relinking} onOpenChange={setRelinking} initialSchool={account.school} />
    </div>
  )
}
