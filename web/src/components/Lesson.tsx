import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRightLeft, Ban, ChevronDown, Clock, DoorOpen, FileText, GraduationCap, MapPin, User, Users, Video } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Drawer, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import { Chip } from '@/components/bits'
import { api, type Lesson } from '@/lib/api'
import { formatDay, formatTime } from '@/lib/dates'
import { hyphenate } from '@/lib/hyphenate'
import { useI18n, type Key } from '@/lib/i18n'
import { cn } from '@/lib/utils'

const changeIcons = { teacher: User, location: DoorOpen, group: Users, time: ArrowRightLeft }

function typeLabel(type: string): Key | null {
  return type === 'lesson' ? null : ((['exam', 'activity', 'choice', 'talk'].includes(type) ? `type_${type}` : 'type_other') as Key)
}

/**
 * Zermelo's note on a cancelled lesson is usually just "Les vervalt" or "Deze
 * les vervalt.", which the Vervalt chip already says. Only a note that adds
 * something (a reason, a replacement) is worth showing next to it.
 */
function onlySaysCancelled(note: string): boolean {
  return /^(de(ze)?\s+)?(les|activiteit|afspraak)\s+(vervalt|is vervallen)\.?$/i.test(note.trim())
}

/** Status chips: what changed is spelled out, never just a colour. */
function Chips({ lesson, isNow }: { lesson: Lesson; isNow?: boolean }) {
  const { t } = useI18n()
  const label = typeLabel(lesson.type)
  return (
    <>
      {isNow && <Chip tone="honeydew">{t('now')}</Chip>}
      {lesson.status === 'cancelled' && (
        <Chip tone="watermelon">
          <Ban />
          {t('cancelled')}
        </Chip>
      )}
      {lesson.status === 'changed' &&
        lesson.changes.map((c) => {
          const Icon = changeIcons[c]
          return (
            <Chip key={c} tone="cantaloupe" className="max-w-full">
              <Icon />
              {t(`change_${c}` as Key)}
              {lesson.previous?.[c] && <span className="min-w-0 truncate font-normal">{t('wasValue', { value: lesson.previous[c] })}</span>}
            </Chip>
          )
        })}
      {label && (
        <Chip tone="honeydew">
          {lesson.type === 'exam' && <GraduationCap />}
          {t(label)}
        </Chip>
      )}
      {lesson.online && (
        <Chip tone="honeydew">
          <Video />
          {t('online')}
        </Chip>
      )}
    </>
  )
}

export function LessonCard({ lesson, isNow, onOpen }: { lesson: Lesson; isNow: boolean; onOpen: () => void }) {
  const { t, lang, locale } = useI18n()
  const cancelled = lesson.status === 'cancelled'
  const changed = lesson.status === 'changed'
  const where = [lesson.locations.join(', '), lesson.teachers.map((x) => x.name).join(', ')].filter(Boolean).join(' · ')
  return (
    <button
      type="button"
      onClick={onOpen}
      data-status={lesson.status}
      className={cn(
        'relative flex w-full items-stretch gap-3 overflow-hidden rounded-xl border bg-card py-2.5 pr-3 pl-4 text-left transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none md:h-full md:gap-2.5 md:py-2 md:pr-2.5 md:pl-3.5',
        cancelled && 'border-watermelon/50',
        changed && 'border-cantaloupe/60',
        isNow && !cancelled && 'border-honeydew ring-2 ring-honeydew/40',
      )}
    >
      {/* The coloured edge makes changes visible while scrolling. */}
      <span aria-hidden="true" className={cn('absolute inset-y-0 left-0 w-1.5', cancelled ? 'bg-watermelon' : changed ? 'bg-cantaloupe' : isNow ? 'bg-honeydew' : 'bg-transparent')} />
      <span className="flex w-11 shrink-0 flex-col items-center justify-center text-center md:w-10">
        {lesson.period && (
          <span className={cn('leading-none', cancelled && 'text-muted-foreground')} aria-label={t('period', { n: lesson.period.from })}>
            <span className="text-xl font-semibold tabular-nums">{t('periodNumber', { n: lesson.period.from })}</span>
          </span>
        )}
        <span className={cn('text-[0.7rem] leading-tight text-muted-foreground tabular-nums', lesson.period && 'mt-1.5')}>
          {formatTime(lesson.start, locale)}
          <br />
          {formatTime(lesson.end, locale)}
        </span>
      </span>
      <span className="grid min-w-0 flex-1 grid-cols-[minmax(0,1fr)] content-center gap-1">
        <span className={cn('line-clamp-2 text-base leading-snug font-medium break-words hyphens-auto', cancelled && 'text-muted-foreground line-through decoration-watermelon decoration-2')}>
          {hyphenate(lesson.title, lang)}
        </span>
        {where && <span className={cn('truncate text-sm text-muted-foreground', cancelled && 'line-through')}>{where}</span>}
        {(lesson.status !== 'scheduled' || lesson.type !== 'lesson' || lesson.online || isNow) && (
          <span className="mt-0.5 flex min-w-0 flex-wrap gap-1.5">
            <Chips lesson={lesson} isNow={isNow && !cancelled} />
          </span>
        )}
        {lesson.changeRest && !cancelled && <span className="text-sm text-cantaloupe-ink">{lesson.changeRest}</span>}
        {lesson.note && lesson.note !== lesson.title && <span className="truncate text-sm text-muted-foreground">{lesson.note}</span>}
      </span>
    </button>
  )
}

