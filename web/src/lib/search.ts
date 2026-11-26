// The schedule search. Everything runs in the browser on schedules that are
// already loaded, so results come in as you type. A query like "huiswerk voor
// economie volgende week donderdag" is read word by word: words that mean
// something (next, homework, Thursday, week 48) become filters, the rest is
// matched against subjects, teachers and rooms, forgiving typos.

import type { Homework, Lesson } from '@/lib/api'
import { addDays, isoWeek, monday, weekday } from '@/lib/dates'

export type Tag =
  | { k: 'next' }
  | { k: 'prev' }
  | { k: 'now' }
  | { k: 'this' }
  | { k: 'hw' }
  | { k: 'plan' }
  | { k: 'det' }
  | { k: 'cancel' }
  | { k: 'change' }
  | { k: 'week' }
  | { k: 'period' }
  | { k: 'room' }
  | { k: 'rel'; n: number }
  | { k: 'wd'; d: number }
  | { k: 'stop' }

const words: [Tag, string[]][] = [
  [{ k: 'next' }, ['volgende', 'eerstvolgende', 'komende', 'aankomende', 'next', 'upcoming', 'coming']],
  [{ k: 'prev' }, ['vorige', 'laatste', 'afgelopen', 'previous', 'last', 'prev']],
  [{ k: 'now' }, ['nu', 'now', 'huidige', 'current', 'momenteel', 'right']],
  [{ k: 'this' }, ['deze', 'dit', 'this']],
  [{ k: 'hw' }, ['huiswerk', 'homework', 'hw', 'opdracht', 'opdrachten', 'maken', 'leren', 'inleveren', 'assignment', 'assignments', 'due']],
  [{ k: 'plan' }, ['weekplanning', 'planning', 'weekplan', 'itslearning', 'studiewijzer', 'plan', 'planner', 'elo', 'its']],
  [{ k: 'det' }, ['nakomen', 'detention', 'strafuur', 'nablijven']],
  [{ k: 'cancel' }, ['uitval', 'uitvallen', 'vervalt', 'vervallen', 'uitgevallen', 'valt', 'cancelled', 'canceled', 'cancel', 'geannuleerd']],
  [{ k: 'change' }, ['wijziging', 'wijzigingen', 'gewijzigd', 'veranderd', 'roosterwijziging', 'roosterwijzigingen', 'changes', 'changed', 'change']],
  [{ k: 'week' }, ['week', 'wk', 'weeknummer']],
  [{ k: 'period' }, ['uur', 'lesuur', 'hour', 'period']],
  [{ k: 'room' }, ['lokaal', 'room', 'lok', 'klaslokaal', 'classroom']],
  [{ k: 'rel', n: 0 }, ['vandaag', 'today', 'vanavond', 'tonight']],
  [{ k: 'rel', n: 1 }, ['morgen', 'tomorrow', 'tmrw', 'tmr']],
  [{ k: 'rel', n: 2 }, ['overmorgen']],
  [{ k: 'rel', n: -1 }, ['gisteren', 'yesterday']],
  [{ k: 'rel', n: -2 }, ['eergisteren']],
  [{ k: 'wd', d: 0 }, ['maandag', 'ma', 'monday', 'mon']],
  [{ k: 'wd', d: 1 }, ['dinsdag', 'di', 'tuesday', 'tue', 'tues']],
  [{ k: 'wd', d: 2 }, ['woensdag', 'wo', 'wednesday', 'wed']],
  [{ k: 'wd', d: 3 }, ['donderdag', 'do', 'thursday', 'thu', 'thur', 'thurs']],
  [{ k: 'wd', d: 4 }, ['vrijdag', 'vr', 'vrij', 'friday', 'fri']],
  [{ k: 'wd', d: 5 }, ['zaterdag', 'za', 'saturday', 'sat']],
  [{ k: 'wd', d: 6 }, ['zondag', 'zo', 'sunday', 'sun']],
  [
    { k: 'stop' },
    [
      'les', 'lessen', 'lesson', 'lessons', 'class', 'voor', 'van', 'de', 'het', 'een', 'in', 'op', 'om', 'wat', 'heb', 'ik', 'wanneer', 'is', 'the', 'for',
      'of', 'on', 'at', 'when', 'i', 'have', 'my', 'mijn', 'bij', 'met', 'with', 'welke', 'hoe', 'laat', 'waar', 'where', 'what', 'toon', 'show', 'open',
      'ga', 'naar', 'go', 'to', 'a', 'an', 'er', 'ook', 'en', 'and', 'nog', 'moet', 'me', 'hebben', 'krijg', 'heeft', 'wie', 'who', 'geef', 'find', 'zoek',
      'search', 'hoeveel', 'staat', 'zijn', 'was', 'had', 'je', 'jij', 'do', 'does', 'did', 'which', 'whats', 'wats', 'gaat', 'over', 'about', 'tot', 'until',
    ],
  ],
]

