import { useState, type ComponentProps, type FormEvent } from 'react'
import { CircleAlert, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'

/** Four digits, masked, with the number keyboard on phones. */
export function PinInput({ value, onChange, className, ...props }: Omit<ComponentProps<typeof Input>, 'value' | 'onChange'> & { value: string; onChange: (pin: string) => void }) {
  return (
    <Input
      {...props}
      value={value}
      onChange={(e) => onChange(e.target.value.replace(/\D/g, '').slice(0, 4))}
      type="password"
      inputMode="numeric"
      pattern="[0-9]{4}"
      maxLength={4}
      required
      className={cn('h-11 text-base tracking-[0.5em]', className)}
    />
  )
}

/** Sign in to an existing Melon account: Zermelo user number + PIN. */
export function SignInForm() {
  const { t, error } = useI18n()
  const { swap } = useSession()
  const [number, setNumber] = useState('')
  const [pin, setPin] = useState('')
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState('')

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (busy) return
    setBusy(true)
    setProblem('')
    try {
      await swap(await api.signIn(number.trim(), pin))
    } catch (err) {
      setPin('')
      setProblem(error(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="grid gap-4" noValidate>
      <div className="grid gap-1.5">
        <Label htmlFor="user-number">{t('userNumber')}</Label>
        <Input
          id="user-number"
          name="username"
          value={number}
          onChange={(e) => setNumber(e.target.value)}
          placeholder="16528"
          autoComplete="username"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          required
          aria-describedby="user-number-hint"
          className="h-11 text-base tracking-wide"
        />
        <p id="user-number-hint" className="text-sm text-muted-foreground">
          {t('userNumberHint')}
        </p>
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="pin">{t('pin')}</Label>
        <PinInput id="pin" name="password" value={pin} onChange={setPin} autoComplete="current-password" aria-invalid={problem !== ''} />
      </div>
      {problem && (
        <p role="alert" className="flex items-center gap-2 rounded-lg bg-watermelon-tint px-3 py-2 text-sm text-watermelon-ink">
          <CircleAlert className="size-4 shrink-0" />
          {problem}
        </p>
      )}
      <Button type="submit" className="h-12 text-base" disabled={busy || !number.trim() || pin.length !== 4}>
        {busy && <Loader2 className="animate-spin" />}
        {busy ? t('signingIn') : t('signIn')}
      </Button>
    </form>
  )
}
