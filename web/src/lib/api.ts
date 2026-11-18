// Thin client for Melon's own API. The web app uses the same public REST API
// as everyone else (with its session cookie), plus /api/app for management.

export type Account = {
  id: string
  school: string
  name: string
  role: 'student' | 'employee' | 'parent' | 'other'
  userCode: string
  canViewOthers: boolean
}

export type Person = { code: string; name: string; type: 'student' | 'employee' }

export type Lesson = {
  id: string
  date: string
  start: string
  end: string
  period: { from: number; to: number } | null
  type: string
  title: string
  subjects: { code: string; name: string }[]
  teachers: { code: string; name: string }[]
  locations: string[]
  groups: string[]
  status: 'scheduled' | 'changed' | 'cancelled'
  changes: ('teacher' | 'location' | 'group' | 'time')[]
  changeNote: string
  note: string
  online: boolean
  onlineUrl?: string
  optional: boolean
}

export type Schedule = {
  person: Person
  from: string
  to: string
  lessons: Lesson[]
  summary: { lessons: number; changed: number; cancelled: number }
}

export type Announcement = { id: string; title: string; text: string; start: string; end: string }
export type SubjectEntry = { code: string; name: string; defaultName: string; custom: boolean }
export type TeacherEntry = { code: string; name: string; custom: boolean }
export type ApiKey = { id: string; name: string; hint: string; accountIds: string[]; createdAt: string; lastUsedAt: string | null; admin: boolean; key?: string }
export type Grant = { id: string; app: string; accountIds: string[]; createdAt: string; lastUsedAt: string | null; admin: boolean }
export type Calendar = { enabled: boolean; url?: string }
/** How this profile signs in on other devices: a Zermelo user number, plus a PIN the server keeps. */
export type Login = { userId: string; school: string }
export type Session = {
  demo: boolean
  version: string
  defaultSchool: string
  accounts: Account[]
  login: Login | null
  /** This profile may open /admin. */
  admin: boolean
  /** An admin is looking at Melon as this profile. */
  impersonating: boolean
}

// ---- admin ----

export type AdminStats = {
  profiles: number
  profilesNew7d: number
  withPin: number
  lockedPins: number
  accounts: number
  students: number
  staff: number
  parents: number
  schools: number
  active1d: number
  active7d: number
  active30d: number
  sessions: number
  apiKeys: number
  oauthApps: number
  calendars: number
  subjectNames: number
  teacherNames: number
}
export type UsageChannel = 'session' | 'api_key' | 'oauth' | 'calendar' | 'admin'
export type Usage = { day: string; channel: UsageChannel; requests: number; lessons: number }
export type SchoolStat = { school: string; profiles: number; accounts: number; students: number; staff: number; teacherNames: number }
export type AdminOverview = { stats: AdminStats; totals: { requests: number; lessons: number }; since: string; today: string; usage: Usage[]; schools: SchoolStat[] }
export type AdminAccount = Account & { calendar: boolean; createdAt: string; tokenExpires: string | null }
export type AdminProfile = { id: string; createdAt: string; lastSeenAt: string | null; login: Login | null; accounts: AdminAccount[]; keys: number; admin: boolean; self: boolean; pin: boolean }
export type AdminProfileDetail = {
  profile: AdminProfile
  pinLock: { fails: number; lockedUntil: string | null } | null
  sessions: number
  keys: ApiKey[]
  grants: Grant[]
  names: { subjects: number; teachers: number }
}
export type AdminSubject = { code: string; nl: string; en: string; builtinNl: string; builtinEn: string; global: boolean }
export type Suggestion = { school?: string; code: string; name: string; profiles: number; schools?: string[]; current: string }
export type AccountCheck = {
  account: Account
  profileId: string
  tokenWorks: boolean
  problem?: string
  tokenExpires: string | null
  zermeloUser?: string
  zermeloName?: string
  sameUser: boolean
  canListUsers: boolean
  usersVisible: number
  storedAsWide: boolean
  checkedAt: string
}
export type LogEntry = { id: number; at: string; admin: string; adminName: string; action: string; target: string; detail: string }
export type Meta = { fetchedAt?: string; stale?: boolean }