/** Subjects schools teach, to tell "not on your schedule" apart from "no idea what you mean". */
const knownSubjects = [
  'wiskunde', 'engels', 'nederlands', 'duits', 'frans', 'spaans', 'latijn', 'grieks', 'economie', 'bedrijfseconomie', 'geschiedenis', 'aardrijkskunde',
  'biologie', 'scheikunde', 'natuurkunde', 'nask', 'informatica', 'muziek', 'tekenen', 'gym', 'maatschappijleer', 'maatschappijwetenschappen', 'filosofie',
  'kunst', 'ckv', 'drama', 'techniek', 'levensbeschouwing', 'godsdienst', 'chinees', 'arabisch', 'turks', 'rekenen', 'mentoruur', 'lichamelijke',
  'maths', 'math', 'mathematics', 'english', 'dutch', 'german', 'french', 'spanish', 'latin', 'greek', 'economics', 'history', 'geography', 'biology',
  'chemistry', 'physics', 'science', 'computing', 'music', 'art', 'drawing', 'philosophy', 'religion', 'chinese',
]

const vocab = new Map<string, Tag>()
for (const [tag, list] of words) for (const w of list) if (!vocab.has(w)) vocab.set(w, tag)
// In English, "do" is a verb ("when do I have…"); in Dutch, it is Thursday.
const dutchOnly = new Set(['do', 'ma', 'di', 'wo', 'vr', 'za', 'zo'])
// Fuzzy matching only for real words, so "ec" does not turn into "en".
const fuzzyVocab = [...vocab.keys()].filter((w) => w.length >= 4)

/** Lowercase, without accents: "Économie" and "economie" are the same word. */
export const fold = (s: string) =>
  s
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')

const tokenize = (s: string) => fold(s).split(/[^a-z0-9]+/).filter(Boolean)

/** Optimal string alignment distance (Levenshtein plus swapped letters), stopping early past max. */
export function distance(a: string, b: string, max = 3): number {
  if (a === b) return 0
  if (Math.abs(a.length - b.length) > max) return max + 1
  let prev2: number[] = []
  let prev = Array.from({ length: b.length + 1 }, (_, j) => j)
  for (let i = 1; i <= a.length; i++) {
    const cur = [i]
    let best = i
    for (let j = 1; j <= b.length; j++) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1
      let v = Math.min(prev[j] + 1, cur[j - 1] + 1, prev[j - 1] + cost)
      if (i > 1 && j > 1 && a[i - 1] === b[j - 2] && a[i - 2] === b[j - 1]) v = Math.min(v, prev2[j - 2] + 1)
      cur.push(v)
      if (v < best) best = v
    }
    if (best > max) return max + 1
    prev2 = prev
    prev = cur
  }
  return prev[b.length]
}

/** How many typos a word of this length may have. */
const allowed = (n: number) => (n <= 3 ? 0 : n <= 5 ? 1 : n <= 8 ? 2 : 3)

/** How well a typed word matches a word we know: 1 is exact, 0 is no match. */
function wordScore(token: string, word: string): number {
  if (token === word) return 1
  if (token.length >= 3 && word.startsWith(token)) return 0.92 - (word.length - token.length) * 0.01
  const max = allowed(Math.min(token.length, word.length))
  if (max === 0) return 0
  const d = distance(token, word, max)
  if (d <= max) return 0.85 - d * 0.1
  // A typo in a prefix: "ecno" for "economie".
  if (token.length >= 4 && word.length > token.length) {
    const dp = distance(token, word.slice(0, token.length), 1)
    if (dp <= 1) return 0.7
  }
  return 0
}

export type Named = { code: string; name: string }

/** Everything the parser can match words against, taken from the loaded lessons. */
export type Index = { subjects: Named[]; teachers: Named[]; rooms: string[] }

