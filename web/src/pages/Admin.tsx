import { useMemo, useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import { ArrowLeft, Check, Eye, KeyRound, Loader2, LockOpen, LogOut, Plus, RotateCcw, Search, ShieldCheck, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Avatar, Chip, ConfirmDialog, CopyField } from '@/components/bits'
import { PinInput } from '@/components/SignInForm'
import { api, type AdminOverview, type AdminProfile, type AdminSubject, type Suggestion, type UsageChannel } from '@/lib/api'
import { useDebounced } from '@/lib/hooks'
import { useI18n, type Key } from '@/lib/i18n'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'

function Section({ title, text, action, children }: { title: string; text?: string; action?: ReactNode; children: ReactNode }) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div className="grid gap-1">
          <CardTitle className="text-base">{title}</CardTitle>
          {text && <CardDescription>{text}</CardDescription>}
        </div>
        {action}
      </CardHeader>
      <CardContent className="grid grid-cols-1 gap-3">{children}</CardContent>
    </Card>
  )
}

function useFormat() {
  const { locale } = useI18n()
  return useMemo(() => {
    const num = new Intl.NumberFormat(locale)
    const day = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short' })
    const dayLong = new Intl.DateTimeFormat(locale, { weekday: 'short', day: 'numeric', month: 'short' })
    const when = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })
    return {
      n: (v: number) => num.format(v),
      day: (iso: string) => day.format(new Date(iso)),
      // "YYYY-MM-DD" is a calendar day, not an instant: read it at noon so no timezone moves it.
      dayOnly: (d: string) => dayLong.format(new Date(`${d}T12:00:00`)),
      when: (iso: string) => when.format(new Date(iso)),
    }
  }, [locale])
}

// ---- Overview ----

function Stat({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="grid content-start gap-0.5 rounded-xl border bg-card p-3.5">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-2xl font-semibold tabular-nums">{value}</span>
      {sub && <span className="text-xs text-muted-foreground">{sub}</span>}
    </div>
  )
}

const CHANNELS: UsageChannel[] = ['session', 'api_key', 'oauth', 'calendar']

function days(since: string, today: string): string[] {
  const out: string[] = []
  for (let d = new Date(`${since}T12:00:00Z`); d.toISOString().slice(0, 10) <= today; d.setUTCDate(d.getUTCDate() + 1)) out.push(d.toISOString().slice(0, 10))
  return out
}

