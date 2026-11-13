import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { ArrowDown, ChevronLeft, ChevronRight, CloudOff, Megaphone, RefreshCw, Settings, TriangleAlert, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { AccountMenu } from '@/components/AccountMenu'
import { Banner } from '@/components/bits'
import { CodeLookup } from '@/components/CodeLookup'
import { LessonCard, LessonDrawer } from '@/components/Lesson'
import { LinkDialog } from '@/components/LinkForm'
import { Logo } from '@/components/Logo'
import { PeopleSearch } from '@/components/PeopleSearch'
import { api, type Lesson, type Person } from '@/lib/api'
import { addDays, defaultDay, formatDay, formatTime, isoWeek, monday, today, weekNumber } from '@/lib/dates'
import { useNow, usePullToRefresh } from '@/lib/hooks'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'

export function Schedule() {
  const { t, lang, locale, plural, error } = useI18n()
  const { active } = useSession()
  const account = active!
  const qc = useQueryClient()

  const [day, setDay] = useState(defaultDay)
  const [person, setPerson] = useState<Person | null>(null)
  const [openLesson, setOpenLesson] = useState<Lesson | null>(null)
  const [relinking, setRelinking] = useState(false)
  const [manual, setManual] = useState(false)
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
    if (!s) return
    const dx = e.changedTouches[0].clientX - s.x
    const dy = e.changedTouches[0].clientY - s.y
    if (Math.abs(dx) < 70 || Math.abs(dx) < Math.abs(dy) * 2) return
    const i = days.indexOf(day)
    if (dx < 0) setDay(i < days.length - 1 ? days[i + 1] : addDays(mon, 7))
    else setDay(i > 0 ? days[i - 1] : addDays(mon, -3))
  }

  const isNow = (l: Lesson) => new Date(l.start).getTime() <= now && now < new Date(l.end).getTime()
  const busy = manual || ptr.refreshing || (query.isFetching && !query.isPending)

  return (
    <div className="mx-auto min-h-dvh w-full max-w-6xl">
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
          <span className="flex-1" />
          {day !== defaultDay() && (
            <Button variant="outline" className="h-9 rounded-full px-3.5" onClick={() => setDay(defaultDay())}>
              {t('today')}
            </Button>
          )}
        </div>
        <nav aria-label={t('week', { n: weekNumber(day) })} className="-mx-1 flex gap-1 pt-1 pb-2 md:hidden">
          {days.map((d) => {
            const ls = byDay.get(d) ?? []
            const cancelled = ls.some((l) => l.status === 'cancelled')
            const changed = ls.some((l) => l.status === 'changed')
            const selected = d === day
            return (
              <button
                key={d}
                type="button"
                onClick={() => setDay(d)}
                aria-current={selected ? 'date' : undefined}
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
      </header>

      <main className="grid gap-3 px-3 pt-3 pb-[max(2rem,env(safe-area-inset-bottom))] md:px-6" onTouchStart={onTouchStart} onTouchEnd={onTouchEnd}>
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
          <Banner tone="watermelon" icon={<CloudOff />} title={error(query.error)} action={<Button variant="destructive" className="h-9" onClick={() => query.refetch()}>{t('retry')}</Button>} />
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

        <div className={cn('grid gap-4 md:grid-flow-col md:auto-cols-fr md:items-start md:gap-3', query.isPlaceholderData && 'opacity-60')}>
          {days.map((d) => {
            const ls = byDay.get(d) ?? []
            return (
              <section key={d} aria-label={formatDay(d, locale, { weekday: 'long', day: 'numeric', month: 'long' })} className={cn('min-w-0 gap-2', d === day ? 'grid' : 'hidden md:grid')}>
                <h2 className={cn('hidden items-baseline gap-1.5 rounded-lg px-2 py-1 text-sm font-medium md:flex', d === todayStr && 'bg-primary text-primary-foreground')}>
                  <span className="first-letter:uppercase">{formatDay(d, locale, { weekday: 'short' })}</span>
                  <span className={d === todayStr ? '' : 'text-muted-foreground'}>{formatDay(d, locale, { day: 'numeric', month: 'short' })}</span>
                </h2>
                {loading ? (
                  [0, 1, 2, 3, 4].map((i) => <Skeleton key={i} className="h-[4.75rem] rounded-xl" />)
                ) : ls.length === 0 ? (
                  !query.isError && (
                    <div className="grid place-items-center gap-1 rounded-xl border border-dashed px-4 py-10 text-center md:py-6">
                      <Logo className="text-3xl opacity-80" />
                      <p className="mt-2 font-medium">{t('noLessons')}</p>
                      <p className="text-sm text-muted-foreground md:hidden">{t('noLessonsHint')}</p>
                    </div>
                  )
                ) : (
                  ls.map((l) => <LessonCard key={l.id} lesson={l} isNow={isNow(l)} onOpen={() => setOpenLesson(l)} />)
                )}
              </section>
            )
          })}
        </div>
      </main>

      <LessonDrawer lesson={openLesson} accountId={account.id} onClose={() => setOpenLesson(null)} />
      <LinkDialog open={relinking} onOpenChange={setRelinking} initialSchool={account.school} />
    </div>
  )
}
