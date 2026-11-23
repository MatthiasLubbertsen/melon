import type { Lesson } from '@/lib/api'
import { groupOverlapping } from '@/lib/groupLessons'

/**
 * Lays a week out by period, so the 3rd hour sits on the same row on every
 * day and a day that starts late shows its free first hours.
 *
 * Rows are grid lines on the desktop week grid: period p takes line
 * 2 * (p - first) + 1, and the even lines between periods hold the breaks. A lesson over several periods spans them.
 */

/** Minutes since midnight in school time; the API sends local times with their offset. */
export const minuteOfDay = (iso: string) => Number(iso.slice(11, 13)) * 60 + Number(iso.slice(14, 16))

/**
 * The periods a stretch of time takes: the ones it covers at least half of,
 * or else the one it overlaps most. Sorted, contiguous by construction of the times.
 */
export function coveredPeriods(times: Map<number, { start: number; end: number }>, start: number, end: number): number[] {
  const overlap = [...times]
    .map(([p, t]) => ({ p, len: t.end - t.start, o: Math.min(end, t.end) - Math.max(start, t.start) }))
    .filter((x) => x.o > 0)
  const most = overlap.filter((x) => x.o * 2 >= x.len)
  const picked = most.length ? most : overlap.sort((a, b) => b.o - a.o).slice(0, 1)
  return picked.map((x) => x.p).sort((a, b) => a - b)
}

/**
 * The periods a detention can take: this week's, plus the ones before them and
 * two after (detention is often after school), with times guessed from the
 * period next to them.
 */
export function detentionSlots(times: Map<number, { start: number; end: number }>): Map<number, { start: number; end: number }> {
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
}

/** A detention the account added itself, not something from Zermelo. */
export const isDetention = (l: Lesson) => l.type === 'detention'

export type Cell =
  /** groups can be empty when only detentions fill the periods. */
  | { kind: 'lessons'; groups: Lesson[][]; detentions: Lesson[]; from: number; to: number; start: number; end: number }
  | { kind: 'free'; period: number }

export type DayLayout = {
  /** Lessons and free periods, in order. Free periods only before or between lessons. */
  cells: Cell[]
  /** Lessons Melon cannot place in a period; shown after the rest. */
  unplaced: Lesson[][]
  /** Breaks between one period and the next, in minutes. */
  breaks: { after: number; minutes: number }[]
}

/** A gap shorter than this is changing rooms, not a break. */
const MIN_BREAK = 5

export type WeekLayout = {
  first: number
  last: number
  /** Lines the period rows take on the desktop grid. */
  rows: number
  days: Map<string, DayLayout>
  /** Usual start and end of each period this week, in minutes since midnight. */
  times: Map<number, { start: number; end: number }>
}

/** The grid line a period starts on. */
export const lineOf = (layout: WeekLayout, period: number) => 2 * (period - layout.first) + 1
/** How many grid lines a run of periods covers, the gaps between them included. */
export const spanOf = (from: number, to: number) => 2 * (to - from) + 1