/** One series (lessons per day) as bars. Hovering or focusing a day shows its numbers in the readout above. */
function UsageChart({ data }: { data: AdminOverview }) {
  const { t } = useI18n()
  const f = useFormat()
  const series = useMemo(() => {
    const by = new Map<string, { lessons: number; requests: number }>()
    for (const u of data.usage) {
      const v = by.get(u.day) ?? { lessons: 0, requests: 0 }
      v.lessons += u.lessons
      v.requests += u.requests
      by.set(u.day, v)
    }
    return days(data.since, data.today).map((day) => ({ day, ...(by.get(day) ?? { lessons: 0, requests: 0 }) }))
  }, [data])
  const [hover, setHover] = useState<number | null>(null)
  const max = Math.max(1, ...series.map((s) => s.lessons))
  const shown = series[hover ?? series.length - 1]
  return (
    <Section title={t('usageTitle')} text={t('usageText')}>
      <p className="flex flex-wrap items-baseline gap-x-2 text-sm" aria-live="polite">
        <span className="font-medium first-letter:uppercase">{f.dayOnly(shown.day)}</span>
        <span className="text-muted-foreground">{t('usageTip', { lessons: f.n(shown.lessons), requests: f.n(shown.requests) })}</span>
      </p>
      <div className="relative" onMouseLeave={() => setHover(null)}>
        <div className="pointer-events-none absolute inset-x-0 top-0 border-t border-dashed border-border" />
        <span className="pointer-events-none absolute top-0 right-0 -translate-y-full pb-0.5 text-[11px] text-muted-foreground tabular-nums">{f.n(max)}</span>
        <div className="flex h-36 items-end gap-0.5 border-b border-border" aria-hidden="true">
          {series.map((s, i) => (
            <div key={s.day} className="flex h-full min-w-0 flex-1 items-end" onMouseEnter={() => setHover(i)}>
              <div
                className={cn('w-full rounded-t-[4px] bg-honeydew transition-opacity', hover !== null && hover !== i && 'opacity-50')}
                style={{ height: s.lessons ? `max(2px, ${(s.lessons / max) * 100}%)` : 0 }}
              />
            </div>
          ))}
        </div>
        <div className="mt-1 flex justify-between text-[11px] text-muted-foreground">
          <span>{f.day(`${series[0].day}T12:00:00`)}</span>
          <span>{f.day(`${series[series.length - 1].day}T12:00:00`)}</span>
        </div>
      </div>
      <table className="sr-only">
        <caption>{t('usageTable')}</caption>
        <thead>
          <tr>
            <th>{t('colDay')}</th>
            <th>{t('colLessons')}</th>
            <th>{t('colRequests')}</th>
          </tr>
        </thead>
        <tbody>
          {series.map((s) => (
            <tr key={s.day}>
              <td>{s.day}</td>
              <td>{s.lessons}</td>
              <td>{s.requests}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </Section>
  )
}

function Channels({ data }: { data: AdminOverview }) {
  const { t } = useI18n()
  const f = useFormat()
  const totals = CHANNELS.map((c) => {
    const rows = data.usage.filter((u) => u.channel === c)
    return { c, lessons: rows.reduce((a, u) => a + u.lessons, 0), requests: rows.reduce((a, u) => a + u.requests, 0) }
  })
  return (
    <Section title={t('usageByChannel')}>
      <dl className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {totals.map(({ c, lessons, requests }) => (
          <div key={c} className="grid gap-0.5 rounded-xl border p-3">
            <dt className="text-sm text-muted-foreground">{t(`channel_${c}` as Key)}</dt>
            <dd className="text-lg font-semibold tabular-nums">{f.n(lessons)}</dd>
            <dd className="text-xs text-muted-foreground tabular-nums">{t('requestsCount', { n: f.n(requests) })}</dd>
          </div>
        ))}
      </dl>
    </Section>
  )
}

function OverviewTab() {
  const { t } = useI18n()
  const f = useFormat()
  const q = useQuery({ queryKey: ['admin', 'overview'], queryFn: api.admin.overview, refetchInterval: 60_000 })
  if (!q.data) return <div className="grid grid-cols-2 gap-2 md:grid-cols-3">{[0, 1, 2, 3, 4, 5].map((i) => <Skeleton key={i} className="h-24 rounded-xl" />)}</div>
  const { stats: s, totals, schools } = q.data
  return (
    <>
      <div className="grid grid-cols-2 gap-2 md:grid-cols-3">
        <Stat label={t('statProfiles')} value={f.n(s.profiles)} sub={t('statProfilesSub', { n: f.n(s.profilesNew7d) })} />
        <Stat label={t('statActive')} value={f.n(s.active7d)} sub={t('statActiveSub', { day: f.n(s.active1d), month: f.n(s.active30d) })} />
        <Stat label={t('statLessons')} value={f.n(totals.lessons)} sub={t('statLessonsSub', { n: f.n(totals.requests) })} />
        <Stat label={t('statAccounts')} value={f.n(s.accounts)} sub={t('statAccountsSub', { students: f.n(s.students), staff: f.n(s.staff), parents: f.n(s.parents) })} />
        <Stat label={t('statPins')} value={f.n(s.withPin)} sub={t('statPinsSub', { n: f.n(s.lockedPins) })} />
        <Stat label={t('statConnections')} value={f.n(s.apiKeys + s.oauthApps + s.calendars)} sub={t('statConnectionsSub', { keys: f.n(s.apiKeys), apps: f.n(s.oauthApps), cal: f.n(s.calendars) })} />
      </div>
      <UsageChart data={q.data} />
      <Channels data={q.data} />
      <Section title={`${t('schoolsTitle')} (${f.n(s.schools)})`}>
        {schools.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('noSchools')}</p>
        ) : (
          <div className="-mx-1 overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="text-left text-muted-foreground">
                <tr>
                  <th className="px-1 py-1.5 font-medium">{t('school')}</th>
                  <th className="px-1 py-1.5 text-right font-medium">{t('colProfiles')}</th>
                  <th className="px-1 py-1.5 text-right font-medium">{t('colAccounts')}</th>
                  <th className="px-1 py-1.5 text-right font-medium max-sm:hidden">{t('colStudents')}</th>
                  <th className="px-1 py-1.5 text-right font-medium max-sm:hidden">{t('colStaff')}</th>
                </tr>
              </thead>
              <tbody className="tabular-nums">
                {schools.map((sc) => (
                  <tr key={sc.school} className="border-t">
                    <td className="px-1 py-2 font-medium">{sc.school}</td>
                    <td className="px-1 py-2 text-right">{f.n(sc.profiles)}</td>
                    <td className="px-1 py-2 text-right">{f.n(sc.accounts)}</td>
                    <td className="px-1 py-2 text-right max-sm:hidden">{f.n(sc.students)}</td>
                    <td className="px-1 py-2 text-right max-sm:hidden">{f.n(sc.staff)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>
    </>
  )
}

// ---- Users ----

function profileName(p: AdminProfile, fallback: string) {
  return p.accounts[0]?.name ?? fallback
}

function UsersTab() {
  const { t } = useI18n()
  const f = useFormat()
  const [query, setQuery] = useState('')
  const q = useDebounced(query, 250)
  const list = useQuery({ queryKey: ['admin', 'profiles', q], queryFn: () => api.admin.profiles(q), placeholderData: (prev) => prev })
  const [open, setOpen] = useState<string | null>(null)
  return (
    <>
      <label className="flex h-11 items-center gap-2 rounded-xl border border-input bg-card px-3 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50">
        <Search className="size-4 shrink-0 text-muted-foreground" />
        <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t('usersSearch')} aria-label={t('usersSearch')} className="h-full min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-muted-foreground" />
      </label>
      {list.data?.length === 0 && <p className="px-1 text-sm text-muted-foreground">{t('noUsers')}</p>}
      <ul className={cn('grid gap-2', list.isPlaceholderData && 'opacity-60')}>
        {list.data?.map((p) => (
          <li key={p.id}>
            <button
              type="button"
              onClick={() => setOpen(p.id)}
              className="grid w-full grid-cols-[auto_minmax(0,1fr)] items-center gap-3 rounded-xl border bg-card p-3 text-left transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
            >
              <Avatar name={profileName(p, '?')} className="size-10 text-sm" />
              <span className="grid min-w-0 gap-0.5">
                <span className="flex min-w-0 items-center gap-2">
                  <span className="truncate font-medium">{p.accounts.map((a) => a.name).join(', ') || t('noAccounts')}</span>
                  {p.admin && (
                    <Chip tone="honeydew">
                      <ShieldCheck />
                      {t('adminBadge')}
                    </Chip>
                  )}
                </span>
                <span className="truncate text-sm text-muted-foreground">
                  {[...new Set(p.accounts.map((a) => a.school))].join(', ') || p.id}
                  {p.login && ` · ${p.login.userId}`}
                  {' · '}
                  {p.lastSeenAt ? t('lastSeen', { date: f.day(p.lastSeenAt) }) : t('neverSeen')}
                </span>
              </span>
            </button>
          </li>
        ))}
      </ul>
      <ProfileDialog id={open} onClose={() => setOpen(null)} />
    </>
  )
}

function Row({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('flex flex-wrap items-center gap-x-3 gap-y-2 rounded-xl border p-3', className)}>{children}</div>
}

function ProfileDialog({ id, onClose }: { id: string | null; onClose: () => void }) {
  const { t, error, plural } = useI18n()
  const f = useFormat()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { swap } = useSession()
  const detail = useQuery({ queryKey: ['admin', 'profile', id], queryFn: () => api.admin.profile(id!), enabled: id !== null })
  const [confirm, setConfirm] = useState<'signout' | 'delete' | 'pin-remove' | null>(null)
  const [pinOpen, setPinOpen] = useState(false)
  const [revealed, setRevealed] = useState<Record<string, string>>({})

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['admin', 'profile', id] })
    qc.invalidateQueries({ queryKey: ['admin', 'profiles'] })
    qc.invalidateQueries({ queryKey: ['admin', 'overview'] })
    qc.invalidateQueries({ queryKey: ['admin', 'log'] })
  }
  const onError = (e: unknown) => toast.error(error(e))
  const after = (done?: string) => () => {
    setConfirm(null)
    refresh()
    if (done) toast.success(done)
  }
  const unlock = useMutation({ mutationFn: () => api.admin.unlock(id!), onSuccess: after(t('pinUnlocked')), onError })
  const clearPin = useMutation({ mutationFn: () => api.admin.clearPin(id!), onSuccess: after(), onError })
  const signOut = useMutation({ mutationFn: () => api.admin.signOut(id!), onSuccess: after(t('signedOutAll')), onError })
  const revoke = useMutation({ mutationFn: (keyId: string) => api.admin.deleteKey(keyId), onSuccess: after(), onError })
  const remove = useMutation({
    mutationFn: () => api.admin.deleteProfile(id!),
    onSuccess: () => {
      setConfirm(null)
      onClose()
      refresh()
    },
    onError,
  })
  const reveal = useMutation({
    mutationFn: (keyId: string) => api.admin.revealKey(keyId),
    onSuccess: (r, keyId) => {
      setRevealed((v) => ({ ...v, [keyId]: r.key }))
      qc.invalidateQueries({ queryKey: ['admin', 'log'] })
    },
    onError,
  })
  const impersonate = useMutation({
    mutationFn: () => api.admin.impersonate(id!),
    onSuccess: async (session) => {
      await swap(session)
      navigate('/')
    },
    onError,
  })

  const d = detail.data
  const p = d?.profile
  const name = p ? profileName(p, p.id) : ''
  const accountName = (ids: string[]) => ids.map((x) => p?.accounts.find((a) => a.id === x)?.name).filter(Boolean).join(', ') || '–'

  return (
    <Dialog open={id !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[92dvh] overflow-y-auto sm:max-w-lg">
        {!d || !p ? (
          <div className="grid gap-3 py-2">
            <DialogTitle className="sr-only">{t('tabUsers')}</DialogTitle>
            <Skeleton className="h-8 w-2/3" />
            <Skeleton className="h-24" />
            <Skeleton className="h-24" />
          </div>
        ) : (
          <div className="grid min-w-0 gap-4">
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <span className="truncate">{name}</span>
                {p.admin && (
                  <Chip tone="honeydew">
                    <ShieldCheck />
                    {t('adminBadge')}
                  </Chip>
                )}
              </DialogTitle>
              <DialogDescription>
                <span className="font-mono text-xs">{p.id}</span>
                <br />
                {t('profileSince', { date: f.day(p.createdAt) })} · {p.lastSeenAt ? t('lastSeen', { date: f.day(p.lastSeenAt) }) : t('neverSeen')} · {plural('devices', d.sessions)}
              </DialogDescription>
            </DialogHeader>

            <section className="grid gap-2">
              <h3 className="text-sm font-medium">{t('accounts')}</h3>
              {p.accounts.length === 0 && <p className="text-sm text-muted-foreground">{t('noAccounts')}</p>}
              {p.accounts.map((a) => (
                <Row key={a.id}>
                  <Avatar name={a.name} />
                  <span className="grid min-w-0 flex-1 leading-tight">
                    <span className="truncate font-medium">{a.name}</span>
                    <span className="truncate text-xs text-muted-foreground">
                      {a.school} · {t(a.role)} · <span className="font-mono">{a.userCode}</span>
                    </span>
                  </span>
                  {a.canViewOthers && <Chip tone="honeydew">{t('canViewOthers')}</Chip>}
                  {a.calendar && <Chip tone="cantaloupe">{t('calendarOnShort')}</Chip>}
                </Row>
              ))}
              <p className="text-xs text-muted-foreground">{t('namesCount', { subjects: d.names.subjects, teachers: d.names.teachers })}</p>
            </section>

            <section className="grid gap-2">
              <h3 className="text-sm font-medium">{t('loginTitle')}</h3>
              <Row>
                <KeyRound className="size-5 shrink-0 text-muted-foreground" />
                <span className="grid min-w-0 flex-1 gap-0.5 text-sm">
                  {p.login ? <span className="font-medium">{t('pinFor', { number: p.login.userId })}</span> : <span className="text-muted-foreground">{t('pinNone')}</span>}
                  {d.pinLock?.lockedUntil && <span className="text-watermelon-ink">{t('pinLockedUntil', { time: f.when(d.pinLock.lockedUntil) })}</span>}
                  {!!d.pinLock?.fails && !d.pinLock.lockedUntil && <span className="text-muted-foreground">{t('pinFails', { n: d.pinLock.fails })}</span>}
                </span>
              </Row>
              <div className="flex flex-wrap gap-2">
                {p.accounts.length > 0 && (
                  <Button variant="outline" className="h-10" onClick={() => setPinOpen(true)}>
                    <KeyRound />
                    {p.login ? t('pinChange') : t('pinSet')}
                  </Button>
                )}
                {!!d.pinLock?.fails && (
                  <Button variant="outline" className="h-10" disabled={unlock.isPending} onClick={() => unlock.mutate()}>
                    <LockOpen />
                    {t('pinUnlock')}
                  </Button>
                )}
                {p.login && (
                  <Button variant="ghost" className="h-10 text-muted-foreground" onClick={() => setConfirm('pin-remove')}>
                    {t('loginRemove')}
                  </Button>
                )}
              </div>
            </section>

            <section className="grid gap-2">
              <h3 className="text-sm font-medium">{t('keysTitle')}</h3>
              {d.keys.length === 0 && <p className="text-sm text-muted-foreground">{t('keysNone')}</p>}
              {d.keys.map((k) => (
                <div key={k.id} className="grid gap-2 rounded-xl border p-3">
                  <div className="flex items-center gap-3">
                    <span className="grid min-w-0 flex-1 gap-0.5">
                      <span className="truncate font-medium">{k.name}</span>
                      <span className="truncate font-mono text-xs text-muted-foreground">{k.hint}</span>
                      <span className="truncate text-xs text-muted-foreground">
                        {accountName(k.accountIds)} · {k.lastUsedAt ? t('lastUsed', { date: f.day(k.lastUsedAt) }) : t('neverUsed')}
                      </span>
                    </span>
                    {k.stored && !revealed[k.id] && (
                      <Button variant="outline" className="h-9 shrink-0" disabled={reveal.isPending} onClick={() => reveal.mutate(k.id)}>
                        <Eye />
                        <span className="max-sm:sr-only">{t('keyReveal')}</span>
                      </Button>
                    )}
                    <Button variant="destructive" className="h-9 shrink-0" disabled={revoke.isPending} onClick={() => revoke.mutate(k.id)}>
                      {t('revoke')}
                    </Button>
                  </div>
                  {!k.stored && <p className="text-xs text-muted-foreground">{t('keyNotStored')}</p>}
                  {revealed[k.id] && (
                    <>
                      <CopyField value={revealed[k.id]} />
                      <p className="text-xs text-muted-foreground">{t('keyRevealNote')}</p>
                    </>
                  )}
                </div>
              ))}
            </section>

            {d.grants.length > 0 && (
              <section className="grid gap-2">
                <h3 className="text-sm font-medium">{t('appsTitle')}</h3>
                {d.grants.map((g) => (
                  <Row key={g.id}>
                    <span className="grid min-w-0 flex-1 gap-0.5">
                      <span className="truncate font-medium">{g.app}</span>
                      <span className="truncate text-xs text-muted-foreground">{accountName(g.accountIds)}</span>
                    </span>
                  </Row>
                ))}
              </section>
            )}

            <DialogFooter className="flex-wrap gap-2 sm:justify-start">
              <Button className="h-11" disabled={impersonate.isPending || p.accounts.length === 0} onClick={() => impersonate.mutate()}>
                {impersonate.isPending ? <Loader2 className="animate-spin" /> : <Eye />}
                {t('impersonate')}
              </Button>
              <Button variant="outline" className="h-11" disabled={d.sessions === 0} onClick={() => setConfirm('signout')}>
                <LogOut />
                {t('signOutAll')}
              </Button>
              <Button variant="destructive" className="h-11" onClick={() => setConfirm('delete')}>
                <Trash2 />
                {t('deleteProfile')}
              </Button>
            </DialogFooter>

            <AdminPinDialog key={`${p.id}:${pinOpen}`} open={pinOpen} onOpenChange={setPinOpen} profile={p} onSaved={refresh} />
            <ConfirmDialog open={confirm === 'signout'} onOpenChange={(o) => !o && setConfirm(null)} title={t('signOutAll')} text={t('signOutAllText')} confirmLabel={t('signOutAll')} busy={signOut.isPending} onConfirm={() => signOut.mutate()} />
            <ConfirmDialog open={confirm === 'pin-remove'} onOpenChange={(o) => !o && setConfirm(null)} title={t('loginRemoveTitle')} text={t('loginRemoveText')} confirmLabel={t('loginRemove')} busy={clearPin.isPending} onConfirm={() => clearPin.mutate()} />
            <ConfirmDialog open={confirm === 'delete'} onOpenChange={(o) => !o && setConfirm(null)} title={`${t('deleteProfile')}: ${name}`} text={t('deleteProfileText')} confirmLabel={t('deleteProfile')} busy={remove.isPending} onConfirm={() => remove.mutate()} />
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

function AdminPinDialog({ open, onOpenChange, profile, onSaved }: { open: boolean; onOpenChange: (o: boolean) => void; profile: AdminProfile; onSaved: () => void }) {
  const { t, error } = useI18n()
  const current = profile.login && profile.accounts.find((a) => a.userCode === profile.login!.userId && a.school === profile.login!.school)
  const [accountId, setAccountId] = useState((current || profile.accounts[0])?.id ?? '')
  const [pin, setPin] = useState('')
  const save = useMutation({
    mutationFn: () => api.admin.setPin(profile.id, accountId, pin),
    onSuccess: (l) => {
      onOpenChange(false)
      onSaved()
      toast.success(t('pinSaved', { number: l.userId }))
    },
  })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (pin.length === 4 && !save.isPending) save.mutate()
          }}
        >
          <DialogHeader>
            <DialogTitle>{profile.login ? t('pinChange') : t('pinSet')}</DialogTitle>
            <DialogDescription>{t('adminPinText')}</DialogDescription>
          </DialogHeader>
          {profile.accounts.length > 1 && (
            <fieldset className="grid gap-1">
              <legend className="mb-1.5 text-sm font-medium">{t('loginAccount')}</legend>
              {profile.accounts.map((a) => (
                <Label key={a.id} className={cn('flex min-h-11 items-center gap-3 rounded-lg border px-3 font-normal has-focus-visible:ring-3 has-focus-visible:ring-ring/50', a.id === accountId && 'border-honeydew-ink/40 bg-honeydew-tint font-medium text-honeydew-ink')}>
                  <input type="radio" name="admin-pin-account" className="sr-only" checked={a.id === accountId} onChange={() => setAccountId(a.id)} />
                  <span className="truncate">{a.name}</span>
                  <span className={cn('ml-auto shrink-0 font-mono text-sm', a.id !== accountId && 'text-muted-foreground')}>{a.userCode}</span>
                </Label>
              ))}
            </fieldset>
          )}
          <div className="grid gap-1.5">
            <Label htmlFor="admin-pin">{t('pin')}</Label>
            <PinInput id="admin-pin" value={pin} onChange={setPin} autoComplete="off" autoFocus aria-invalid={save.isError} />
            {save.isError && <p className="text-sm text-destructive">{error(save.error)}</p>}
          </div>
          <DialogFooter>
            <Button type="submit" className="h-11" disabled={pin.length !== 4 || save.isPending}>
              {save.isPending && <Loader2 className="animate-spin" />}
              {t('save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// ---- Names ----

function SuggestionsSection() {
  const { t, plural, error } = useI18n()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['admin', 'suggestions'], queryFn: api.admin.suggestions })
  const subjects = useQuery({ queryKey: ['admin', 'subjects'], queryFn: api.admin.subjects })
  const adopt = useMutation({
    mutationFn: (s: Suggestion & { kind: 'subject' | 'teacher' }) => {
      if (s.kind === 'teacher') return api.admin.setTeacher(s.school!, s.code, s.name)
      // Keep the English name the subject already has; users name things in Dutch.
      const cur = subjects.data?.find((x) => x.code === s.code)
      return api.admin.setSubject(s.code, s.name, cur?.global ? cur.en : '')
    },
    onSuccess: () => {
      toast.success(t('adopted'))
      qc.invalidateQueries({ queryKey: ['admin'] })
    },
    onError: (e) => toast.error(error(e)),
  })
  const all = [...(q.data?.teachers ?? []).map((s) => ({ ...s, kind: 'teacher' as const })), ...(q.data?.subjects ?? []).map((s) => ({ ...s, kind: 'subject' as const }))].sort((a, b) => b.profiles - a.profiles)
  return (
    <Section title={t('suggestionsTitle')} text={t('suggestionsText')}>
      {q.data && all.length === 0 && <p className="text-sm text-muted-foreground">{t('noSuggestions')}</p>}
      <ul className="grid gap-2">
        {all.slice(0, 50).map((s) => (
          <li key={`${s.kind}:${s.school ?? ''}:${s.code}:${s.name}`} className="flex items-center gap-3 rounded-xl border p-3">
            <span className="w-14 shrink-0 truncate rounded-md bg-muted px-2 py-1 text-center font-mono text-xs">{s.code}</span>
            <span className="grid min-w-0 flex-1 gap-0.5">
              <span className="truncate font-medium">{s.name}</span>
              <span className="truncate text-xs text-muted-foreground">
                {s.kind === 'teacher' ? `${t('teacher')} · ${s.school}` : t('subjectWord')} · {plural('suggestionBy', s.profiles)}
                {s.current && ` · ${t('suggestionNow', { name: s.current })}`}
              </span>
            </span>
            <Button variant="outline" className="h-9 shrink-0" disabled={adopt.isPending} onClick={() => adopt.mutate(s)}>
              <Check />
              <span className="max-sm:sr-only">{t('adopt')}</span>
            </Button>
          </li>
        ))}
      </ul>
    </Section>
  )
}

function SubjectRow({ s }: { s: AdminSubject }) {
  const { t, error } = useI18n()
  const qc = useQueryClient()
  const [nl, setNl] = useState(s.nl)
  const [en, setEn] = useState(s.global ? s.en : '')
  const done = () => qc.invalidateQueries({ queryKey: ['admin'] })
  const save = useMutation({ mutationFn: () => api.admin.setSubject(s.code, nl.trim(), en.trim()), onSuccess: done, onError: (e) => toast.error(error(e)) })
  const reset = useMutation({ mutationFn: () => api.admin.resetSubject(s.code), onSuccess: done, onError: (e) => toast.error(error(e)) })
  const dirty = nl.trim() !== '' && (nl.trim() !== s.nl || en.trim() !== (s.global ? s.en : ''))
  return (
    <li>
      <form
        className="grid grid-cols-[3.5rem_minmax(0,1fr)_auto] items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          if (dirty) save.mutate()
        }}
      >
        <span className="truncate rounded-md bg-muted px-2 py-1 text-center font-mono text-xs">{s.code}</span>
        <div className="grid min-w-0 grid-cols-1 gap-1.5 sm:grid-cols-2">
          <Input value={nl} onChange={(e) => setNl(e.target.value)} aria-label={`${t('subjectNl')}: ${s.code}`} maxLength={80} className={cn('h-10 text-base', s.global && 'border-honeydew')} />
          <Input value={en} onChange={(e) => setEn(e.target.value)} placeholder={s.builtinEn || t('subjectEn')} aria-label={`${t('subjectEn')}: ${s.code}`} maxLength={80} className={cn('h-10 text-base', s.global && en && 'border-honeydew')} />
        </div>
        {dirty ? (
          <Button type="submit" className="h-10" disabled={save.isPending}>
            {t('save')}
          </Button>
        ) : s.global ? (
          <Button type="button" variant="outline" className="h-10" disabled={reset.isPending} onClick={() => reset.mutate()} aria-label={s.builtinNl ? t('builtin', { name: s.builtinNl }) : t('remove')} title={s.builtinNl ? t('builtin', { name: s.builtinNl }) : undefined}>
            {s.builtinNl ? <RotateCcw /> : <Trash2 />}
          </Button>
        ) : (
          <span className="w-10" />
        )}
      </form>
    </li>
  )
}

function GlobalSubjectsSection() {
  const { t, error } = useI18n()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['admin', 'subjects'], queryFn: api.admin.subjects })
  const [filter, setFilter] = useState('')
  const [code, setCode] = useState('')
  const [nl, setNl] = useState('')
  const add = useMutation({
    mutationFn: () => api.admin.setSubject(code.trim(), nl.trim(), ''),
    onSuccess: () => {
      setCode('')
      setNl('')
      qc.invalidateQueries({ queryKey: ['admin'] })
    },
    onError: (e) => toast.error(error(e)),
  })
  const shown = useMemo(() => {
    const fl = filter.trim().toLowerCase()
    const list = (q.data ?? []).filter((s) => !fl || s.code.includes(fl) || s.nl.toLowerCase().includes(fl) || s.en.toLowerCase().includes(fl))
    return [...list.filter((s) => s.global), ...list.filter((s) => !s.global)]
  }, [q.data, filter])
  return (
    <Section title={t('globalSubjectsTitle')} text={t('globalSubjectsText')}>
      <form
        className="flex items-end gap-2 rounded-xl border border-dashed p-3"
        onSubmit={(e) => {
          e.preventDefault()
          if (code.trim() && nl.trim()) add.mutate()
        }}
      >
        <div className="grid w-24 shrink-0 gap-1.5">
          <Label htmlFor="global-code">{t('subjectCode')}</Label>
          <Input id="global-code" value={code} onChange={(e) => setCode(e.target.value)} placeholder="wi" maxLength={32} autoCapitalize="none" className="h-10 font-mono text-base" />
        </div>
        <div className="grid min-w-0 flex-1 gap-1.5">
          <Label htmlFor="global-nl">{t('subjectNl')}</Label>
          <Input id="global-nl" value={nl} onChange={(e) => setNl(e.target.value)} placeholder="Wiskunde" maxLength={80} className="h-10 text-base" />
        </div>
        <Button type="submit" className="h-10 shrink-0" disabled={!code.trim() || !nl.trim() || add.isPending} aria-label={t('subjectAdd')}>
          <Plus />
        </Button>
      </form>
      <label className="flex h-11 items-center gap-2 rounded-xl border border-input px-3 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50">
        <Search className="size-4 shrink-0 text-muted-foreground" />
        <input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t('subjectsFilter')} aria-label={t('subjectsFilter')} className="h-full min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-muted-foreground" />
      </label>
      <ul className="grid gap-2">
        {shown.map((s) => (
          <SubjectRow key={`${s.code}:${s.nl}:${s.en}:${s.global}`} s={s} />
        ))}
      </ul>
    </Section>
  )
}

function TeacherRow({ school, code, name }: { school: string; code: string; name: string }) {
  const { t, error } = useI18n()
  const qc = useQueryClient()
  const [value, setValue] = useState(name)
  const done = () => qc.invalidateQueries({ queryKey: ['admin'] })
  const save = useMutation({ mutationFn: () => api.admin.setTeacher(school, code, value.trim()), onSuccess: done, onError: (e) => toast.error(error(e)) })
  const reset = useMutation({ mutationFn: () => api.admin.resetTeacher(school, code), onSuccess: done, onError: (e) => toast.error(error(e)) })
  const dirty = value.trim() !== '' && value.trim() !== name
  return (
    <li>
      <form
        className="flex items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          if (dirty) save.mutate()
        }}
      >
        <span className="w-14 shrink-0 truncate rounded-md bg-muted px-2 py-1 text-center font-mono text-xs">{code}</span>
        <Input value={value} onChange={(e) => setValue(e.target.value)} aria-label={`${t('teacherName')}: ${code}`} maxLength={80} className="h-10 min-w-0 flex-1 border-honeydew text-base" />
        {dirty ? (
          <Button type="submit" className="h-10 shrink-0" disabled={save.isPending}>
            {t('save')}
          </Button>
        ) : (
          <Button type="button" variant="outline" className="h-10 shrink-0" disabled={reset.isPending} onClick={() => reset.mutate()}>
            {t('remove')}
          </Button>
        )}
      </form>
    </li>
  )
}

