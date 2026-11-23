import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, CalendarX, Check, ChevronRight, NotebookPen, Pencil, Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Drawer, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import { Chip } from '@/components/bits'
import { MarkdownEditor } from '@/components/MarkdownEditor'
import { api, type Homework, type Lesson } from '@/lib/api'
import { formatDay, formatTime, today } from '@/lib/dates'
import { useI18n } from '@/lib/i18n'
import { Markdown, markdownClass, tasksDone, toggleTask } from '@/lib/markdown'
import { cn, reveal } from '@/lib/utils'

const MAX = 5000

/** A text change, with done following along when the homework is its own checklist. */
const withDone = (text: string) => {
  const done = tasksDone(text)
  return done === null ? { text } : { text, done }
}

/** Markdown as one line of plain text, for previews in lists. */
const plain = (md: string) =>
  md
    .split('\n')
    .map((l) =>
      l
        .replace(/^\s*([-*]|\d+\.)\s+(\[[ xX]\]\s+)?|^#{1,3}\s+|^>\s?/, '')
        .replace(/\[(.+?)\]\(\S+?\)/g, '$1')
        .replace(/\*\*|~~|[*`]/g, '')
        .trim(),
    )
    .filter(Boolean)
    .join(' · ')

/** Whether a piece of homework lost its lesson: it dropped out or left the schedule. */
const isLoose = (h: Homework) => !h.done && (h.lesson?.status === 'cancelled' || h.lesson?.status === 'gone')

function useHomeworkActions(accountId: string) {
  const { t, locale, error } = useI18n()
  const qc = useQueryClient()
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['schedule', accountId] })
    void qc.invalidateQueries({ queryKey: ['homework', accountId] })
  }
  const onError = (e: unknown) => toast.error(error(e))
  const add = useMutation({ mutationFn: ({ lesson, text }: { lesson: Lesson; text: string }) => api.homework.add(accountId, lesson, text), onSuccess: refresh, onError })
  const update = useMutation({
    mutationFn: ({ id, ...change }: { id: string; text?: string; done?: boolean; moveTo?: string }) => api.homework.update(accountId, id, change),
    onSuccess: (h, v) => {
      refresh()
      if (v.moveTo && h.lesson) toast.success(t('homeworkMoved', { day: `${formatDay(h.lesson.date, locale, { weekday: 'long', day: 'numeric', month: 'long' })}, ${formatTime(h.lesson.start, locale)}` }))
    },
    onError,
  })
  const remove = useMutation({
    mutationFn: (id: string) => api.homework.remove(accountId, id),
    onSuccess: () => {
      refresh()
      toast.success(t('homeworkDeleted'))
    },
    onError,
  })
  return { add, update, remove }
}

/** One piece of homework: tick it off, tick its checklist, edit or delete it. */
function HomeworkItem({ h, accountId, loose }: { h: Homework; accountId: string; loose?: boolean }) {
  const { t } = useI18n()
  const { update, remove } = useHomeworkActions(accountId)
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState(h.text)
  if (editing) {
    const save = () => text.trim() && update.mutate({ id: h.id, ...withDone(text) }, { onSuccess: () => setEditing(false) })
    return (
      <div className={cn('grid gap-2', reveal)}>
        <MarkdownEditor value={text} onChange={setText} maxLength={MAX} autoFocus label={t('homeworkEdit')} onSubmit={save} />
        <div className="flex justify-end gap-2">
          <Button variant="ghost" className="h-10" onClick={() => (setText(h.text), setEditing(false))}>
            {t('cancel')}
          </Button>
          <Button className="h-10" disabled={!text.trim() || update.isPending} onClick={save}>
            {t('save')}
          </Button>
        </div>
      </div>
    )
  }
  // Homework that is a checklist is done when its items are, so it needs no checkbox of its own.
  const checklist = tasksDone(h.text) !== null
  return (
    <div data-homework={h.id} className={cn('group/hw flex gap-3 rounded-xl border bg-card px-3 py-2.5', loose && 'border-watermelon/50', h.done && 'bg-muted/40')}>
      {!checklist && <Checkbox className="mt-0.5 size-5 border-muted-foreground/60" checked={h.done} aria-label={t('homeworkMarkDone')} onCheckedChange={(done) => update.mutate({ id: h.id, done })} />}
      <div className={cn(markdownClass, 'min-w-0 flex-1', h.done && !checklist && 'text-muted-foreground [&_li]:line-through [&_p]:line-through decoration-muted-foreground/60', h.done && checklist && '[&_p]:text-muted-foreground')}>
        <Markdown source={h.text} headingIds={false} onToggleTask={(n) => update.mutate({ id: h.id, ...withDone(toggleTask(h.text, n)) })} />
      </div>
      <div className="-my-1 -mr-1.5 flex shrink-0 items-start">
        <Button variant="ghost" size="icon" className="size-9 text-muted-foreground" aria-label={t('homeworkEdit')} onClick={() => setEditing(true)}>
          <Pencil />
        </Button>
        <Button variant="ghost" size="icon" className="size-9 text-muted-foreground hover:text-destructive" aria-label={t('remove')} disabled={remove.isPending} onClick={() => remove.mutate(h.id)}>
          <Trash2 />
        </Button>
      </div>
    </div>
  )
}

/** The homework part of a lesson's details. */
export function LessonHomework({ lesson, accountId }: { lesson: Lesson; accountId: string }) {
  const { t } = useI18n()
  const { add, update } = useHomeworkActions(accountId)
  const [writing, setWriting] = useState(false)
  const [text, setText] = useState('')
  const cancelled = lesson.status === 'cancelled'
  const open = lesson.homework.filter((h) => !h.done)
  const save = () => text.trim() && add.mutate({ lesson, text }, { onSuccess: () => (setText(''), setWriting(false)) })
  return (
    <section aria-labelledby="homework-title" className="mt-4 grid gap-2 border-t pt-4">
      <div className="flex items-center gap-2">
        <NotebookPen className="size-4 text-muted-foreground" />
        <h3 id="homework-title" className="flex-1 text-sm font-medium">
          {t('homework')}
        </h3>
        {!writing && (
          <Button variant="outline" className="h-9 rounded-full" onClick={() => setWriting(true)}>
            <Plus />
            {t('homeworkAdd')}
          </Button>
        )}
      </div>
      {cancelled && open.length > 0 && (
        <div className="grid gap-2 rounded-xl bg-watermelon-tint px-3 py-2.5 text-sm text-watermelon-ink">
          <div className="flex items-start gap-2">
            <CalendarX className="mt-0.5 size-4 shrink-0" />
            <p>
              <span className="font-medium">{t('homeworkDroppedTitle')}.</span> {t('homeworkDroppedText')}
            </p>
          </div>
          <Button variant="destructive" className="h-9 justify-self-end bg-background/70 dark:bg-background/30" disabled={update.isPending} onClick={() => open.forEach((h) => update.mutate({ id: h.id, moveTo: 'next' }))}>
            {t('homeworkMoveNext')}
            <ArrowRight />
          </Button>
        </div>
      )}
      {lesson.homework.map((h) => (
        <HomeworkItem key={h.id} h={h} accountId={accountId} loose={cancelled && !h.done} />
      ))}
      {writing && (
        <div className={cn('grid gap-2', reveal)}>
          <MarkdownEditor value={text} onChange={setText} maxLength={MAX} autoFocus placeholder={t('homeworkPlaceholder')} label={t('homeworkAdd')} onSubmit={save} />
          <div className="flex justify-end gap-2">
            <Button variant="ghost" className="h-10" onClick={() => (setText(''), setWriting(false))}>
              {t('cancel')}
            </Button>
            <Button className="h-10" disabled={!text.trim() || add.isPending} onClick={save}>
              {t('save')}
            </Button>
          </div>
        </div>
      )}
    </section>
  )
}

/** A short line naming the lesson a piece of homework is on. */
function LessonLine({ h }: { h: Homework }) {
  const { t, locale } = useI18n()
  const l = h.lesson!
  return (
    <span className="flex min-w-0 items-center gap-2 text-sm">
      <span className="truncate font-medium">{l.title}</span>
      <span className="shrink-0 text-muted-foreground first-letter:uppercase">
        {formatDay(l.date, locale, { weekday: 'short', day: 'numeric', month: 'short' })} · {formatTime(l.start, locale)}
      </span>
      {l.status === 'gone' ? <Chip tone="watermelon">{t('homeworkGone')}</Chip> : l.status === 'cancelled' && <Chip tone="watermelon">{t('cancelled')}</Chip>}
    </span>
  )
}

/**
 * The homework button in the schedule's header. Its drawer lists homework
 * whose lesson dropped out first, so it can go to the next lesson, then
 * what is coming up.
 */
export function HomeworkMenu({ accountId, onPick }: { accountId: string; onPick: (date: string) => void }) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const list = useQuery({ queryKey: ['homework', accountId], queryFn: () => api.homework.list(accountId), staleTime: 30_000 })
  const { update } = useHomeworkActions(accountId)
  const all = list.data ?? []
  const loose = all.filter(isLoose)
  const now = today()
  const upcoming = all.filter((h) => !h.done && !isLoose(h) && h.lesson && h.lesson.date >= now)
  const finished = all.filter((h) => h.done)
  return (
    <>
      <Button variant="ghost" size="icon" className="size-11 shrink-0" aria-label={t('homework')} onClick={() => setOpen(true)}>
        <NotebookPen className="size-5" />
      </Button>
      <Drawer open={open} onOpenChange={setOpen} showSwipeHandle>
        <DrawerContent className="mx-auto max-w-lg">
          <div className="overflow-y-auto px-5 pt-2 pb-[max(1.5rem,env(safe-area-inset-bottom))]">
            <DrawerHeader className="items-start px-0 text-left">
              <DrawerTitle className="text-xl">{t('homework')}</DrawerTitle>
              {all.length === 0 && !list.isPending && <DrawerDescription>{t('homeworkNone')}</DrawerDescription>}
            </DrawerHeader>
            {loose.length > 0 && (
              <section className="grid gap-2 rounded-2xl bg-watermelon-tint p-3">
                <div className="flex items-start gap-2 text-watermelon-ink">
                  <CalendarX className="mt-0.5 size-4 shrink-0" />
                  <div>
                    <h3 className="text-sm font-semibold">{t('homeworkLooseTitle')}</h3>
                    <p className="text-sm opacity-90">{t('homeworkLooseText')}</p>
                  </div>
                </div>
                {loose.map((h) => (
                  <div key={h.id} className="grid gap-1.5">
                    <LessonLine h={h} />
                    <HomeworkItem h={h} accountId={accountId} loose />
                    <Button variant="destructive" className="h-9 justify-self-end bg-background/70 dark:bg-background/30" disabled={update.isPending} onClick={() => update.mutate({ id: h.id, moveTo: 'next' })}>
                      {t('homeworkMoveNext')}
                      <ArrowRight />
                    </Button>
                  </div>
                ))}
              </section>
            )}
            {upcoming.length > 0 && (
              <section className="mt-4 grid gap-1">
                <h3 className="px-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">{t('homeworkUpcoming')}</h3>
                <ul className="grid divide-y">
                  {upcoming.map((h) => (
                    <li key={h.id}>
                      <button
                        type="button"
                        onClick={() => (setOpen(false), onPick(h.lesson!.date))}
                        className="flex w-full items-center gap-3 rounded-lg px-1 py-2.5 text-left hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
                      >
                        <span className="grid min-w-0 flex-1 gap-0.5">
                          <LessonLine h={h} />
                          <span className="line-clamp-2 text-sm text-muted-foreground">{plain(h.text)}</span>
                        </span>
                        <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
                      </button>
                    </li>
                  ))}
                </ul>
              </section>
            )}
            {finished.length > 0 && (
              <p className="mt-4 flex items-center gap-1.5 px-1 text-sm text-muted-foreground">
                <Check className="size-4" />
                {t('homeworkFinished', { n: finished.length })}
              </p>
            )}
          </div>
        </DrawerContent>
      </Drawer>
    </>
  )
}