export function buildIndex(lessons: Lesson[]): Index {
  const subjects = new Map<string, Named>()
  const teachers = new Map<string, Named>()
  const rooms = new Set<string>()
  for (const l of lessons) {
    if (l.type === 'detention') continue
    for (const s of l.subjects) if (!subjects.has(s.code.toLowerCase())) subjects.set(s.code.toLowerCase(), { code: s.code.toLowerCase(), name: s.name })
    for (const t of l.teachers) if (!teachers.has(t.code.toLowerCase())) teachers.set(t.code.toLowerCase(), { code: t.code.toLowerCase(), name: t.name })
    for (const r of l.locations) rooms.add(fold(r))
  }
  return { subjects: [...subjects.values()], teachers: [...teachers.values()], rooms: [...rooms] }
}

/** The best match of one word against a subject or teacher: its code exactly, or a word of its name. */
function namedScore(token: string, n: Named): number {
  if (token === n.code) return token.length >= 2 ? 1 : 0
  let best = 0
  for (const w of tokenize(n.name)) best = Math.max(best, wordScore(token, w))
  // The whole name typed as one word: "lichamelijkeopvoeding".
  const whole = tokenize(n.name).join('')
  if (whole !== n.name && token.length >= 6) best = Math.max(best, wordScore(token, whole))
  return best
}

export type Parsed = {
  /** What the query is after. */
  intent: 'lesson' | 'hw' | 'plan' | 'det' | 'cancel' | 'change'
  when?: 'next' | 'prev' | 'now'
  subjects: Named[]
  teachers: Named[]
  room?: string
  period?: number
  /** A specific day. */
  date?: string
  /** A whole ISO week ("2026-W48") when no day was named. */
  week?: string
  /** Words that matched nothing; searched for in homework and notes. */
  rest: string[]
  /** A subject that was typed but is not on the schedule, like "wiskunde" for someone without maths. */
  missing?: string
  /** Weeks this query needs loaded. */
  weeks: string[]
  /** What was understood, for showing as chips. */
  chips: Chip[]
}

export type Chip =
  | { k: 'subject'; name: string }
  | { k: 'teacher'; name: string }
  | { k: 'room'; name: string }
  | { k: 'intent'; intent: Parsed['intent'] }
  | { k: 'when'; when: 'next' | 'prev' | 'now' }
  | { k: 'date'; date: string }
  | { k: 'week'; week: string }
  | { k: 'period'; n: number }
  | { k: 'text'; text: string }

/** Monday of ISO week n, the one nearest to today (so week 2 in December is next January). */
function weekMonday(n: number, today: string): string {
  const year = Number(today.slice(0, 4))
  let best = ''
  let bestGap = Infinity
  for (const y of [year - 1, year, year + 1]) {
    const jan4 = `${y}-01-04`
    const m = addDays(monday(jan4), (n - 1) * 7)
    if (isoWeek(m) !== `${y}-W${String(n).padStart(2, '0')}`) continue
    // Prefer the coming half year over the past one.
    const gap = (Date.parse(m) - Date.parse(monday(today))) / 86400000
    const cost = gap >= -7 * 20 ? Math.abs(gap) : Math.abs(gap) * 3
    if (cost < bestGap) {
      bestGap = cost
      best = m
    }
  }
  return best
}

type Tok = { raw: string; tag?: Tag; num?: number; periodNum?: number; weekNum?: number }

