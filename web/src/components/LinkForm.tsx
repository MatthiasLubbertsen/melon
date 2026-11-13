import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { CircleAlert, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { api, type Account } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'

/** Link a Zermelo account: school + link code, or school + API token. */
export function LinkForm({ onLinked, initialSchool = '' }: { onLinked?: (a: Account) => void; initialSchool?: string }) {
  const { t, error } = useI18n()
  const { setActive, reload } = useSession()
  const qc = useQueryClient()
  const [mode, setMode] = useState<'code' | 'token'>('code')
  const [school, setSchool] = useState(initialSchool)
  const [secret, setSecret] = useState('')
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<{ field: 'school' | 'secret' | 'form'; text: string } | null>(null)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (busy) return
    setBusy(true)
    setProblem(null)
    try {
      const account = await api.link(mode === 'code' ? { school, code: secret } : { school, token: secret })
      setSecret('')
      setActive(account.id)
      await reload()
      qc.removeQueries({ queryKey: ['schedule', account.id] })
      toast.success(t('linked', { name: account.name }))
      onLinked?.(account)
    } catch (err) {
      const code = (err as { code?: string }).code
      const field = code === 'school_not_found' ? 'school' : code === 'invalid_link_code' || code === 'invalid_token' ? 'secret' : 'form'
      setProblem({ field, text: error(err) })
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="grid gap-4" noValidate>
      <div className="grid gap-1.5">
        <Label htmlFor="school">{t('school')}</Label>
        <div className="flex h-11 items-center rounded-lg border border-input bg-background focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50 aria-invalid:border-destructive" aria-invalid={problem?.field === 'school'}>
          <input
            id="school"
            name="school"
            value={school}
            onChange={(e) => setSchool(e.target.value)}
            placeholder={t('schoolPlaceholder')}
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            autoComplete="organization"
            required
            aria-describedby="school-hint"
            className="h-full min-w-0 flex-1 bg-transparent pl-3 text-base outline-none placeholder:text-muted-foreground"
          />
          <span className="pr-3 pl-1 text-base text-muted-foreground select-none">.zportal.nl</span>
        </div>
        <p id="school-hint" className={problem?.field === 'school' ? 'text-sm text-destructive' : 'text-sm text-muted-foreground'}>
          {problem?.field === 'school' ? problem.text : t('schoolHint')}
        </p>
      </div>

      <div className="grid gap-1.5">
        <Tabs value={mode} onValueChange={(v) => { setMode(v as 'code' | 'token'); setSecret(''); setProblem(null) }}>
          <TabsList className="h-10 w-full">
            <TabsTrigger value="code">{t('withCode')}</TabsTrigger>
            <TabsTrigger value="token">{t('withToken')}</TabsTrigger>
          </TabsList>
        </Tabs>
        <Label htmlFor="secret" className="sr-only">
          {mode === 'code' ? t('code') : t('token')}
        </Label>
        <Input
          id="secret"
          name={mode}
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
          placeholder={mode === 'code' ? '123 456 789 012' : ''}
          inputMode={mode === 'code' ? 'numeric' : 'text'}
          type={mode === 'code' ? 'text' : 'password'}
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          required
          aria-invalid={problem?.field === 'secret'}
          aria-describedby="secret-hint"
          className="h-11 text-base tracking-wide"
        />
        <p id="secret-hint" className={problem?.field === 'secret' ? 'text-sm text-destructive' : 'text-sm text-muted-foreground'}>
          {problem?.field === 'secret' ? problem.text : mode === 'code' ? t('codeHint') : t('tokenHint')}
        </p>
      </div>

      {problem?.field === 'form' && (
        <p role="alert" className="flex items-center gap-2 rounded-lg bg-watermelon-tint px-3 py-2 text-sm text-watermelon-ink">
          <CircleAlert className="size-4 shrink-0" />
          {problem.text}
        </p>
      )}

      <Button type="submit" className="h-12 text-base" disabled={busy || !school.trim() || !secret.trim()}>
        {busy && <Loader2 className="animate-spin" />}
        {busy ? t('connecting') : t('connect')}
      </Button>
    </form>
  )
}

export function LinkDialog({ open, onOpenChange, initialSchool }: { open: boolean; onOpenChange: (o: boolean) => void; initialSchool?: string }) {
  const { t } = useI18n()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('linkAnother')}</DialogTitle>
        </DialogHeader>
        <LinkForm initialSchool={initialSchool} onLinked={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  )
}
