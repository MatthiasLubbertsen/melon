import type { Lesson } from '@/lib/api'

const ms = (iso: string) => new Date(iso).getTime()

/**
 * Splits one day's lessons into groups of lessons that overlap in time, the way
 * Zermelo shows two lessons at the same hour as one "Multiple lessons (2)" card.
 * Groups keep their lessons in start order; a lesson that touches the end of a
 * group (same end and start time) does not overlap it.
 */
export function groupOverlapping(lessons: Lesson[]): Lesson[][] {
  const sorted = [...lessons].sort((a, b) => ms(a.start) - ms(b.start) || ms(a.end) - ms(b.end))
  const groups: Lesson[][] = []
  let current: Lesson[] = []
  let currentEnd = -Infinity
  for (const l of sorted) {
    if (current.length > 0 && ms(l.start) < currentEnd) {
      current.push(l)
      currentEnd = Math.max(currentEnd, ms(l.end))
    } else {
      if (current.length > 0) groups.push(current)
      current = [l]
      currentEnd = ms(l.end)
    }
  }
  if (current.length > 0) groups.push(current)
  return groups
}
