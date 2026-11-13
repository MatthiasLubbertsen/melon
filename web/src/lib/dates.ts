// Date helpers. Days are plain "YYYY-MM-DD" strings in school time
// (Europe/Amsterdam), so nothing shifts when the device is in another zone.

export const SCHOOL_TZ = 'Europe/Amsterdam'

const pad = (n: number) => String(n).padStart(2, '0')
const toUTC = (day: string) => new Date(`${day}T00:00:00Z`)
const fromUTC = (d: Date) => `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}`

export function today(): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: SCHOOL_TZ, year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date())
}

export function addDays(day: string, n: number): string {
  const d = toUTC(day)
  d.setUTCDate(d.getUTCDate() + n)
  return fromUTC(d)
}

/** 0 = Monday … 6 = Sunday */
export const weekday = (day: string) => (toUTC(day).getUTCDay() + 6) % 7
export const monday = (day: string) => addDays(day, -weekday(day))

/** ISO week as the API wants it: "2026-W46". */
export function isoWeek(day: string): string {
  const d = toUTC(day)
  d.setUTCDate(d.getUTCDate() + 3 - weekday(day)) // Thursday decides the year
  const year = d.getUTCFullYear()
  const week = Math.floor((d.getTime() - Date.UTC(year, 0, 4)) / 86400000 / 7 + 0.5) + 1
  return `${year}-W${pad(week)}`
}

export const weekNumber = (day: string) => Number(isoWeek(day).slice(-2))

/** The first school day to show: today, or Monday when it is weekend. */
export function defaultDay(): string {
  const t = today()
  return weekday(t) > 4 ? addDays(monday(t), 7) : t
}

export function formatTime(iso: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { timeZone: SCHOOL_TZ, hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(iso))
}

export function formatDay(day: string, locale: string, opts: Intl.DateTimeFormatOptions): string {
  return new Intl.DateTimeFormat(locale, { timeZone: 'UTC', ...opts }).format(toUTC(day))
}