function SchoolTeachersSection() {
  const { t, error } = useI18n()
  const qc = useQueryClient()
  const overview = useQuery({ queryKey: ['admin', 'overview'], queryFn: api.admin.overview })
  const schools = overview.data?.schools.map((s) => s.school) ?? []
  const [picked, setPicked] = useState<string | null>(null)
  const school = picked ?? schools[0] ?? ''
  const q = useQuery({ queryKey: ['admin', 'teachers', school], queryFn: () => api.admin.teachers(school), enabled: school !== '' })
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const add = useMutation({
    mutationFn: () => api.admin.setTeacher(school, code.trim(), name.trim()),
    onSuccess: () => {
      setCode('')
      setName('')
      qc.invalidateQueries({ queryKey: ['admin'] })
    },
    onError: (e) => toast.error(error(e)),
  })
  return (
    <Section title={t('schoolTeachersTitle')} text={t('schoolTeachersText')}>
      {schools.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('noSchools')}</p>
      ) : (
        <>
          <div role="radiogroup" aria-label={t('school')} className="flex flex-wrap gap-1.5">
            {schools.map((s) => (
              <button
                key={s}
                type="button"
                role="radio"
                aria-checked={s === school}
                onClick={() => setPicked(s)}
                className={cn('h-9 rounded-full border px-3.5 text-sm font-medium transition-colors focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none', s === school ? 'border-primary bg-primary text-primary-foreground' : 'hover:bg-muted')}
              >
                {s}
              </button>
            ))}
          </div>
          <form
            className="flex items-end gap-2 rounded-xl border border-dashed p-3"
            onSubmit={(e) => {
              e.preventDefault()
              if (code.trim() && name.trim()) add.mutate()
            }}
          >
            <div className="grid w-24 shrink-0 gap-1.5">
              <Label htmlFor="school-teacher-code">{t('subjectCode')}</Label>
              <Input id="school-teacher-code" value={code} onChange={(e) => setCode(e.target.value)} placeholder="dkr" maxLength={32} autoCapitalize="none" className="h-10 font-mono text-base" />
            </div>
            <div className="grid min-w-0 flex-1 gap-1.5">
              <Label htmlFor="school-teacher-name">{t('keyName')}</Label>
              <Input id="school-teacher-name" value={name} onChange={(e) => setName(e.target.value)} placeholder={t('teacherNamePlaceholder')} maxLength={80} className="h-10 text-base" />
            </div>
            <Button type="submit" className="h-10 shrink-0" disabled={!code.trim() || !name.trim() || add.isPending} aria-label={t('save')}>
              <Plus />
            </Button>
          </form>
          {q.data?.length === 0 && <p className="text-sm text-muted-foreground">{t('noSchoolTeachers')}</p>}
          <ul className="grid gap-2">
            {q.data?.map((x) => (
              <TeacherRow key={`${school}:${x.code}:${x.name}`} school={school} code={x.code} name={x.name} />
            ))}
          </ul>
        </>
      )}
    </Section>
  )
}

