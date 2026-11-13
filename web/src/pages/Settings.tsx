import { useMemo, useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import { useTheme } from 'next-themes'
import { ArrowLeft, BookOpen, CalendarPlus, Eye, KeyRound, Loader2, Monitor, Moon, Plus, RotateCcw, Search, Sun, Trash2, Unplug } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Avatar, Chip, ConfirmDialog, CopyField } from '@/components/bits'
import { LinkDialog } from '@/components/LinkForm'
import { api, type Account, type ApiKey, type Grant } from '@/lib/api'
import { useI18n, type Lang } from '@/lib/i18n'
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
      <CardContent className="grid gap-3">{children}</CardContent>
    </Card>
  )
}

/** A segmented choice: every option visible, the current one filled. */
function Choice<T extends string>({ value, onChange, options, label }: { value: T; onChange: (v: T) => void; options: { value: T; label: string; icon?: ReactNode }[]; label: string }) {
  return (
    <div role="radiogroup" aria-label={label} className="grid auto-cols-fr grid-flow-col gap-1 rounded-xl bg-muted p-1">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={o.value === value}
          onClick={() => onChange(o.value)}
          className={cn(
            'flex h-10 items-center justify-center gap-2 rounded-lg text-sm font-medium transition-colors focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none [&>svg]:size-4',
            o.value === value ? 'bg-primary text-primary-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground',
          )}
        >
          {o.icon}
          {o.label}
        </button>
      ))}
    </div>
  )
}

