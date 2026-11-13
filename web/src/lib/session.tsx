import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Account, type Session } from './api'

type SessionState = {
  session: Session
  accounts: Account[]
  /** The account being looked at; undefined only when nothing is linked. */
  active: Account | undefined
  setActive: (id: string) => void
  reload: () => Promise<unknown>
  /** Switch to another profile (after signing in or out): take its session and drop everything cached for the old one. */
  swap: (next?: Session) => Promise<void>
}

const Ctx = createContext<SessionState | null>(null)
const STORAGE = 'melon.account'

export function SessionProvider({ children, fallback }: { children: ReactNode; fallback: ReactNode }) {
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['session'], queryFn: api.session, staleTime: 60_000 })
  const [activeId, setActiveId] = useState<string | null>(() => {
    try {
      return localStorage.getItem(STORAGE)
    } catch {
      return null
    }
  })
  const setActive = useCallback((id: string) => {
    setActiveId(id)
    try {
      localStorage.setItem(STORAGE, id)
    } catch {
      /* private mode */
    }
  }, [])
  const reload = useCallback(() => qc.invalidateQueries({ queryKey: ['session'] }), [qc])

  const swap = useCallback(
    async (next?: Session) => {
      qc.setQueryData(['session'], next ?? (await api.session()))
      qc.removeQueries({ predicate: (query) => query.queryKey[0] !== 'session' })
    },
    [qc],
  )

  const value = useMemo<SessionState | null>(() => {
    if (!q.data) return null
    const accounts = q.data.accounts
    return { session: q.data, accounts, active: accounts.find((a) => a.id === activeId) ?? accounts[0], setActive, reload, swap }
  }, [q.data, activeId, setActive, reload, swap])

  if (!value) return <>{fallback}</>
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useSession(): SessionState {
  const v = useContext(Ctx)
  if (!v) throw new Error('useSession outside provider')
  return v
}