// ---- Log ----

function LogTab() {
  const { t } = useI18n()
  const f = useFormat()
  const q = useQuery({ queryKey: ['admin', 'log'], queryFn: api.admin.log })
  if (q.data?.length === 0) return <p className="px-1 text-sm text-muted-foreground">{t('logEmpty')}</p>
  return (
    <ul className="grid gap-2">
      {q.data?.map((e) => (
        <li key={e.id} className="grid gap-0.5 rounded-xl border bg-card p-3 text-sm">
          <span className="text-xs text-muted-foreground">{f.when(e.at)}</span>
          <span className="min-w-0 break-words">
            <span className="font-medium">{e.adminName || e.admin}</span> {t(`log_${e.action}` as Key)} <span className="font-mono text-xs">{e.target}</span>
            {e.detail && <span className="text-muted-foreground"> · {e.detail}</span>}
          </span>
        </li>
      ))}
    </ul>
  )
}

export default function Admin() {
  const { t } = useI18n()
  const { session } = useSession()
  const [tab, setTab] = useState('overview')
  if (!session.admin) return null
  return (
    <div className="mx-auto min-h-dvh w-full max-w-3xl px-3 pt-[max(0.5rem,env(safe-area-inset-top))] pb-[max(2rem,env(safe-area-inset-bottom))] md:px-6">
      <header className="flex h-14 items-center gap-1">
        <Button variant="ghost" size="icon" className="size-11" aria-label={t('back')} render={<Link to="/settings" />}>
          <ArrowLeft className="size-5" />
        </Button>
        <h1 className="text-xl font-semibold">{t('admin')}</h1>
      </header>
      <Tabs value={tab} onValueChange={(v) => setTab(String(v))} className="gap-4">
        <TabsList className="h-11 w-full">
          <TabsTrigger value="overview">{t('tabOverview')}</TabsTrigger>
          <TabsTrigger value="users">{t('tabUsers')}</TabsTrigger>
          <TabsTrigger value="names">{t('tabNames')}</TabsTrigger>
          <TabsTrigger value="log">{t('tabLog')}</TabsTrigger>
        </TabsList>
        <TabsContent value="overview" className="grid gap-4">
          <OverviewTab />
        </TabsContent>
        <TabsContent value="users" className="grid gap-3">
          <UsersTab />
        </TabsContent>
        <TabsContent value="names" className="grid gap-4">
          <SuggestionsSection />
          <SchoolTeachersSection />
          <GlobalSubjectsSection />
        </TabsContent>
        <TabsContent value="log" className="grid gap-2">
          <LogTab />
        </TabsContent>
      </Tabs>
    </div>
  )
}