function AccountPicker({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  const { accounts } = useSession()
  const { t } = useI18n()
  if (accounts.length < 2) return null
  return <Choice label={t('accounts')} value={value} onChange={onChange} options={accounts.map((a) => ({ value: a.id, label: a.name.split(' ')[0] }))} />
}

const accountNames = (ids: string[], accounts: Account[]) => ids.map((id) => accounts.find((a) => a.id === id)?.name).filter(Boolean).join(', ')

// ---- Accounts ----

function AccountsTab() {
  const { t } = useI18n()
  const { accounts, reload } = useSession()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [linking, setLinking] = useState(false)
  const [unlinking, setUnlinking] = useState<Account | null>(null)
  const [forgetting, setForgetting] = useState(false)
  const unlink = useMutation({
    mutationFn: (a: Account) => api.unlink(a.id),
    onSuccess: async () => {
      setUnlinking(null)
      qc.invalidateQueries({ queryKey: ['keys'] })
      qc.invalidateQueries({ queryKey: ['grants'] })
      await reload()
    },
  })
  const forget = useMutation({
    mutationFn: api.forget,
    onSuccess: async () => {
      qc.clear()
      await reload()
      navigate('/')
    },
  })
  return (
    <>
      <Section title={t('accounts')}>
        <ul className="grid gap-2">
          {accounts.map((a) => (
            <li key={a.id} className="flex items-center gap-3 rounded-xl border p-3">
              <Avatar name={a.name} className="size-10 text-sm" />
              <div className="grid min-w-0 flex-1 gap-0.5">
                <span className="truncate font-medium">{a.name}</span>
                <span className="truncate text-sm text-muted-foreground">
                  {a.school}.zportal.nl · {t(a.role)}
                </span>
                {a.canViewOthers && (
                  <Chip tone="honeydew" className="mt-1 w-fit">
                    <Eye />
                    {t('canViewOthers')}
                  </Chip>
                )}
              </div>
              <Button variant="destructive" className="h-10 shrink-0" onClick={() => setUnlinking(a)}>
                <Unplug />
                <span className="hidden sm:inline">{t('unlink')}</span>
                <span className="sr-only sm:hidden">{t('unlink')}</span>
              </Button>
            </li>
          ))}
        </ul>
        <Button variant="outline" className="h-11" onClick={() => setLinking(true)}>
          <Plus />
          {t('linkAnother')}
        </Button>
      </Section>

      <Section title={t('forgetTitle')} text={t('forgetText')}>
        <Button variant="destructive" className="h-11 w-fit" onClick={() => setForgetting(true)}>
          <Trash2 />
          {t('forgetTitle')}
        </Button>
      </Section>

      <LinkDialog open={linking} onOpenChange={setLinking} />
      <ConfirmDialog
        open={unlinking !== null}
        onOpenChange={(o) => !o && setUnlinking(null)}
        title={t('unlinkTitle', { name: unlinking?.name ?? '' })}
        text={t('unlinkText')}
        confirmLabel={t('unlink')}
        busy={unlink.isPending}
        onConfirm={() => unlinking && unlink.mutate(unlinking)}
      />
      <ConfirmDialog open={forgetting} onOpenChange={setForgetting} title={t('forgetConfirmTitle')} text={t('forgetText')} confirmLabel={t('forgetConfirm')} busy={forget.isPending} onConfirm={() => forget.mutate()} />
    </>
  )
}

// ---- Display ----

function LookTab() {
  const { t, lang, setLang } = useI18n()
  const { theme, setTheme } = useTheme()
  return (
    <>
      <Section title={t('language')}>
        <Choice<Lang> label={t('language')} value={lang} onChange={setLang} options={[{ value: 'nl', label: 'Nederlands' }, { value: 'en', label: 'English' }]} />
      </Section>
      <Section title={t('theme')}>
        <Choice
          label={t('theme')}
          value={theme ?? 'system'}
          onChange={setTheme}
          options={[
            { value: 'system', label: t('themeSystem'), icon: <Monitor /> },
            { value: 'light', label: t('themeLight'), icon: <Sun /> },
            { value: 'dark', label: t('themeDark'), icon: <Moon /> },
          ]}
        />
      </Section>
    </>
  )
}

// ---- Connections: calendar, API keys, MCP, connected apps ----

function CalendarSection() {
  const { t } = useI18n()
  const { active } = useSession()
  const qc = useQueryClient()
  const [accountId, setAccountId] = useState(active!.id)
  const cal = useQuery({ queryKey: ['calendar', accountId], queryFn: () => api.calendar(accountId) })
  const set = useMutation({
    mutationFn: (on: boolean) => (on ? api.enableCalendar(accountId) : api.disableCalendar(accountId)),
    onSuccess: (data) => qc.setQueryData(['calendar', accountId], data),
  })
  const enabled = cal.data?.enabled ?? false
  return (
    <Section title={t('calendarTitle')} text={t('calendarText')}>
      <AccountPicker value={accountId} onChange={setAccountId} />
      <Label className="flex min-h-11 items-center justify-between gap-3 text-sm font-normal">
        {t('calendarOn')}
        <Switch checked={enabled} disabled={cal.isPending || set.isPending} onCheckedChange={(on) => set.mutate(on)} />
      </Label>
      {enabled && cal.data?.url && (
        <>
          <CopyField value={cal.data.url} />
          <p className="text-sm text-muted-foreground">{t('calendarSecret')}</p>
          <div className="flex flex-wrap gap-2">
            <Button className="h-11" render={<a href={cal.data.url.replace(/^https?:/, 'webcal:')} />}>
              <CalendarPlus />
              {t('calendarOpen')}
            </Button>
            <Button variant="outline" className="h-11" disabled={set.isPending} onClick={() => set.mutate(true, { onSuccess: () => toast.success(t('calendarNewDone')) })}>
              <RotateCcw />
              {t('calendarNew')}
            </Button>
          </div>
        </>
      )}
    </Section>
  )
}

function KeysSection() {
  const { t, locale } = useI18n()
  const { accounts } = useSession()
  const qc = useQueryClient()
  const keys = useQuery({ queryKey: ['keys'], queryFn: api.keys })
  const [creating, setCreating] = useState(false)
  const [name, setName] = useState('')
  const [picked, setPicked] = useState<string[]>([])
  const [created, setCreated] = useState<ApiKey | null>(null)
  const [revoking, setRevoking] = useState<ApiKey | null>(null)

  const open = () => {
    setName('')
    setPicked(accounts.length === 1 ? [accounts[0].id] : [])
    setCreated(null)
    setCreating(true)
  }
  const create = useMutation({
    mutationFn: () => api.createKey(name, picked),
    onSuccess: (key) => {
      setCreated(key)
      qc.invalidateQueries({ queryKey: ['keys'] })
    },
  })
  const revoke = useMutation({
    mutationFn: (k: ApiKey) => api.deleteKey(k.id),
    onSuccess: () => {
      setRevoking(null)
      qc.invalidateQueries({ queryKey: ['keys'] })
    },
  })
  const date = (iso: string) => new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short' }).format(new Date(iso))

  return (
    <Section
      title={t('keysTitle')}
      text={t('keysText')}
      action={
        <Button variant="ghost" className="-mt-1 h-9 shrink-0" render={<Link to="/docs" />}>
          <BookOpen />
          <span className="hidden sm:inline">{t('keysDocs')}</span>
          <span className="sm:hidden">Docs</span>
        </Button>
      }
    >
      {keys.data?.length === 0 && <p className="text-sm text-muted-foreground">{t('noKeys')}</p>}
      <ul className="grid gap-2">
        {keys.data?.map((k) => (
          <li key={k.id} className="flex items-center gap-3 rounded-xl border p-3">
            <KeyRound className="size-5 shrink-0 text-muted-foreground" />
            <div className="grid min-w-0 flex-1 gap-0.5">
              <span className="truncate font-medium">{k.name}</span>
              <span className="truncate font-mono text-xs text-muted-foreground">{k.hint}</span>
              <span className="truncate text-sm text-muted-foreground">
                {accountNames(k.accountIds, accounts) || '–'} · {k.lastUsedAt ? t('lastUsed', { date: date(k.lastUsedAt) }) : t('neverUsed')}
              </span>
            </div>
            <Button variant="destructive" className="h-10 shrink-0" onClick={() => setRevoking(k)}>
              {t('revoke')}
            </Button>
          </li>
        ))}
      </ul>
      <Button variant="outline" className="h-11" onClick={open}>
        <Plus />
        {t('keyNew')}
      </Button>

      <Dialog open={creating} onOpenChange={setCreating}>
        <DialogContent className="sm:max-w-md">
          {created ? (
            <>
              <DialogHeader>
                <DialogTitle>{t('keyCreatedTitle')}</DialogTitle>
                <DialogDescription>{t('keyCreatedText')}</DialogDescription>
              </DialogHeader>
              <CopyField value={created.key ?? ''} />
              <DialogFooter>
                <Button className="h-11" onClick={() => setCreating(false)}>
                  {t('done')}
                </Button>
              </DialogFooter>
            </>
          ) : (
            <form
              className="grid gap-4"
              onSubmit={(e) => {
                e.preventDefault()
                create.mutate()
              }}
            >
              <DialogHeader>
                <DialogTitle>{t('keyNew')}</DialogTitle>
              </DialogHeader>
              <div className="grid gap-1.5">
                <Label htmlFor="key-name">{t('keyName')}</Label>
                <Input id="key-name" value={name} onChange={(e) => setName(e.target.value)} placeholder={t('keyNamePlaceholder')} maxLength={60} className="h-11 text-base" autoComplete="off" />
              </div>
              <fieldset className="grid gap-1">
                <legend className="mb-1.5 text-sm font-medium">{t('keyAccounts')}</legend>
                {accounts.map((a) => (
                  <Label key={a.id} className="flex min-h-11 items-center gap-3 rounded-lg border px-3 font-normal">
                    <Checkbox checked={picked.includes(a.id)} onCheckedChange={(on) => setPicked((p) => (on ? [...p, a.id] : p.filter((id) => id !== a.id)))} />
                    <span className="truncate">{a.name}</span>
                    <span className="ml-auto truncate text-xs text-muted-foreground">{a.school}</span>
                  </Label>
                ))}
              </fieldset>
              <DialogFooter>
                <Button type="submit" className="h-11" disabled={!name.trim() || picked.length === 0 || create.isPending}>
                  {create.isPending && <Loader2 className="animate-spin" />}
                  {t('keyCreate')}
                </Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(o) => !o && setRevoking(null)}
        title={t('revokeKeyTitle', { name: revoking?.name ?? '' })}
        text={t('revokeKeyText')}
        confirmLabel={t('revoke')}
        busy={revoke.isPending}
        onConfirm={() => revoking && revoke.mutate(revoking)}
      />
    </Section>
  )
}

function AppsSection() {
  const { t } = useI18n()
  const { accounts } = useSession()
  const qc = useQueryClient()
  const grants = useQuery({ queryKey: ['grants'], queryFn: api.grants })
  const [revoking, setRevoking] = useState<Grant | null>(null)
  const revoke = useMutation({
    mutationFn: (g: Grant) => api.deleteGrant(g.id),
    onSuccess: () => {
      setRevoking(null)
      qc.invalidateQueries({ queryKey: ['grants'] })
    },
  })
  return (
    <Section title={t('mcpTitle')} text={t('mcpText')}>
      <CopyField value={`${location.origin}/mcp`} />
      <h3 className="mt-2 text-sm font-medium">{t('appsTitle')}</h3>
      {grants.data?.length === 0 && <p className="text-sm text-muted-foreground">{t('noApps')}</p>}
      <ul className="grid gap-2">
        {grants.data?.map((g) => (
          <li key={g.id} className="flex items-center gap-3 rounded-xl border p-3">
            <div className="grid min-w-0 flex-1 gap-0.5">
              <span className="truncate font-medium">{g.app}</span>
              <span className="truncate text-sm text-muted-foreground">{accountNames(g.accountIds, accounts) || '–'}</span>
            </div>
            <Button variant="destructive" className="h-10 shrink-0" onClick={() => setRevoking(g)}>
              {t('revoke')}
            </Button>
          </li>
        ))}
      </ul>
      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(o) => !o && setRevoking(null)}
        title={t('revokeAppTitle', { name: revoking?.app ?? '' })}
        text={t('revokeAppText')}
        confirmLabel={t('revoke')}
        busy={revoke.isPending}
        onConfirm={() => revoking && revoke.mutate(revoking)}
      />
    </Section>
  )
}

// ---- Subjects ----

function NameRow({ accountId, kind, code, name, custom }: { accountId: string; kind: 'subject' | 'teacher'; code: string; name: string; custom: boolean }) {
  const { t } = useI18n()
  const qc = useQueryClient()
  const [value, setValue] = useState(name)
  const refresh = () => {
    qc.invalidateQueries({ queryKey: [kind === 'subject' ? 'subjects' : 'teachers', accountId] })
    qc.invalidateQueries({ queryKey: ['schedule', accountId] })
  }
  const save = useMutation({ mutationFn: () => (kind === 'subject' ? api.setSubject(accountId, code, value) : api.setTeacher(accountId, code, value)), onSuccess: refresh })
  const reset = useMutation({ mutationFn: () => (kind === 'subject' ? api.resetSubject(accountId, code) : api.resetTeacher(accountId, code)), onSuccess: refresh })
  const dirty = value.trim() !== name && value.trim() !== ''
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
        <Input value={value} onChange={(e) => setValue(e.target.value)} aria-label={`${t(kind === 'subject' ? 'subjectName' : 'teacherName')}: ${code}`} maxLength={80} className={cn('h-10 min-w-0 flex-1 text-base', custom && 'border-honeydew')} />
        {dirty ? (
          <Button type="submit" className="h-10 shrink-0" disabled={save.isPending}>
            {t('save')}
          </Button>
        ) : (
          custom && (
            <Button type="button" variant="outline" className="h-10 shrink-0" disabled={reset.isPending} onClick={() => reset.mutate()}>
              {t(kind === 'subject' ? 'reset' : 'remove')}
            </Button>
          )
        )}
      </form>
    </li>
  )
}

