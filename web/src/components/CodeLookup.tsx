import { useState, type FormEvent } from 'react'
import { ArrowRight, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { Person } from '@/lib/api'
import { useI18n } from '@/lib/i18n'

/**
 * For teacher accounts whose Zermelo token cannot list or search people:
 * type a user code (16528, dh, ...) to open that person's schedule. Zermelo
 * only answers for people the teacher may see; otherwise the schedule shows
 * the refusal.
 */
export function CodeLookup({ onPick }: { onPick: (p: Person) => void }) {
  const { t } = useI18n()
  const [text, setText] = useState('')
  const code = text.trim().toLowerCase()
  const valid = /^[a-z0-9._-]{1,32}$/.test(code)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!valid) return
    onPick({ code, name: code, type: /^\d+$/.test(code) ? 'student' : 'employee' })
    setText('')
  }

  return (
    <form onSubmit={submit} className="flex gap-2">
      <label className="flex h-11 min-w-0 flex-1 items-center gap-2 rounded-xl border border-input bg-card px-3 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50">
        <Search className="size-4 shrink-0 text-muted-foreground" />
        <input
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={t('lookupCode')}
          aria-label={t('lookupCode')}
          autoComplete="off"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          enterKeyHint="go"
          maxLength={32}
          className="h-full min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-muted-foreground"
        />
      </label>
      <Button type="submit" size="icon" className="size-11 shrink-0" disabled={!valid} aria-label={t('lookupOpen')}>
        <ArrowRight className="size-5" />
      </Button>
    </form>
  )
}