/**
 * Lessons that overlap in time, shown as one card like Zermelo does. Tapping it
 * expands in place to show every lesson; each one still opens its own details.
 */
export function LessonGroup({ lessons, isNow, onOpen }: { lessons: Lesson[]; isNow: (l: Lesson) => boolean; onOpen: (l: Lesson) => void }) {
  const { t, locale } = useI18n()
  const [expanded, setExpanded] = useState(false)
  const start = Math.min(...lessons.map((l) => new Date(l.start).getTime()))
  const end = Math.max(...lessons.map((l) => new Date(l.end).getTime()))
  const changed = lessons.some((l) => l.status === 'changed')
  const cancelled = lessons.every((l) => l.status === 'cancelled')
  const names = lessons.map((l) => l.title).join(', ')
  return (
    <div className="grid gap-2">
      <button
        type="button"
        onClick={() => setExpanded((e) => !e)}
        aria-expanded={expanded}
        className={cn(
          'relative flex w-full items-stretch gap-3 overflow-hidden rounded-xl border bg-card py-2.5 pr-3 pl-4 text-left md:py-2 transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
          changed && 'border-cantaloupe/60',
          lessons.some(isNow) && !cancelled && 'border-honeydew ring-2 ring-honeydew/40',
        )}
      >
        <span aria-hidden="true" className={cn('absolute inset-y-0 left-0 w-1.5', changed ? 'bg-cantaloupe' : lessons.some(isNow) ? 'bg-honeydew' : 'bg-transparent')} />
        <span className="flex w-11 shrink-0 flex-col items-center justify-center text-center text-[0.7rem] leading-tight text-muted-foreground tabular-nums">
          {formatTime(new Date(start).toISOString(), locale)}
          <br />
          {formatTime(new Date(end).toISOString(), locale)}
        </span>
        <span className="grid min-w-0 flex-1 content-center gap-1">
          <span className="text-base leading-snug font-medium">{t('multiLessons', { n: lessons.length })}</span>
          <span className="truncate text-sm text-muted-foreground">{names}</span>
          <span className="sr-only">{t('multiExpand')}</span>
        </span>
        <ChevronDown aria-hidden="true" className={cn('size-4 shrink-0 self-center text-muted-foreground transition-transform', expanded && 'rotate-180')} />
      </button>
      {/* Rows animate from 0fr to 1fr, so the lessons slide open and closed instead of popping in. */}
      <div
        inert={!expanded}
        className={cn(
          '-mt-2 grid transition-[grid-template-rows,opacity,margin] duration-300 ease-out motion-reduce:transition-none',
          expanded ? 'mt-0 grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0',
        )}
      >
        <div className="min-h-0 overflow-hidden">
          <div className={cn('grid gap-2 border-l-2 border-border pl-3 transition-transform duration-300 ease-out motion-reduce:transition-none', expanded ? 'translate-y-0' : '-translate-y-2')}>
            {lessons.map((l) => (
              <LessonCard key={l.id} lesson={l} isNow={isNow(l)} onOpen={() => onOpen(l)} />
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function Row({ icon: Icon, label, value }: { icon: typeof Clock; label: string; value: string }) {
  if (!value) return null
  return (
    <div className="flex items-start gap-3 py-2.5">
      <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
      <div className="grid min-w-0">
        <span className="text-xs text-muted-foreground">{label}</span>
        <span className="text-sm break-words">{value}</span>
      </div>
    </div>
  )
}

/** Naming a subject or teacher happens right where you see it. */
function NameEdit({ accountId, kind, code, name }: { accountId: string; kind: 'subject' | 'teacher'; code: string; name: string }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  // A teacher nobody named yet shows as its code; start that field empty.
  const current = kind === 'teacher' && name === code ? '' : name
  const [value, setValue] = useState(current)
  useEffect(() => setValue(current), [current])
  const list = kind === 'subject' ? 'subjects' : 'teachers'
  const known = useQuery<{ code: string; custom: boolean }[]>({
    queryKey: kind === 'subject' ? ['subjects', accountId, lang] : ['teachers', accountId],
    queryFn: () => (kind === 'subject' ? api.subjects(accountId, lang) : api.teachers(accountId)),
    staleTime: 60_000,
  })
  const custom = known.data?.find((e) => e.code === code.toLowerCase())?.custom
  const done = () => {
    qc.invalidateQueries({ queryKey: ['schedule', accountId] })
    qc.invalidateQueries({ queryKey: [list, accountId] })
    toast.success(t('saved'))
  }
  const save = useMutation({ mutationFn: () => (kind === 'subject' ? api.setSubject(accountId, code, value) : api.setTeacher(accountId, code, value)), onSuccess: done })
  const reset = useMutation({ mutationFn: () => (kind === 'subject' ? api.resetSubject(accountId, code) : api.resetTeacher(accountId, code)), onSuccess: done })
  const id = `${kind}-${code}`
  return (
    <form
      className="grid gap-1.5"
      onSubmit={(e) => {
        e.preventDefault()
        if (value.trim()) save.mutate()
      }}
    >
      <Label htmlFor={id}>{t(kind === 'subject' ? 'subjectName' : 'teacherName')}</Label>
      <div className="flex gap-2">
        <Input id={id} value={value} onChange={(e) => setValue(e.target.value)} maxLength={80} placeholder={kind === 'teacher' ? t('teacherNamePlaceholder') : undefined} className="h-11 text-base" />
        <Button type="submit" className="h-11" disabled={!value.trim() || value.trim() === current || save.isPending}>
          {t('save')}
        </Button>
        {custom && (
          <Button type="button" variant="outline" className="h-11" onClick={() => reset.mutate()} disabled={reset.isPending}>
            {t(kind === 'subject' ? 'reset' : 'remove')}
          </Button>
        )}
      </div>
      <p className="text-sm text-muted-foreground">{t('subjectNameHint', { code })}</p>
    </form>
  )
}

export function LessonDrawer({ lesson, accountId, onClose }: { lesson: Lesson | null; accountId: string; onClose: () => void }) {
  const { t, locale } = useI18n()
  // Keep the last lesson around so the sheet can animate out with content.
  const [shown, setShown] = useState(lesson)
  useEffect(() => {
    if (lesson) setShown(lesson)
  }, [lesson])
  const l = lesson ?? shown
  return (
    <Drawer open={lesson !== null} onOpenChange={(o) => !o && onClose()} showSwipeHandle>
      <DrawerContent className="mx-auto max-w-lg">
        {l && (
          <div className="overflow-y-auto px-5 pt-2 pb-[max(1.5rem,env(safe-area-inset-bottom))]">
            <DrawerHeader className="items-start px-0 text-left">
              <DrawerTitle className={cn('text-xl', l.status === 'cancelled' && 'line-through decoration-watermelon decoration-2')}>{l.title}</DrawerTitle>
              <DrawerDescription>
                {formatDay(l.date, locale, { weekday: 'long', day: 'numeric', month: 'long' })}
                {l.period ? ` · ${t('period', { n: l.period.from })}` : ''}
              </DrawerDescription>
            </DrawerHeader>
            <div className="flex flex-wrap gap-1.5">
              <Chips lesson={l} />
            </div>
            {(() => {
              const note = l.status === 'changed' ? l.changeRest : l.changeNote
              if (!note || (l.status === 'cancelled' && onlySaysCancelled(note))) return null
              return <p className={cn('mt-3 rounded-lg px-3 py-2 text-sm', l.status === 'cancelled' ? 'bg-watermelon-tint text-watermelon-ink' : 'bg-cantaloupe-tint text-cantaloupe-ink')}>{note}</p>
            })()}
            <div className="mt-2 divide-y">
              <Row icon={Clock} label={t('time')} value={`${formatTime(l.start, locale)} – ${formatTime(l.end, locale)}`} />
              <Row icon={MapPin} label={t('location')} value={l.locations.join(', ')} />
              <Row icon={User} label={t('teacher')} value={l.teachers.map((x) => (x.name === x.code ? x.code : `${x.name} (${x.code})`)).join(', ')} />
              <Row icon={Users} label={t('group')} value={l.groups.join(', ')} />
              <Row icon={FileText} label={t('note')} value={l.note} />
            </div>
            {l.onlineUrl && (
              <Button className="mt-2 h-11 w-full" render={<a href={l.onlineUrl} target="_blank" rel="noreferrer" />}>
                <Video />
                {t('online')}
              </Button>
            )}
            {l.subjects.length + l.teachers.length > 0 && (
              <div className="mt-4 grid gap-4 border-t pt-4">
                {l.subjects.map((s) => (
                  <NameEdit key={`s-${s.code}`} accountId={accountId} kind="subject" code={s.code} name={s.name} />
                ))}
                {l.teachers.map((x) => (
                  <NameEdit key={`t-${x.code}`} accountId={accountId} kind="teacher" code={x.code} name={x.name} />
                ))}
              </div>
            )}
          </div>
        )}
      </DrawerContent>
    </Drawer>
  )
}