function SubjectsSection({ accountId }: { accountId: string }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const [filter, setFilter] = useState('')
  const [newCode, setNewCode] = useState('')
  const [newName, setNewName] = useState('')
  const subjects = useQuery({ queryKey: ['subjects', accountId, lang], queryFn: () => api.subjects(accountId, lang) })
  const add = useMutation({
    mutationFn: () => api.setSubject(accountId, newCode.trim(), newName.trim()),
    onSuccess: () => {
      setNewCode('')
      setNewName('')
      qc.invalidateQueries({ queryKey: ['subjects', accountId] })
      qc.invalidateQueries({ queryKey: ['schedule', accountId] })
    },
  })
  const shown = useMemo(() => {
    const f = filter.trim().toLowerCase()
    const list = (subjects.data ?? []).filter((s) => !f || s.code.includes(f) || s.name.toLowerCase().includes(f))
    // The user's own names first.
    return [...list.filter((s) => s.custom), ...list.filter((s) => !s.custom)]
  }, [subjects.data, filter])
  return (
    <Section title={t('tabSubjects')} text={t('subjectsText')}>
      <form
        className="flex items-end gap-2 rounded-xl border border-dashed p-3"
        onSubmit={(e) => {
          e.preventDefault()
          if (newCode.trim() && newName.trim()) add.mutate()
        }}
      >
        <div className="grid w-24 shrink-0 gap-1.5">
          <Label htmlFor="new-code">{t('subjectCode')}</Label>
          <Input id="new-code" value={newCode} onChange={(e) => setNewCode(e.target.value)} placeholder="wi" maxLength={32} autoCapitalize="none" className="h-10 font-mono text-base" />
        </div>
        <div className="grid min-w-0 flex-1 gap-1.5">
          <Label htmlFor="new-name">{t('keyName')}</Label>
          <Input id="new-name" value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="Wiskunde" maxLength={80} className="h-10 text-base" />
        </div>
        <Button type="submit" className="h-10 shrink-0" disabled={!newCode.trim() || !newName.trim() || add.isPending} aria-label={t('subjectAdd')}>
          <Plus />
        </Button>
      </form>
      <label className="flex h-11 items-center gap-2 rounded-xl border border-input px-3 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50">
        <Search className="size-4 shrink-0 text-muted-foreground" />
        <input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t('subjectsFilter')} aria-label={t('subjectsFilter')} className="h-full min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-muted-foreground" />
      </label>
      <ul className="grid gap-2">
        {shown.map((s) => (
          <NameRow key={`${accountId}:${s.code}:${s.name}`} accountId={accountId} kind="subject" code={s.code} name={s.name} custom={s.custom} />
        ))}
      </ul>
    </Section>
  )
}

