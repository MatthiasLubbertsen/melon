import type { Lesson } from '@/lib/api'
import { groupOverlapping } from '@/lib/groupLessons'

/**
 * Lays a week out by period, so the 3rd hour sits on the same row on every
 * day and a day that starts late shows its free first hours.
 *
 * Rows are grid lines on the desktop week grid: period p takes line
 * 2 * (p - first) + 1, and the even lines between periods are left for what
 * happens between them (breaks). A lesson over several periods spans them.
 */

/** Minutes since midnight in school time; the API sends local times with their offset. */
export const minuteOfDay = (iso: string) => Number(iso.slice(11, 13)) * 60 + Number(iso.slice(14, 16))

export type Cell =
  | { kind: 'lessons'; groups: Lesson[][]; from: number; to: number; start: number; end: number }
  | { kind: 'free'; period: number }

export type DayLayout = {
  /** Lessons and free periods, in order. Free periods only before or between lessons. */
  cells: Cell[]
  /** Lessons Melon cannot place in a period; shown after the rest. */
  unplaced: Lesson[][]
}

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
      if (!l.period) continue
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

  let first = Infinity
  let last = -Infinity
  const placed = new Map<string, { groups: Lesson[][]; from: number; to: number }[]>()
  const unplacedByDay = new Map<string, Lesson[][]>()
  for (const d of days) {
    const runs: { groups: Lesson[][]; from: number; to: number }[] = []
    const unplaced: Lesson[][] = []
    for (const g of groupOverlapping(byDay.get(d) ?? [])) {
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
        runs.push({ groups: [g], from, to })
      }
      first = Math.min(first, from)
      last = Math.max(last, to)
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
      const all = run.groups.flat()
      cells.push({
        kind: 'lessons',
        groups: run.groups,
        from: run.from,
        to: run.to,
        start: Math.min(...all.map((l) => minuteOfDay(l.start))),
        end: Math.max(...all.map((l) => minuteOfDay(l.end))),
      })
      next = run.to + 1
    }
    layoutDays.set(d, { cells, unplaced: unplacedByDay.get(d)! })
  }
  return { first, last, rows: spanOf(first, last), days: layoutDays, times }
}
