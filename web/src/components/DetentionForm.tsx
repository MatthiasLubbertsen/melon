import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Hand, Hourglass } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api, type Detention } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { coveredPeriods, detentionSlots } from '@/lib/layoutWeek'
import { cn } from '@/lib/utils'

const hhmm = (minutes: number) => `${String(Math.floor(minutes / 60)).padStart(2, '0')}:${String(minutes % 60).padStart(2, '0')}`
const minutes = (v: string) => (/^\d{2}:\d{2}$/.test(v) ? Number(v.slice(0, 2)) * 60 + Number(v.slice(3, 5)) : NaN)

type Range = { from: number; to: number }

/** Periods picked by dragging over the schedule itself. */
export type PickedPeriods = Range & { date: string }

/**
 * Adding or changing a detention: pick the periods by dragging over the
 * schedule (the dialog steps aside for it) or type the times, then
 * optionally link a subject and say where.
 */
export function DetentionDialog({
  open,
  onOpenChange,
  accountId,
  day,
  times,
  subjects,
  rooms,
  onAdded,
  session,
  picked,
  onPick,
  editing,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  accountId: string
  day: string
  /** Usual start and end of each period, in minutes since midnight. */
  times: Map<number, { start: number; end: number }>
  /** Subjects to link to, from the lessons on screen. */
  subjects: { code: string; name: string }[]
  /** Rooms from the lessons on screen, offered as suggestions. */
  rooms: string[]
  onAdded: (date: string) => void
  /** Goes up each time someone starts a new detention, so the form starts empty then and not when it comes back from picking. */
  session: number
  /** What was last picked on the schedule. */
  picked: PickedPeriods | null
  /** Step aside to pick the periods on the schedule. */
  onPick: () => void
  /** The detention being changed, or null when adding a new one. */
  editing: Detention | null
}) {
  const { t } = useI18n()
  const qc = useQueryClient()
  const slots = useMemo(() => detentionSlots(times), [times])

  const [date, setDate] = useState(day)
  const [range, setRange] = useState<Range | null>(null)
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [subject, setSubject] = useState('')
  const [note, setNote] = useState('')
  const [location, setLocation] = useState('')

  // Start fresh on the day being looked at for every new detention, or from the one being changed.
  useEffect(() => {
    setDate(editing?.date ?? day)
    setRange(editing?.period ?? null)
    setStart(editing?.start ?? '')
    setEnd(editing?.end ?? '')
    setSubject(editing?.subject ?? '')
    setNote(editing?.title ?? '')
    setLocation(editing?.location ?? '')
    // Not on day: the day changes while picking on the schedule, and that must not empty the form.
  }, [session]) // eslint-disable-line react-hooks/exhaustive-deps

  const pick = (r: Range) => {
    const from = Math.min(r.from, r.to)
    const to = Math.max(r.from, r.to)
    setRange({ from, to })
    const a = slots.get(from)
    const b = slots.get(to)
    if (a && b) {
      setStart(hhmm(a.start))
      setEnd(hhmm(b.end))
    }
  }

  // Back from the schedule: take the day and the periods dragged over there.
  useEffect(() => {
    if (!picked) return
    setDate(picked.date)
    pick(picked)
    // pick only reads slots, which do not change while picking.
  }, [picked]) // eslint-disable-line react-hooks/exhaustive-deps

  // Typed times take the periods they overlap, so the card still says which hour it is.
  const typed = (s: string, e: string) => {
    setStart(s)
    setEnd(e)
    const a = minutes(s)
    const b = minutes(e)
    const hit = b > a ? coveredPeriods(slots, a, b) : []
    setRange(hit.length ? { from: hit[0], to: hit[hit.length - 1] } : null)
  }

  const valid = /^\d{4}-\d{2}-\d{2}$/.test(date) && minutes(end) > minutes(start)
  const badTime = !Number.isNaN(minutes(start)) && !Number.isNaN(minutes(end)) && minutes(end) <= minutes(start)

  const add = useMutation({
    mutationFn: () => {
      const body = { date, start, end, period: range, subject, title: note.trim(), location: location.trim() }
      return editing ? api.updateDetention(accountId, editing.id, body) : api.addDetention(accountId, body)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['schedule', accountId] })
      toast.success(t(editing ? 'detentionSaved' : 'detentionAdded'))
      onAdded(date)
      onOpenChange(false)
    },
    onError: (e) => toast.error(e.message),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[90dvh] flex-col sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-lg font-semibold">
            <Hourglass className="size-5 text-cantaloupe-ink" />
            {t(editing ? 'editDetention' : 'addDetention')}
          </DialogTitle>
          <DialogDescription>{t('detentionIntro')}</DialogDescription>
        </DialogHeader>
        <form
          id="detention-form"
          className="-mx-1 grid gap-4 overflow-y-auto px-1 pb-1"
          onSubmit={(e) => {
            e.preventDefault()
            if (valid) add.mutate()
          }}
        >
          <div className="grid gap-1.5">
            <Label htmlFor="detention-date">{t('detentionDay')}</Label>
            <Input id="detention-date" type="date" required value={date} onChange={(e) => setDate(e.target.value)} className="h-11 text-base" />
          </div>

          <div className="grid gap-1.5">
            <Button type="button" variant="outline" className={cn('h-11 justify-start', range && 'border-cantaloupe bg-cantaloupe-tint text-cantaloupe-ink hover:bg-cantaloupe-tint/70')} onClick={onPick}>
              <Hand />
              {!range ? t('detentionPickOnSchedule') : range.from === range.to ? t('detentionPickedOne', { n: t('periodNumber', { n: range.from }) }) : t('detentionPicked', { from: t('periodNumber', { n: range.from }), to: t('periodNumber', { n: range.to }) })}
            </Button>
            <p className="text-xs text-muted-foreground">{t('detentionPickHint')}</p>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="detention-start">{t('detentionFrom')}</Label>
              <Input id="detention-start" type="time" required value={start} onChange={(e) => typed(e.target.value, end)} className="h-11 text-base" />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="detention-end">{t('detentionTo')}</Label>
              <Input id="detention-end" type="time" required value={end} onChange={(e) => typed(start, e.target.value)} className="h-11 text-base" aria-invalid={badTime || undefined} />
            </div>
            {badTime && <p className="col-span-2 -mt-1 text-sm text-watermelon-ink">{t('detentionTimeInvalid')}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="detention-location">
              {t('location')} <span className="font-normal text-muted-foreground">({t('optional').toLowerCase()})</span>
            </Label>
            <Input id="detention-location" value={location} onChange={(e) => setLocation(e.target.value)} maxLength={40} list="detention-rooms" autoComplete="off" placeholder={t('detentionLocationPlaceholder')} className="h-11 text-base" />
            <datalist id="detention-rooms">
              {rooms.map((r) => (
                <option key={r} value={r} />
              ))}
            </datalist>
          </div>

          <div className="grid gap-1.5">
            <span className="text-sm font-medium" id="detention-subject">
              {t('detentionSubject')}
            </span>
            <div role="radiogroup" aria-labelledby="detention-subject" className="flex flex-wrap gap-1.5">
              {[{ code: '', name: t('detentionNoSubject') }, ...subjects].map((s) => (
                <button
                  key={s.code}
                  type="button"
                  role="radio"
                  aria-checked={subject === s.code}
                  onClick={() => setSubject(s.code)}
                  className={cn(
                    'h-8 max-w-full truncate rounded-full border px-3 text-sm transition-colors focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
                    subject === s.code ? 'border-primary bg-primary text-primary-foreground' : 'hover:bg-muted',
                  )}
                >
                  {s.name}
                </button>
              ))}
            </div>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="detention-note">
              {t('detentionNote')} <span className="font-normal text-muted-foreground">({t('optional').toLowerCase()})</span>
            </Label>
            <Input id="detention-note" value={note} onChange={(e) => setNote(e.target.value)} maxLength={80} placeholder={t('detentionNotePlaceholder')} className="h-11 text-base" />
          </div>

        </form>
        <DialogFooter>
          <Button type="button" variant="outline" className="h-11" onClick={() => onOpenChange(false)}>
            {t('cancel')}
          </Button>
          <Button type="submit" form="detention-form" className="h-11" disabled={!valid || add.isPending}>
            {t(editing ? 'save' : 'detentionAdd')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