function TeachersSection({ accountId }: { accountId: string }) {
  const { t } = useI18n()
  const qc = useQueryClient()
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const teachers = useQuery({ queryKey: ['teachers', accountId], queryFn: () => api.teachers(accountId) })
  const add = useMutation({
    mutationFn: () => api.setTeacher(accountId, code.trim(), name.trim()),
    onSuccess: () => {
      setCode('')
      setName('')
      qc.invalidateQueries({ queryKey: ['teachers', accountId] })
      qc.invalidateQueries({ queryKey: ['schedule', accountId] })
    },
  })
  const shown = useMemo(() => {
    const list = teachers.data ?? []
    return [...list.filter((x) => x.custom), ...list.filter((x) => !x.custom)]
  }, [teachers.data])
  return (
    <Section title={t('teachersTitle')} text={t('teachersText')}>
      <form
        className="flex items-end gap-2 rounded-xl border border-dashed p-3"
        onSubmit={(e) => {
          e.preventDefault()
          if (code.trim() && name.trim()) add.mutate()
        }}
      >
        <div className="grid w-24 shrink-0 gap-1.5">
          <Label htmlFor="teacher-code">{t('subjectCode')}</Label>
          <Input id="teacher-code" value={code} onChange={(e) => setCode(e.target.value)} placeholder="dkr" maxLength={32} autoCapitalize="none" className="h-10 font-mono text-base" />
        </div>
        <div className="grid min-w-0 flex-1 gap-1.5">
          <Label htmlFor="teacher-name">{t('keyName')}</Label>
          <Input id="teacher-name" value={name} onChange={(e) => setName(e.target.value)} placeholder={t('teacherNamePlaceholder')} maxLength={80} className="h-10 text-base" />
        </div>
        <Button type="submit" className="h-10 shrink-0" disabled={!code.trim() || !name.trim() || add.isPending} aria-label={t('save')}>
          <Plus />
        </Button>
      </form>
      {teachers.data?.length === 0 && <p className="text-sm text-muted-foreground">{t('noTeachers')}</p>}
      <ul className="grid gap-2">
        {shown.map((x) => (
          <NameRow key={`${accountId}:${x.code}:${x.name}`} accountId={accountId} kind="teacher" code={x.code} name={x.name} custom={x.custom} />
        ))}
      </ul>
    </Section>
  )
}

