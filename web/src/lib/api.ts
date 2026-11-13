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
  teachers: string[]
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
export type ApiKey = { id: string; name: string; hint: string; accountIds: string[]; createdAt: string; lastUsedAt: string | null; key?: string }
export type Grant = { id: string; app: string; accountIds: string[]; createdAt: string; lastUsedAt: string | null }
export type Calendar = { enabled: boolean; url?: string }
export type Session = { demo: boolean; version: string; defaultSchool: string; accounts: Account[] }
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

  calendar: (account: string) => data(request<Calendar>('GET', `/api/app/accounts/${account}/calendar`)),
  enableCalendar: (account: string) => data(request<Calendar>('POST', `/api/app/accounts/${account}/calendar`, {})),
  disableCalendar: (account: string) => data(request<Calendar>('DELETE', `/api/app/accounts/${account}/calendar`)),

  keys: () => data(request<ApiKey[]>('GET', '/api/app/keys')),
  createKey: (name: string, accountIds: string[]) => data(request<ApiKey>('POST', '/api/app/keys', { name, accountIds })),
  deleteKey: (id: string) => data(request<void>('DELETE', `/api/app/keys/${id}`)),

  grants: () => data(request<Grant[]>('GET', '/api/app/grants')),
  deleteGrant: (id: string) => data(request<void>('DELETE', `/api/app/grants/${id}`)),

  oauthRequest: (clientId: string, redirectUri: string) =>
    data(request<{ app: string; returnsTo: string }>('GET', `/api/app/oauth/request${qs({ client_id: clientId, redirect_uri: redirectUri })}`)),
  oauthApprove: (body: Record<string, unknown>) => data(request<{ redirect: string }>('POST', '/api/app/oauth/approve', body)),
}
