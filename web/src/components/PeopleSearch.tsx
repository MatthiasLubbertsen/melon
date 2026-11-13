import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2, Search, X } from 'lucide-react'
import { Avatar } from '@/components/bits'
import { api, type Person } from '@/lib/api'
import { useDebounced } from '@/lib/hooks'
import { useI18n } from '@/lib/i18n'

/** Search bar for accounts that can see other people's schedules. */
export function PeopleSearch({ accountId, onPick }: { accountId: string; onPick: (p: Person) => void }) {
  const { t } = useI18n()
  const [text, setText] = useState('')
  const [open, setOpen] = useState(false)
  const q = useDebounced(text.trim(), 180)
  const results = useQuery({
    queryKey: ['people', accountId, q],
    queryFn: () => api.people(accountId, q),
    enabled: q.length >= 2,
    staleTime: 5 * 60_000,
  })
  const show = open && text.trim().length >= 2

  const pick = (p: Person) => {
    onPick(p)
    setText('')
    setOpen(false)
  }

  return (
    <div className="relative" onBlur={(e) => !e.currentTarget.contains(e.relatedTarget) && setOpen(false)}>
      <label className="flex h-11 items-center gap-2 rounded-xl border border-input bg-card px-3 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50">
        {results.isFetching ? <Loader2 className="size-4 shrink-0 animate-spin text-muted-foreground" /> : <Search className="size-4 shrink-0 text-muted-foreground" />}
        <input
          type="search"
          value={text}
          onChange={(e) => { setText(e.target.value); setOpen(true) }}
          onFocus={() => setOpen(true)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') setOpen(false)
            if (e.key === 'Enter' && results.data?.[0]) pick(results.data[0])
          }}
          placeholder={t('searchPeople')}
          aria-label={t('searchPeople')}
          autoComplete="off"
          enterKeyHint="search"
          className="h-full min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-muted-foreground [&::-webkit-search-cancel-button]:hidden"
        />
        {text && (
          <button type="button" onClick={() => setText('')} aria-label={t('cancel')} className="-mr-1 rounded-full p-1.5 text-muted-foreground hover:bg-muted">
            <X className="size-4" />
          </button>
        )}
      </label>
      {show && results.data && (
        <ul role="listbox" className="absolute inset-x-0 top-full z-30 mt-1.5 max-h-80 overflow-y-auto rounded-xl border bg-popover p-1.5 shadow-lg">
          {results.data.length === 0 && <li className="px-3 py-3 text-sm text-muted-foreground">{t('noResults')}</li>}
          {results.data.map((p) => (
            <li key={p.code} role="option" aria-selected="false">
              <button type="button" onClick={() => pick(p)} className="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left hover:bg-accent focus-visible:bg-accent focus-visible:outline-none">
                <Avatar name={p.name} />
                <span className="grid min-w-0 leading-tight">
                  <span className="truncate text-sm font-medium">{p.name}</span>
                  <span className="truncate text-xs text-muted-foreground">
                    {t(p.type)} · {p.code}
                  </span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