export function layoutWeek(byDay: Map<string, Lesson[]>, days: string[]): WeekLayout {
  // The most common start and end of each period this week.
  const seen = new Map<number, { starts: number[]; ends: number[] }>()
  for (const ls of byDay.values()) {
    for (const l of ls) {
      if (!l.period || isDetention(l)) continue
      const s = seen.get(l.period.from) ?? { starts: [], ends: [] }
      s.starts.push(minuteOfDay(l.start))
      if (l.period.to === l.period.from) s.ends.push(minuteOfDay(l.end))
      seen.set(l.period.from, s)
    }
  }
  const mode = (xs: number[]) => {
    const count = new Map<number, number>()
    for (const x of xs) count.set(x, (count.get(x) ?? 0) + 1)
    return [...count].sort((a, b) => b[1] - a[1] || a[0] - b[0])[0]?.[0]
  }
  const times = new Map<number, { start: number; end: number }>()
  for (const [p, s] of seen) {
    const start = mode(s.starts)!
    times.set(p, { start, end: mode(s.ends) ?? start + 50 })
  }

  // A lesson without a period goes in the period it starts in, if it clearly does.
  const periodOf = (l: Lesson): { from: number; to: number } | null => {
    if (l.period) return l.period
    const start = minuteOfDay(l.start)
    for (const [p, t] of times) if (Math.abs(t.start - start) <= 10) return { from: p, to: p }
    return null
  }

  // A detention entered by time takes every period its time overlaps.
  const detentionPeriods = (l: Lesson): { from: number; to: number } | null => {
    if (l.period) return l.period
    const start = minuteOfDay(l.start)
    const end = minuteOfDay(l.end)
    const ps = coveredPeriods(times, start, end)
    if (ps.length) return { from: ps[0], to: ps[ps.length - 1] }
    // After the last period of the day, like most detentions: the next period number.
    const known = [...times].sort((a, b) => a[0] - b[0])
    const lastP = known[known.length - 1]
    if (lastP && start >= lastP[1].end) return { from: lastP[0] + 1, to: lastP[0] + 1 }
    return null
  }

  type Run = { groups: Lesson[][]; detentions: Lesson[]; from: number; to: number }
  let first = Infinity
  let last = -Infinity
  const placed = new Map<string, Run[]>()
  const unplacedByDay = new Map<string, Lesson[][]>()
  for (const d of days) {
    const runs: Run[] = []
    const unplaced: Lesson[][] = []
    const all = byDay.get(d) ?? []
    for (const g of groupOverlapping(all.filter((l) => !isDetention(l)))) {
      const ps = g.map(periodOf).filter((p) => p !== null)
      if (ps.length === 0) {
        unplaced.push(g)
        continue
      }
      const from = Math.min(...ps.map((p) => p.from))
      const to = Math.max(...ps.map((p) => p.to))
      // Groups that land in the same period share one cell, so nothing overlaps on the grid.
      const prev = runs[runs.length - 1]
      if (prev && from <= prev.to) {
        prev.groups.push(g)
        prev.to = Math.max(prev.to, to)
      } else {
        runs.push({ groups: [g], detentions: [], from, to })
      }
    }
    // A detention splits every lesson cell it overlaps, and takes the free periods it covers whole.
    const own: Run[] = []
    for (const found of all.filter(isDetention)) {
      const p = detentionPeriods(found)
      if (!p) {
        unplaced.push([found])
        continue
      }
      // One entered by time shows the period it landed in, like any lesson.
      const l = found.period ? found : { ...found, period: p }
      for (const r of runs) if (r.from <= p.to && p.from <= r.to) r.detentions.push(l)
      for (let k = p.from; k <= p.to; k++) {
        if (runs.some((r) => r.from <= k && k <= r.to)) continue
        const prev = own[own.length - 1]
        if (prev && prev.detentions.includes(l) && prev.to === k - 1) prev.to = k
        else own.push({ groups: [], detentions: [l], from: k, to: k })
      }
    }
    // Two detentions in the same free periods share a cell.
    own.sort((x, y) => x.from - y.from)
    for (const r of own) {
      const prev = runs.find((x) => x.groups.length === 0 && r.from <= x.to && x.from <= r.to)
      if (prev) {
        prev.detentions.push(...r.detentions.filter((x) => !prev.detentions.includes(x)))
        prev.from = Math.min(prev.from, r.from)
        prev.to = Math.max(prev.to, r.to)
      } else runs.push(r)
    }
    runs.sort((x, y) => x.from - y.from)
    for (const r of runs) {
      first = Math.min(first, r.from)
      last = Math.max(last, r.to)
    }
    placed.set(d, runs)
    unplacedByDay.set(d, unplaced)
  }
  if (first === Infinity) {
    first = 1
    last = 1
  }

  const layoutDays = new Map<string, DayLayout>()
  for (const d of days) {
    const cells: Cell[] = []
    let next = first
    for (const run of placed.get(d)!) {
      for (; next < run.from; next++) cells.push({ kind: 'free', period: next })
      const all = [...run.groups.flat(), ...run.detentions]
      cells.push({
        kind: 'lessons',
        groups: run.groups,
        detentions: run.detentions,
        from: run.from,
        to: run.to,
        start: Math.min(...all.map((l) => minuteOfDay(l.start))),
        end: Math.max(...all.map((l) => minuteOfDay(l.end))),
      })
      next = run.to + 1
    }
    const bounds = (c: Cell) => (c.kind === 'lessons' ? { start: c.start, end: c.end, from: c.from, to: c.to } : { ...times.get(c.period), from: c.period, to: c.period })
    const breaks: DayLayout['breaks'] = []
    for (let k = 1; k < cells.length; k++) {
      const a = bounds(cells[k - 1])
      const b = bounds(cells[k])
      if (a.end === undefined || b.start === undefined) continue
      const minutes = b.start - a.end
      if (minutes >= MIN_BREAK) breaks.push({ after: a.to, minutes })
    }
    layoutDays.set(d, { cells, unplaced: unplacedByDay.get(d)!, breaks })
  }
  return { first, last, rows: spanOf(first, last), days: layoutDays, times }
}
