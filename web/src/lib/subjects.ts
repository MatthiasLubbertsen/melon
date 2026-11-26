import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { isoWeek, today } from '@/lib/dates'
import { useI18n } from '@/lib/i18n'

/**
 * Subjects to pick from: the ones on this week's schedule, by name. names
 * also knows every other code the account has a name for, for chips on
 * notes and cards about a subject that is not on this week.
 */
export function useSubjects(accountId: string) {
  const { lang, locale } = useI18n()
  const week = isoWeek(today())
  const all = useQuery({ queryKey: ['subjects', accountId, lang], queryFn: () => api.subjects(accountId, lang), staleTime: 5 * 60_000 })
  const sched = useQuery({ queryKey: ['schedule', accountId, week, '', lang], queryFn: () => api.schedule(accountId, { week, lang }), staleTime: 60_000 })
  return useMemo(() => {
    const names = new Map((all.data ?? []).map((s) => [s.code.toLowerCase(), s.name]))
    const picked = new Map<string, string>()
    const seenNames = new Set<string>()
    for (const l of sched.data?.data.lessons ?? []) {
      if (l.type === 'detention') continue
      for (const s of l.subjects) {
        const code = s.code.toLowerCase()
        if (picked.has(code) || seenNames.has(s.name)) continue
        picked.set(code, s.name)
        seenNames.add(s.name)
        names.set(code, s.name)
      }
    }
    const list = [...picked].map(([code, name]) => ({ code, name })).sort((a, b) => a.name.localeCompare(b.name, locale))
    return { names, list }
  }, [all.data, sched.data, locale])
}

/** Picker options: this week's subjects, plus the one already chosen when it is not among them. */
export function subjectOptions(list: { code: string; name: string }[], names: Map<string, string>, current: string) {
  const out = list.map((s) => ({ value: s.code, label: s.name }))
  if (current && !list.some((s) => s.code === current)) out.unshift({ value: current, label: names.get(current) ?? current.toUpperCase() })
  return out
}