export class ApiError extends Error {
  code: string
  status: number
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<{ data: T; meta: Meta }> {
  let res: Response
  try {
    res = await fetch(path, {
      method,
      credentials: 'same-origin',
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    throw new ApiError(0, 'offline', 'offline')
  }
  if (res.status === 204) return { data: undefined as T, meta: {} }
  const json = await res.json().catch(() => null)
  if (!res.ok || !json) {
    throw new ApiError(res.status, json?.error?.code ?? 'internal_error', json?.error?.message ?? res.statusText)
  }
  return { data: json.data as T, meta: json.meta ?? {} }
}

const data = <T,>(p: Promise<{ data: T }>) => p.then((r) => r.data)
const qs = (params: Record<string, string | boolean | undefined>) => {
  const u = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '' && v !== false) u.set(k, String(v))
  const s = u.toString()
  return s ? `?${s}` : ''
}

export const api = {
  session: () => data(request<Session>('GET', '/api/app/session')),
  forget: () => data(request<void>('DELETE', '/api/app/session')),
  signIn: (userId: string, pin: string) => data(request<Session>('POST', '/api/app/login', { userId, pin })),
  signOut: () => data(request<void>('POST', '/api/app/logout', {})),
  setPin: (accountId: string, pin: string) => data(request<Login>('PUT', '/api/app/pin', { accountId, pin })),
  clearPin: () => data(request<void>('DELETE', '/api/app/pin')),
  link: (body: { school: string; code?: string; token?: string }) => data(request<Account>('POST', '/api/app/accounts', body)),
  demo: () => data(request<Account[]>('POST', '/api/app/demo', {})),
  unlink: (id: string) => data(request<void>('DELETE', `/api/app/accounts/${id}`)),

  schedule: (account: string, p: { week: string; person?: string; lang: string; fresh?: boolean }) =>
    request<Schedule>('GET', `/api/v1/accounts/${account}/schedule${qs(p)}`),
  people: (account: string, q: string) => data(request<Person[]>('GET', `/api/v1/accounts/${account}/people${qs({ q })}`)),
  announcements: (account: string) => data(request<Announcement[]>('GET', `/api/v1/accounts/${account}/announcements`)),
  subjects: (account: string, lang: string) => data(request<SubjectEntry[]>('GET', `/api/v1/accounts/${account}/subjects${qs({ lang })}`)),
  setSubject: (account: string, code: string, name: string) =>
    data(request<SubjectEntry>('PUT', `/api/v1/accounts/${account}/subjects/${encodeURIComponent(code)}`, { name })),
  resetSubject: (account: string, code: string) =>
    data(request<void>('DELETE', `/api/v1/accounts/${account}/subjects/${encodeURIComponent(code)}`)),

  teachers: (account: string) => data(request<TeacherEntry[]>('GET', `/api/v1/accounts/${account}/teachers`)),
  setTeacher: (account: string, code: string, name: string) =>
    data(request<TeacherEntry>('PUT', `/api/v1/accounts/${account}/teachers/${encodeURIComponent(code)}`, { name })),
  resetTeacher: (account: string, code: string) =>
    data(request<void>('DELETE', `/api/v1/accounts/${account}/teachers/${encodeURIComponent(code)}`)),

  calendar: (account: string) => data(request<Calendar>('GET', `/api/app/accounts/${account}/calendar`)),
  enableCalendar: (account: string) => data(request<Calendar>('POST', `/api/app/accounts/${account}/calendar`, {})),
  disableCalendar: (account: string) => data(request<Calendar>('DELETE', `/api/app/accounts/${account}/calendar`)),

  keys: () => data(request<ApiKey[]>('GET', '/api/app/keys')),
  createKey: (name: string, accountIds: string[], admin = false) => data(request<ApiKey>('POST', '/api/app/keys', { name, accountIds, admin })),
  deleteKey: (id: string) => data(request<void>('DELETE', `/api/app/keys/${id}`)),

  grants: () => data(request<Grant[]>('GET', '/api/app/grants')),
  deleteGrant: (id: string) => data(request<void>('DELETE', `/api/app/grants/${id}`)),

  admin: {
    overview: () => data(request<AdminOverview>('GET', '/api/app/admin/overview')),
    profiles: (q: string, pin?: 'yes' | 'no') => data(request<AdminProfile[]>('GET', `/api/app/admin/profiles${qs({ q, pin })}`)),
    profile: (id: string) => data(request<AdminProfileDetail>('GET', `/api/app/admin/profiles/${id}`)),
    deleteProfile: (id: string) => data(request<void>('DELETE', `/api/app/admin/profiles/${id}`)),
    impersonate: (id: string) => data(request<Session>('POST', `/api/app/admin/profiles/${id}/impersonate`, {})),
    stop: () => data(request<Session>('POST', '/api/app/admin/stop', {})),
    setPin: (id: string, accountId: string, pin: string) => data(request<Login>('PUT', `/api/app/admin/profiles/${id}/pin`, { accountId, pin })),
    clearPin: (id: string) => data(request<void>('DELETE', `/api/app/admin/profiles/${id}/pin`)),
    unlock: (id: string) => data(request<void>('POST', `/api/app/admin/profiles/${id}/unlock`, {})),
    signOut: (id: string) => data(request<void>('DELETE', `/api/app/admin/profiles/${id}/sessions`)),
    zermeloToken: (accountId: string) => data(request<{ token: string; school: string; expiresAt: string | null }>('GET', `/api/app/admin/accounts/${accountId}/token`)),
    checkAccount: (accountId: string) => data(request<AccountCheck>('POST', `/api/app/admin/accounts/${accountId}/check`, {})),
    deleteKey: (id: string) => data(request<void>('DELETE', `/api/app/admin/keys/${id}`)),
    subjects: () => data(request<AdminSubject[]>('GET', '/api/app/admin/subjects')),
    setSubject: (code: string, nl: string, en: string) => data(request<unknown>('PUT', `/api/app/admin/subjects/${encodeURIComponent(code)}`, { nl, en })),
    resetSubject: (code: string) => data(request<void>('DELETE', `/api/app/admin/subjects/${encodeURIComponent(code)}`)),
    teachers: (school: string) => data(request<{ code: string; name: string }[]>('GET', `/api/app/admin/teachers${qs({ school })}`)),
    setTeacher: (school: string, code: string, name: string) =>
      data(request<unknown>('PUT', `/api/app/admin/teachers/${encodeURIComponent(school)}/${encodeURIComponent(code)}`, { name })),
    resetTeacher: (school: string, code: string) => data(request<void>('DELETE', `/api/app/admin/teachers/${encodeURIComponent(school)}/${encodeURIComponent(code)}`)),
    suggestions: () => data(request<{ subjects: Suggestion[]; teachers: Suggestion[] }>('GET', '/api/app/admin/suggestions')),
    log: () => data(request<LogEntry[]>('GET', '/api/app/admin/log')),
  },

  oauthRequest: (clientId: string, redirectUri: string) =>
    data(request<{ app: string; returnsTo: string }>('GET', `/api/app/oauth/request${qs({ client_id: clientId, redirect_uri: redirectUri })}`)),
  oauthApprove: (body: Record<string, unknown>) => data(request<{ redirect: string }>('POST', '/api/app/oauth/approve', body)),
}
