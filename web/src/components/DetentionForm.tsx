import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Hourglass } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { coveredPeriods } from '@/lib/layoutWeek'
import { cn } from '@/lib/utils'

const hhmm = (minutes: number) => `${String(Math.floor(minutes / 60)).padStart(2, '0')}:${String(minutes % 60).padStart(2, '0')}`
const minutes = (v: string) => (/^\d{2}:\d{2}$/.test(v) ? Number(v.slice(0, 2)) * 60 + Number(v.slice(3, 5)) : NaN)

type Range = { from: number; to: number }

/**
 * Adding a detention: pick the periods (tap one, or drag across several) or
 * type the times, then optionally link a subject and say where.
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
}) {
  const { t } = useI18n()
  const qc = useQueryClient()
  // The periods this week has, plus the ones before and two after it (detention is often after school),
  // with times guessed from the period next to them.
  const slots = useMemo(() => {
    const out = new Map(times)
    const known = [...times.keys()].sort((a, b) => a - b)
    if (known.length === 0) return out
    const first = known[0]
    const last = known[known.length - 1]
    for (let p = first - 1; p >= 1; p--) {
      const next = out.get(p + 1)!
      out.set(p, { start: next.start - (next.end - next.start), end: next.start })
    }
    for (let p = last + 1; p <= last + 2; p++) {
      const prev = out.get(p - 1)!
      if (prev.end + (prev.end - prev.start) >= 24 * 60) break
      out.set(p, { start: prev.end, end: prev.end + (prev.end - prev.start) })
    }
    return out
  }, [times])
  const periods = useMemo(() => [...slots.keys()].sort((a, b) => a - b), [slots])

  const [date, setDate] = useState(day)
  const [range, setRange] = useState<Range | null>(null)
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [subject, setSubject] = useState('')
  const [note, setNote] = useState('')
  const [location, setLocation] = useState('')

  // Start fresh on the day being looked at every time it opens.
  useEffect(() => {
    if (!open) return
    setDate(day)
    setRange(null)
    setStart('')
    setEnd('')
    setSubject('')
    setNote('')
    setLocation('')
  }, [open, day])

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

  // Click and drag: the period the pointer went down on is the anchor, the one under it now the other end.
  const drag = useRef<number | null>(null)
  useEffect(() => {
    const stop = () => (drag.current = null)
    window.addEventListener('pointerup', stop)
    return () => window.removeEventListener('pointerup', stop)
  }, [])
  const periodAt = (x: number, y: number) => {
    const el = document.elementFromPoint(x, y)?.closest<HTMLElement>('[data-period]')
    return el ? Number(el.dataset.period) : null
  }

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
    mutationFn: () => api.addDetention(accountId, { date, start, end, period: range, subject, title: note.trim(), location: location.trim() }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['schedule', accountId] })
      toast.success(t('detentionAdded'))
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
            {t('addDetention')}
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

          {periods.length > 0 && (
            <div className="grid gap-1.5">
              <span className="text-sm font-medium" id="detention-periods">
                {t('detentionPeriods')}
              </span>
              <div
                role="group"
                aria-labelledby="detention-periods"
                className="flex touch-none gap-1 select-none"
                onPointerMove={(e) => {
                  if (drag.current === null) return
                  const p = periodAt(e.clientX, e.clientY)
                  if (p !== null) pick({ from: drag.current, to: p })
                }}
              >
                {periods.map((p) => {
                  const on = range !== null && p >= range.from && p <= range.to
                  return (
                    <button
                      key={p}
                      type="button"
                      data-period={p}
                      aria-pressed={on}
                      onPointerDown={(e) => {
                        // Let the moves reach the strip, not this one button.
                        ;(e.currentTarget as HTMLElement).releasePointerCapture?.(e.pointerId)
                        drag.current = p
                        pick({ from: p, to: p })
                      }}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault()
                          pick(e.shiftKey && range ? { from: range.from, to: p } : { from: p, to: p })
                        }
                      }}
                      className={cn(
                        'flex h-11 min-w-0 flex-1 items-center justify-center rounded-lg border text-sm font-semibold tabular-nums transition-colors focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
                        on ? 'border-cantaloupe bg-cantaloupe-tint text-cantaloupe-ink' : 'hover:bg-muted',
                      )}
                    >
                      {t('periodNumber', { n: p })}
                    </button>
                  )
                })}
              </div>
              <p className="text-xs text-muted-foreground">{t('detentionPeriodsHint')}</p>
            </div>
          )}

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
            {t('detentionAdd')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