export function parse(query: string, index: Index, today: string, lang: 'nl' | 'en'): Parsed {
  const toks: Tok[] = []
  for (const raw of tokenize(query)) {
    const t: Tok = { raw }
    let m: RegExpMatchArray | null
    if ((m = raw.match(/^w(?:k|eek)?(\d{1,2})$/))) t.weekNum = Number(m[1])
    else if ((m = raw.match(/^(\d{1,2})(?:e|de|ste|st|nd|rd|th)$/))) t.periodNum = Number(m[1])
    else if (/^\d{1,3}$/.test(raw)) t.num = Number(raw)
    else {
      const exact = vocab.get(raw)
      if (exact) t.tag = lang === 'en' && dutchOnly.has(raw) ? { k: 'stop' } : exact
    }
    toks.push(t)
  }

  // Words that are not exactly a keyword: a subject, teacher or room, or a keyword with a typo.
  const subjectScores = new Map<string, number>()
  const teacherScores = new Map<string, number>()
  let room: string | undefined
  const rest: string[] = []
  const leftovers: Tok[] = []
  for (const t of toks) {
    if (t.tag || t.weekNum !== undefined || t.periodNum !== undefined) continue
    if (index.rooms.includes(t.raw) && (t.num !== undefined || /\d/.test(t.raw))) {
      room = t.raw
      t.tag = { k: 'stop' }
      continue
    }
    if (t.num !== undefined) continue
    let bestSubject = 0
    let bestSubjectCode = ''
    for (const s of index.subjects) {
      const v = namedScore(t.raw, s)
      if (v > 0) subjectScores.set(s.code, Math.max(subjectScores.get(s.code) ?? 0, v))
      if (v > bestSubject) {
        bestSubject = v
        bestSubjectCode = s.code
      }
    }
    let bestTeacher = 0
    for (const p of index.teachers) {
      const v = t.raw.length >= 3 || t.raw === p.code ? namedScore(t.raw, p) : 0
      if (v > bestTeacher) bestTeacher = v
      if (v >= 0.7) teacherScores.set(p.code, Math.max(teacherScores.get(p.code) ?? 0, v))
    }
    let bestWord = 0
    let bestTag: Tag | undefined
    if (t.raw.length >= 4) {
      for (const w of fuzzyVocab) {
        // Typos and half-typed words: "volgnde", "volg".
        const v = wordScore(t.raw, w)
        if (v > 0) {
          if (v > bestWord) {
            bestWord = v
            bestTag = vocab.get(w)
          }
        }
      }
    }
    // A keyword with a typo only wins when it is clearly closer than any name.
    if (bestTag && bestWord > Math.max(bestSubject, bestTeacher) + 0.02) {
      t.tag = bestTag
      if (bestSubjectCode) subjectScores.delete(bestSubjectCode)
      continue
    }
    if (Math.max(bestSubject, bestTeacher) < 0.6) {
      // The start of a room: "gym" for gym1.
      const r = t.raw.length >= 2 && index.rooms.find((x) => x.startsWith(t.raw))
      if (r) {
        room = r
        t.tag = { k: 'stop' }
      } else if (t.raw.length >= 2) leftovers.push(t)
      continue
    }
    t.tag = { k: 'stop' }
  }

  // Keep only the subjects that matched about as well as the best one.
  const pick = (scores: Map<string, number>, all: Named[]) => {
    const top = Math.max(0, ...scores.values())
    return all.filter((n) => (scores.get(n.code) ?? 0) >= Math.max(0.6, top - 0.12)).sort((a, b) => (scores.get(b.code) ?? 0) - (scores.get(a.code) ?? 0))
  }
  let subjects = pick(subjectScores, index.subjects)
  let teachers = pick(teacherScores, index.teachers)
  // A teacher that is also a subject word ("ne" for Nederlands): subjects win.
  if (subjects.length && teachers.length && Math.max(...subjectScores.values()) >= Math.max(...teacherScores.values())) teachers = []

  // Two-letter stop words that are also a subject code, like "en": only a subject when nothing else is.
  if (!subjects.length && !teachers.length) {
    for (const t of toks) {
      const s = t.tag?.k === 'stop' && index.subjects.find((x) => x.code === t.raw && t.raw.length >= 2)
      if (s) {
        subjects = [s]
        break
      }
    }
  }
  let missing: string | undefined
  for (const t of leftovers) {
    const known = t.raw.length >= 4 && knownSubjects.find((w) => wordScore(t.raw, w) >= 0.7)
    if (known && !subjects.length) missing = known
    else rest.push(t.raw)
  }

  // Now read the tags in order.
  const intents = new Set<string>()
  let when: Parsed['when']
  let rel: number | undefined
  let wd: number | undefined
  let weekShift: number | undefined
  let weekNum: number | undefined
  let period: number | undefined
  let wantRoom = false
  for (let i = 0; i < toks.length; i++) {
    const t = toks[i]
    const tag = t.tag
    // "volgende week", "vorige week", "deze week": the modifier is about the week, not the lesson.
    const nextTag = toks.slice(i + 1).find((x) => x.tag?.k !== 'stop')
    if (t.weekNum !== undefined) weekNum = t.weekNum
    else if (t.periodNum !== undefined) period = t.periodNum
    else if (t.num !== undefined) {
      const prevTag = toks[i - 1]?.tag?.k
      const nextK = toks[i + 1]?.tag?.k
      if (prevTag === 'week') weekNum = t.num
      else if (prevTag === 'period' || nextK === 'period') period = t.num
      else if (wantRoom) room = String(t.num)
      else if (t.num >= 1 && t.num <= 53) {
        // A bare number: a period with a day ("morgen 3"), a week otherwise ("weekplanning 48").
        if (t.num <= 9 && (toks.some((x) => x.tag?.k === 'rel' || x.tag?.k === 'wd') || toks.some((x) => x.tag?.k === 'period'))) period = t.num
        else weekNum = t.num
      }
    }
    if (!tag) continue
    switch (tag.k) {
      case 'next':
      case 'prev':
      case 'this':
        if (nextTag?.tag?.k === 'week') {
          weekShift = tag.k === 'next' ? 1 : tag.k === 'prev' ? -1 : 0
          nextTag.tag = { k: 'stop' }
        } else if (tag.k !== 'this') when = tag.k
        break
      case 'now':
        when = 'now'
        break
      case 'hw':
      case 'plan':
      case 'det':
      case 'cancel':
      case 'change':
        intents.add(tag.k)
        break
      case 'rel':
        rel = tag.n
        break
      case 'wd':
        wd = tag.d
        break
      case 'room':
        wantRoom = true
        break
      case 'week':
        // A week word on its own: "week 48" handled by the number, "deze week" by the modifier.
        if (weekShift === undefined && weekNum === undefined && !toks.slice(i + 1).some((x) => x.num !== undefined)) weekShift = 0
        break
    }
  }
  // "lokaal 112" where 112 is not a room in the loaded weeks yet.
  if (wantRoom && !room) {
    const r = toks.find((x) => x.num !== undefined || leftovers.includes(x))
    if (r) {
      room = r.raw
      const k = rest.indexOf(r.raw)
      if (k >= 0) rest.splice(k, 1)
      if (weekNum === r.num) weekNum = undefined
    }
  }
  // Order of importance when several were typed.
  const intent: Parsed['intent'] = intents.has('plan') ? 'plan' : intents.has('det') ? 'det' : intents.has('hw') ? 'hw' : intents.has('cancel') ? 'cancel' : intents.has('change') ? 'change' : 'lesson'

  // Dates.
  const thisMonday = monday(today)
  let date: string | undefined
  let week: string | undefined
  const baseMonday = weekNum !== undefined ? weekMonday(weekNum, today) : weekShift !== undefined ? addDays(thisMonday, 7 * weekShift) : undefined
  if (rel !== undefined) date = addDays(today, rel)
  else if (wd !== undefined) {
    if (baseMonday) date = addDays(baseMonday, wd)
    else if (when === 'prev') {
      // "vorige donderdag": the last one before today.
      date = addDays(thisMonday, wd)
      if (date >= today) date = addDays(date, -7)
      when = undefined
    } else {
      // The coming one, today included; "volgende donderdag" on a Thursday is next week's.
      date = addDays(thisMonday, wd)
      if (date < today || (date === today && when === 'next')) date = addDays(date, 7)
      if (when === 'next') when = undefined
    }
  } else if (baseMonday) week = isoWeek(baseMonday)
  if (date && when === 'now') when = undefined

  const weeks = new Set<string>([isoWeek(today)])
  if (date) weeks.add(isoWeek(date))
  if (week) weeks.add(week)

  const chips: Chip[] = []
  if (intent !== 'lesson') chips.push({ k: 'intent', intent })
  for (const s of subjects.slice(0, 2)) chips.push({ k: 'subject', name: s.name })
  for (const p of teachers.slice(0, 2)) chips.push({ k: 'teacher', name: p.name })
  if (room) chips.push({ k: 'room', name: room })
  if (when) chips.push({ k: 'when', when })
  if (date) chips.push({ k: 'date', date })
  if (week) chips.push({ k: 'week', week })
  if (period !== undefined) chips.push({ k: 'period', n: period })
  for (const r of rest) chips.push({ k: 'text', text: r })

  if (missing) chips.push({ k: 'subject', name: missing[0].toUpperCase() + missing.slice(1) })
  return { intent, when, subjects, teachers, room, period, date, week, rest, missing, weeks: [...weeks], chips }
}

