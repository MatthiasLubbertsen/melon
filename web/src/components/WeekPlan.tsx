import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { CalendarRange, ExternalLink, Pencil, Plus } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { api, type Lesson } from '@/lib/api'
import { isoWeek, weekNumber } from '@/lib/dates'
import { useI18n } from '@/lib/i18n'
import { cn, reveal } from '@/lib/utils'

const host = (u: string) => {
  try {
    return new URL(u).host.replace(/^www\./, '')
  } catch {
    return u
  }
}

/**
 * The week plan (an itslearning plan, say) for this lesson's subject. One link
 * per subject per week, so it shows on every lesson of that subject that week.
 */
export function WeekPlan({ lesson, accountId }: { lesson: Lesson; accountId: string }) {
  const { t, error } = useI18n()
  const qc = useQueryClient()
  const subject = lesson.subjects[0]
  const week = isoWeek(lesson.date)
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState(lesson.weekPlan)
  const save = useMutation({
    mutationFn: async (url: string) => {
      if (url) await api.setWeekPlan(accountId, subject.code, week, url)
      else await api.deleteWeekPlan(accountId, subject.code, week)
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['schedule', accountId] })
      setEditing(false)
      toast.success(t('saved'))
    },
    onError: (e) => toast.error(error(e)),
  })
  if (!subject) return null
  const vars = { subject: subject.name, n: weekNumber(lesson.date) }

  if (editing) {
    return (
      <form
        className={cn('mt-4 grid gap-1.5 border-t pt-4', reveal)}
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate(value.trim())
        }}
      >
        <label htmlFor="week-plan" className="text-sm font-medium">
          {t('weekPlanTitle', vars)}
        </label>
        <Input id="week-plan" type="url" inputMode="url" autoFocus value={value} onChange={(e) => setValue(e.target.value)} placeholder="https://school.itslearning.com/plans/…" className="h-11 text-base" />
        <p className="text-sm text-muted-foreground">{t('weekPlanHint', vars)}</p>
        <div className="flex justify-end gap-2">
          {lesson.weekPlan && (
            <Button type="button" variant="outline" className="mr-auto h-10" disabled={save.isPending} onClick={() => save.mutate('')}>
              {t('remove')}
            </Button>
          )}
          <Button type="button" variant="ghost" className="h-10" onClick={() => (setValue(lesson.weekPlan), setEditing(false))}>
            {t('cancel')}
          </Button>
          <Button type="submit" className="h-10" disabled={!value.trim() || value.trim() === lesson.weekPlan || save.isPending}>
            {t('save')}
          </Button>
        </div>
      </form>
    )
  }

  if (!lesson.weekPlan) {
    return (
      <div className="mt-4 border-t pt-4">
        <Button variant="ghost" className="-ml-2 h-9 text-muted-foreground" onClick={() => (setValue(''), setEditing(true))}>
          <Plus />
          {t('weekPlanAdd')}
        </Button>
      </div>
    )
  }

  return (
    <div className="mt-4 flex items-center gap-2 border-t pt-4">
      <a
        href={lesson.weekPlan}
        target="_blank"
        rel="noreferrer"
        className="flex min-h-12 min-w-0 flex-1 items-center gap-3 rounded-xl border bg-honeydew-tint px-3 py-2 text-honeydew-ink transition-colors hover:bg-honeydew/30 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
      >
        <CalendarRange className="size-5 shrink-0" />
        <span className="grid min-w-0 flex-1">
          <span className="text-sm font-medium">{t('weekPlanOpen', vars)}</span>
          <span className="truncate text-xs opacity-80">{host(lesson.weekPlan)}</span>
        </span>
        <ExternalLink className="size-4 shrink-0" />
      </a>
      <Button variant="ghost" size="icon" className="size-11 shrink-0 text-muted-foreground" aria-label={t('weekPlanEdit')} onClick={() => (setValue(lesson.weekPlan), setEditing(true))}>
        <Pencil />
      </Button>
    </div>
  )
}
