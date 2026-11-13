import { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowRightLeft, Ban, Clock, DoorOpen, FileText, GraduationCap, MapPin, User, Users, Video } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Drawer, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import { Chip } from '@/components/bits'
import { api, type Lesson, type SubjectEntry } from '@/lib/api'
import { formatDay, formatTime } from '@/lib/dates'
import { useI18n, type Key } from '@/lib/i18n'
import { cn } from '@/lib/utils'

const changeIcons = { teacher: User, location: DoorOpen, group: Users, time: ArrowRightLeft }

function typeLabel(type: string): Key | null {
  return type === 'lesson' ? null : ((['exam', 'activity', 'choice', 'talk'].includes(type) ? `type_${type}` : 'type_other') as Key)
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
            <Chip key={c} tone="cantaloupe">
              <Icon />
              {t(`change_${c}` as Key)}
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
  const { t, locale } = useI18n()
  const cancelled = lesson.status === 'cancelled'
  const changed = lesson.status === 'changed'
  const where = [lesson.locations.join(', '), lesson.teachers.join(', ')].filter(Boolean).join(' · ')
  return (
    <button
      type="button"
      onClick={onOpen}
      data-status={lesson.status}
      className={cn(
        'relative flex w-full items-stretch gap-3 overflow-hidden rounded-xl border bg-card py-3 pr-3 pl-4 text-left transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
        cancelled && 'border-watermelon/50',
        changed && 'border-cantaloupe/60',
        isNow && !cancelled && 'border-honeydew ring-2 ring-honeydew/40',
      )}
    >
      {/* The coloured edge makes changes visible while scrolling. */}
      <span aria-hidden="true" className={cn('absolute inset-y-0 left-0 w-1.5', cancelled ? 'bg-watermelon' : changed ? 'bg-cantaloupe' : isNow ? 'bg-honeydew' : 'bg-transparent')} />
      <span className="flex w-11 shrink-0 flex-col items-center justify-center text-center">
        <span className={cn('text-xl leading-none font-semibold tabular-nums', cancelled && 'text-muted-foreground')}>{lesson.period ? lesson.period.from : '·'}</span>
        <span className="mt-1.5 text-[0.7rem] leading-tight text-muted-foreground tabular-nums">
          {formatTime(lesson.start, locale)}
          <br />
          {formatTime(lesson.end, locale)}
        </span>
      </span>
      <span className="grid min-w-0 flex-1 content-center gap-1">
        <span className={cn('line-clamp-2 text-base leading-snug font-medium break-words', cancelled && 'text-muted-foreground line-through decoration-watermelon decoration-2')}>
          {lesson.title}
        </span>
        {where && <span className={cn('truncate text-sm text-muted-foreground', cancelled && 'line-through')}>{where}</span>}
        {(lesson.status !== 'scheduled' || lesson.type !== 'lesson' || lesson.online || isNow) && (
          <span className="mt-0.5 flex flex-wrap gap-1.5">
            <Chips lesson={lesson} isNow={isNow && !cancelled} />
          </span>
        )}
        {lesson.changeNote && !cancelled && <span className="text-sm text-cantaloupe-ink">{lesson.changeNote}</span>}
        {lesson.note && lesson.note !== lesson.title && <span className="truncate text-sm text-muted-foreground">{lesson.note}</span>}
      </span>
      <span className="sr-only">{lesson.period ? t('period', { n: lesson.period.from }) : ''}</span>
    </button>
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

/** Renaming a subject happens right where you see it. */
function SubjectRename({ accountId, code, name }: { accountId: string; code: string; name: string }) {
  const { t } = useI18n()
  const qc = useQueryClient()
  const [value, setValue] = useState(name)
  useEffect(() => setValue(name), [name])
  const done = () => {
    qc.invalidateQueries({ queryKey: ['schedule', accountId] })
    qc.invalidateQueries({ queryKey: ['subjects', accountId] })
    toast.success(t('saved'))
  }
  const custom = qc.getQueryData<SubjectEntry[]>(['subjects', accountId])?.find((s) => s.code === code.toLowerCase())?.custom
  const save = useMutation({ mutationFn: () => api.setSubject(accountId, code, value), onSuccess: done })
  const reset = useMutation({ mutationFn: () => api.resetSubject(accountId, code), onSuccess: done })
  return (
    <form
      className="grid gap-1.5"
      onSubmit={(e) => {
        e.preventDefault()
        if (value.trim()) save.mutate()
      }}
    >
      <Label htmlFor={`subject-${code}`}>{t('subjectName')}</Label>
      <div className="flex gap-2">
        <Input id={`subject-${code}`} value={value} onChange={(e) => setValue(e.target.value)} maxLength={80} className="h-11 text-base" />
        <Button type="submit" className="h-11" disabled={!value.trim() || value.trim() === name || save.isPending}>
          {t('save')}
        </Button>
        {custom && (
          <Button type="button" variant="outline" className="h-11" onClick={() => reset.mutate()} disabled={reset.isPending}>
            {t('reset')}
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
            {l.changeNote && (
              <p className={cn('mt-3 rounded-lg px-3 py-2 text-sm', l.status === 'cancelled' ? 'bg-watermelon-tint text-watermelon-ink' : 'bg-cantaloupe-tint text-cantaloupe-ink')}>{l.changeNote}</p>
            )}
            <div className="mt-2 divide-y">
              <Row icon={Clock} label={t('time')} value={`${formatTime(l.start, locale)} – ${formatTime(l.end, locale)}`} />
              <Row icon={MapPin} label={t('location')} value={l.locations.join(', ')} />
              <Row icon={User} label={t('teacher')} value={l.teachers.join(', ')} />
              <Row icon={Users} label={t('group')} value={l.groups.join(', ')} />
              <Row icon={FileText} label={t('note')} value={l.note} />
            </div>
            {l.onlineUrl && (
              <Button className="mt-2 h-11 w-full" render={<a href={l.onlineUrl} target="_blank" rel="noreferrer" />}>
                <Video />
                {t('online')}
              </Button>
            )}
            {l.subjects.length > 0 && (
              <div className="mt-4 grid gap-4 border-t pt-4">
                {l.subjects.map((s) => (
                  <SubjectRename key={s.code} accountId={accountId} code={s.code} name={s.name} />
                ))}
              </div>
            )}
          </div>
        )}
      </DrawerContent>
    </Drawer>
  )
}