export type Result =
  | { key: string; kind: 'lesson'; lesson: Lesson; open: boolean; why?: 'next' | 'prev' | 'now' }
  | { key: string; kind: 'homework'; homework: Homework; lesson?: Lesson }
  | { key: string; kind: 'plan'; lesson: Lesson; url: string }
  | { key: string; kind: 'noPlan'; lesson: Lesson }
  | { key: string; kind: 'day'; date: string }
  | { key: string; kind: 'week'; week: string }
  | { key: string; kind: 'missing'; name: string }

const MAX = 7

/**
 * Turns a parsed query into results, best first. The first one is what Enter
 * does: focus a lesson, open it, open a week plan, or jump to a day.
 */
export function search(p: Parsed, lessons: Lesson[], homework: Homework[], now: number, today: string): Result[] {
  const codes = new Set(p.subjects.map((s) => s.code))
  const teacherCodes = new Set(p.teachers.map((t) => t.code))
  const startOf = (l: Lesson) => Date.parse(l.start)
  const endOf = (l: Lesson) => Date.parse(l.end)
  const sorted = [...lessons].sort((a, b) => startOf(a) - startOf(b))
  const filtered = p.subjects.length > 0 || p.teachers.length > 0 || p.room !== undefined || p.period !== undefined
  const matches = (l: Lesson) =>
    (!codes.size || l.subjects.some((s) => codes.has(s.code.toLowerCase()))) &&
    (!teacherCodes.size || l.teachers.some((t) => teacherCodes.has(t.code.toLowerCase()))) &&
    (p.room === undefined || l.locations.some((r) => fold(r) === p.room || fold(r).includes(p.room!))) &&
    (p.period === undefined || (l.period !== null && l.period.from <= p.period && p.period <= l.period.to))
  const inTime = (l: Lesson) => (p.date ? l.date === p.date : p.week ? isoWeek(l.date) === p.week : true)
  const out: Result[] = []
  const seen = new Set<string>()
  const push = (r: Result) => {
    if (seen.has(r.key) || out.length >= MAX) return
    seen.add(r.key)
    out.push(r)
  }
  const asLesson = (l: Lesson, open: boolean, why?: 'next' | 'prev' | 'now') => push({ key: `l-${l.id}`, kind: 'lesson', lesson: l, open, why })
  const regular = (l: Lesson) => l.type !== 'detention'

  /** Lessons in the asked time, or coming up (and still going) when no time was named, real ones before cancelled ones. */
  const pickLessons = (pred: (l: Lesson) => boolean): { list: Lesson[]; why?: 'next' | 'prev' | 'now' } => {
    const base = sorted.filter(pred)
    if (p.date || p.week) {
      const at = base.filter(inTime)
      if (p.when === 'prev') return { list: at.reverse() }
      return { list: at }
    }
    if (p.when === 'prev') return { list: base.filter((l) => endOf(l) <= now).reverse(), why: 'prev' }
    if (p.when === 'now') {
      const current = base.filter((l) => startOf(l) <= now && now < endOf(l))
      return { list: [...current, ...base.filter((l) => startOf(l) > now)], why: current.length ? 'now' : 'next' }
    }
    const coming = base.filter((l) => endOf(l) > now)
    return { list: [...coming.filter((l) => l.status !== 'cancelled'), ...coming.filter((l) => l.status === 'cancelled')], why: 'next' }
  }
  const textHit = (s: string) => p.rest.length > 0 && p.rest.every((w) => fold(s).includes(w) || tokenize(s).some((x) => wordScore(w, x) >= 0.7))

  // Homework items, open first, by lesson time.
  const hwItems = homework
    .filter((h) => h.lesson && !h.done)
    .filter((h) => !codes.size || h.lesson!.subjects.some((c) => codes.has(c)))
    .filter((h) => (p.date ? h.lesson!.date === p.date : p.week ? isoWeek(h.lesson!.date) === p.week : p.when === 'prev' ? true : h.lesson!.date >= today || h.lesson!.status === 'cancelled' || h.lesson!.status === 'gone'))
    .sort((a, b) => Date.parse(a.lesson!.start) - Date.parse(b.lesson!.start))
  const hwFor = (h: Homework) => lessons.find((l) => l.instance === h.lesson!.instance && l.date === h.lesson!.date)
  const pushHomework = (list: Homework[]) => list.forEach((h) => push({ key: `h-${h.id}`, kind: 'homework', homework: h, lesson: hwFor(h) }))

  // A subject that is not on the schedule: say so, instead of listing everything else.
  if (p.missing && !filtered) return [{ key: `m-${p.missing}`, kind: 'missing', name: p.missing }]

  switch (p.intent) {
    case 'plan': {
      const week = p.week ?? (p.date ? isoWeek(p.date) : isoWeek(weekday(today) > 4 ? addDays(monday(today), 7) : today))
      const ofWeek = sorted.filter((l) => isoWeek(l.date) === week && regular(l) && matches(l))
      const bySubject = new Map<string, Lesson>()
      for (const l of ofWeek) {
        const code = l.subjects[0]?.code.toLowerCase() ?? l.id
        const had = bySubject.get(code)
        if (!had || (!had.weekPlan && l.weekPlan)) bySubject.set(code, l)
      }
      for (const l of bySubject.values()) if (l.weekPlan) push({ key: `p-${l.subjects[0]?.code}-${week}`, kind: 'plan', lesson: l, url: l.weekPlan })
      // A subject without a plan that week: open a lesson to add one.
      if (codes.size) for (const l of bySubject.values()) if (!l.weekPlan) push({ key: `np-${l.subjects[0]?.code}`, kind: 'noPlan', lesson: l })
      if (!out.length) push({ key: `w-${week}`, kind: 'week', week })
      break
    }
    case 'hw': {
      let { list } = pickLessons((l) => regular(l) && matches(l))
      // "huiswerk economie donderdag" when economie is not on Thursday: the lessons of it nearest that day.
      if (!list.length && p.date && filtered) {
        const at = Date.parse(p.date)
        list = sorted
          .filter((l) => regular(l) && matches(l) && Math.abs(Date.parse(l.date) - at) <= 7 * 86400000)
          .sort((a, b) => Math.abs(Date.parse(a.date) - at) - Math.abs(Date.parse(b.date) - at) || (a.date < b.date ? 1 : -1))
      }
      if (p.date || p.week) {
        // The lesson it is for first: that is where homework is written down.
        if (filtered) list.slice(0, 2).forEach((l) => asLesson(l, true))
        pushHomework(hwItems.filter((h) => !p.rest.length || textHit(h.text)))
        if (!filtered) list.filter((l) => l.homework.length).forEach((l) => asLesson(l, true))
      } else {
        pushHomework(hwItems.filter((h) => !p.rest.length || textHit(h.text)))
        if (filtered) list.slice(0, 2).forEach((l, i) => asLesson(l, true, i === 0 ? 'next' : undefined))
      }
      break
    }
    case 'det': {
      const { list } = pickLessons((l) => l.type === 'detention' && (!codes.size || matches(l)))
      list.forEach((l) => asLesson(l, false))
      break
    }
    case 'cancel':
    case 'change': {
      const want = p.intent === 'cancel' ? (l: Lesson) => l.status === 'cancelled' : (l: Lesson) => l.status !== 'scheduled' || l.type === 'detention'
      const { list } = pickLessons((l) => want(l) && matches(l))
      list.forEach((l) => asLesson(l, false))
      break
    }
    default: {
      if (!filtered && (p.date || p.week) && !p.rest.length) {
        if (p.date) push({ key: `d-${p.date}`, kind: 'day', date: p.date })
        else push({ key: `w-${p.week}`, kind: 'week', week: p.week! })
        if (p.date) sorted.filter((l) => l.date === p.date).forEach((l) => asLesson(l, false))
        break
      }
      if (filtered || p.when) {
        const { list, why } = pickLessons((l) => matches(l))
        list.slice(0, MAX).forEach((l, i) => asLesson(l, false, i === 0 ? why : undefined))
        // Nothing that day: the nearest one after it.
        if (!list.length && (p.date || p.week) && filtered) {
          const from = p.date ?? monday(`${p.week!.slice(0, 4)}-01-04`)
          sorted.filter((l) => matches(l) && l.date >= from).slice(0, 3).forEach((l) => asLesson(l, false))
        }
      }
    }
  }

  // Words nothing else understood: look in homework, lesson titles and notes.
  if (p.rest.length && out.length < MAX) {
    pushHomework(homework.filter((h) => h.lesson && !h.done && textHit(h.text)))
    sorted
      .filter((l) => endOf(l) > now - 7 * 86400000 && inTime(l) && matches(l) && (textHit(l.title) || textHit(l.note) || textHit(l.changeNote) || l.homework.some((h) => textHit(h.text))))
      .slice(0, 4)
      .forEach((l) => asLesson(l, l.homework.some((h) => textHit(h.text))))
  }
  return out
}
