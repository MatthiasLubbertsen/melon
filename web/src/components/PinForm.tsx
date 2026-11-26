import { useState, type ReactNode } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { DialogFooter } from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { PinInput } from '@/components/SignInForm'
import { api } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'

/**
 * Picks the account whose number signs in and the PIN that goes with it.
 * Meant for inside a dialog: the header comes in as children, the footer
 * holds Save and whatever `secondary` adds next to it.
 */
export function PinForm({ onSaved, secondary, children }: { onSaved: () => void; secondary?: ReactNode; children?: ReactNode }) {
  const { t, error } = useI18n()
  const { session, accounts, reload } = useSession()
  const login = session.login
  const [accountId, setAccountId] = useState(() => {
    const current = login && accounts.find((a) => a.userCode === login.userId && a.school === login.school)
    return (current || accounts[0]).id
  })
  const [pin, setPin] = useState('')
  const save = useMutation({
    mutationFn: () => api.setPin(accountId, pin),
    onSuccess: async (l) => {
      onSaved()
      await reload()
      toast.success(t('pinSaved', { number: l.userId }))
    },
  })

  return (
    <form
      className="grid gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (pin.length === 4 && !save.isPending) save.mutate()
      }}
    >
      {children}
      {accounts.length > 1 && (
        <fieldset className="grid gap-1">
          <legend className="mb-1.5 text-sm font-medium">{t('loginAccount')}</legend>
          {accounts.map((a) => (
            <Label key={a.id} className={cn('flex min-h-11 items-center gap-3 rounded-lg border px-3 font-normal has-focus-visible:ring-3 has-focus-visible:ring-ring/50', a.id === accountId && 'border-honeydew-ink/40 bg-honeydew-tint font-medium text-honeydew-ink')}>
              <input type="radio" name="login-account" className="sr-only" checked={a.id === accountId} onChange={() => setAccountId(a.id)} />
              <span className="truncate">{a.name}</span>
              <span className={cn('ml-auto shrink-0 font-mono text-sm', a.id !== accountId && 'text-muted-foreground')}>{a.userCode}</span>
            </Label>
          ))}
        </fieldset>
      )}
      <div className="grid gap-1.5">
        <Label htmlFor="new-pin">{t('pin')}</Label>
        <PinInput id="new-pin" value={pin} onChange={setPin} autoComplete="new-password" autoFocus aria-describedby="new-pin-hint" aria-invalid={save.isError} />
        <p id="new-pin-hint" className={save.isError ? 'text-sm text-destructive' : 'text-sm text-muted-foreground'}>
          {save.isError ? error(save.error) : t('pinHint')}
        </p>
      </div>
      <DialogFooter>
        {secondary}
        <Button type="submit" className="h-11" disabled={pin.length !== 4 || save.isPending}>
          {save.isPending && <Loader2 className="animate-spin" />}
          {t('save')}
        </Button>
      </DialogFooter>
    </form>
  )
}
