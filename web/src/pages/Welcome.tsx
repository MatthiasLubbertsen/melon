import { useState } from 'react'
import { Loader2, ShieldCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Logo } from '@/components/Logo'
import { LinkForm } from '@/components/LinkForm'
import { SignInForm } from '@/components/SignInForm'
import { LanguageToggle } from '@/components/LanguageToggle'
import { api } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'

/** First screen: nothing to explain, just link your school. */
export function Welcome() {
  const { t } = useI18n()
  const { session, reload } = useSession()
  const [busy, setBusy] = useState(false)
  const [signingIn, setSigningIn] = useState(false)

  const demo = async () => {
    setBusy(true)
    try {
      await api.demo()
      await reload()
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center gap-8 px-5 py-10">
      <header className="grid gap-3">
        <Logo className="text-5xl" />
        <h1 className="text-3xl font-semibold tracking-tight">Melon</h1>
        <p className="text-lg text-muted-foreground">{t('tagline')}</p>
      </header>

      {signingIn ? (
        <section aria-labelledby="signin-title" className="grid gap-4">
          <h2 id="signin-title" className="text-lg font-medium">
            {t('signInTitle')}
          </h2>
          <SignInForm />
          <Button variant="ghost" className="h-11 text-base" onClick={() => setSigningIn(false)}>
            {t('noAccountYet')}
          </Button>
        </section>
      ) : (
        <section aria-labelledby="link-title" className="grid gap-4">
          <h2 id="link-title" className="text-lg font-medium">
            {t('linkTitle')}
          </h2>
          <LinkForm />
          {session.demo && (
            <Button variant="outline" className="h-12 text-base" onClick={demo} disabled={busy}>
              {busy && <Loader2 className="animate-spin" />}
              {t('tryDemo')}
            </Button>
          )}
          <Button variant="ghost" className="h-11 text-base" onClick={() => setSigningIn(true)}>
            {t('haveAccount')}
          </Button>
        </section>
      )}

      <footer className="flex items-center justify-between gap-4 text-sm text-muted-foreground">
        <p className="flex items-center gap-2">
          <ShieldCheck className="size-4 shrink-0 text-honeydew-ink" />
          {t('privacyNote')}
        </p>
        <LanguageToggle />
      </footer>
    </main>
  )
}