function NamesTab() {
  const { active } = useSession()
  const [accountId, setAccountId] = useState(active!.id)
  return (
    <>
      <AccountPicker value={accountId} onChange={setAccountId} />
      <TeachersSection key={`t-${accountId}`} accountId={accountId} />
      <SubjectsSection key={`s-${accountId}`} accountId={accountId} />
    </>
  )
}

export function Settings() {
  const { t } = useI18n()
  const [tab, setTab] = useState('accounts')
  return (
    <div className="mx-auto min-h-dvh w-full max-w-2xl px-3 pt-[max(0.5rem,env(safe-area-inset-top))] pb-[max(2rem,env(safe-area-inset-bottom))] md:px-6">
      <header className="flex h-14 items-center gap-1">
        <Button variant="ghost" size="icon" className="size-11" aria-label={t('back')} render={<Link to="/" />}>
          <ArrowLeft className="size-5" />
        </Button>
        <h1 className="text-xl font-semibold">{t('settings')}</h1>
      </header>
      <Tabs value={tab} onValueChange={(v) => setTab(String(v))} className="gap-4">
        <TabsList className="h-11 w-full">
          <TabsTrigger value="accounts">{t('tabAccounts')}</TabsTrigger>
          <TabsTrigger value="subjects">{t('tabNames')}</TabsTrigger>
          <TabsTrigger value="connect">{t('tabConnect')}</TabsTrigger>
          <TabsTrigger value="look">{t('tabLook')}</TabsTrigger>
        </TabsList>
        <TabsContent value="accounts" className="grid gap-4">
          <AccountsTab />
        </TabsContent>
        <TabsContent value="subjects" className="grid gap-4">
          <NamesTab />
        </TabsContent>
        <TabsContent value="connect" className="grid gap-4">
          <CalendarSection />
          <KeysSection />
          <AppsSection />
        </TabsContent>
        <TabsContent value="look" className="grid gap-4">
          <LookTab />
        </TabsContent>
      </Tabs>
    </div>
  )
}
