import { useEffect, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { CircleAlert, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Avatar, Banner } from '@/components/bits'
import { LinkForm } from '@/components/LinkForm'
import { SignInForm } from '@/components/SignInForm'
import { Logo } from '@/components/Logo'
import { api } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'

/** OAuth consent: an app (usually an AI client via MCP) asks for access. */
export function Consent() {
  const { t, error } = useI18n()
  const { accounts } = useSession()
  const [params] = useSearchParams()
  const clientId = params.get('client_id') ?? ''
  const redirectUri = params.get('redirect_uri') ?? ''
  const state = params.get('state') ?? ''
  const [picked, setPicked] = useState<string[]>([])
  const [signingIn, setSigningIn] = useState(false)

  // With one account there is nothing to choose; preselect it.
  useEffect(() => {
    if (accounts.length === 1) setPicked([accounts[0].id])
  }, [accounts])

  const request = useQuery({ queryKey: ['oauth-request', clientId, redirectUri], queryFn: () => api.oauthRequest(clientId, redirectUri), retry: false })
  const approve = useMutation({
    mutationFn: () =>
      api.oauthApprove({
        clientId,
        redirectUri,
        state,
        responseType: params.get('response_type') ?? '',
        codeChallenge: params.get('code_challenge') ?? '',
        codeChallengeMethod: params.get('code_challenge_method') ?? '',
        accountIds: picked,
      }),
    onSuccess: ({ redirect }) => window.location.assign(redirect),
  })

  const deny = () => {
    // Only reached after the server confirmed this redirect URI is registered.
    const u = new URL(redirectUri)
    u.searchParams.set('error', 'access_denied')
    if (state) u.searchParams.set('state', state)
    window.location.assign(u.toString())
  }

  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center gap-6 px-5 py-10">
      <Logo className="text-4xl" />
      {request.isPending ? (
        <Loader2 className="size-6 animate-spin text-muted-foreground" />
      ) : request.isError ? (
        <Banner tone="watermelon" icon={<CircleAlert />} title={t('consentInvalid')}>
          {request.error.message}
        </Banner>
      ) : (
        <>
          <header className="grid gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">{t('consentTitle', { app: request.data.app })}</h1>
            <p className="text-muted-foreground">{t('consentReturns', { host: request.data.returnsTo })}</p>
          </header>

          {accounts.length === 0 ? (
            <section className="grid gap-4">
              <p className="font-medium">{signingIn ? t('signInTitle') : t('consentNeedAccount')}</p>
              {signingIn ? <SignInForm /> : <LinkForm />}
              <Button variant="ghost" className="h-11 text-base" onClick={() => setSigningIn(!signingIn)}>
                {signingIn ? t('noAccountYet') : t('haveAccount')}
              </Button>
            </section>
          ) : (
            <>
              <fieldset className="grid gap-2">
                <legend className="mb-2 font-medium">{t('consentPick')}</legend>
                {accounts.map((a) => (
                  <Label key={a.id} className="flex min-h-14 items-center gap-3 rounded-xl border px-3 font-normal has-data-checked:border-honeydew has-data-checked:bg-honeydew-tint/40">
                    <Checkbox checked={picked.includes(a.id)} onCheckedChange={(on) => setPicked((p) => (on ? [...p, a.id] : p.filter((id) => id !== a.id)))} />
                    <Avatar name={a.name} />
                    <span className="grid min-w-0 leading-tight">
                      <span className="truncate font-medium">{a.name}</span>
                      <span className="truncate text-xs text-muted-foreground">
                        {a.school} · {t(a.role)}
                      </span>
                    </span>
                  </Label>
                ))}
              </fieldset>
              <p className="text-sm text-muted-foreground">{t('consentCan')}</p>
              {approve.isError && (
                <Banner tone="watermelon" icon={<CircleAlert />} title={error(approve.error)} />
              )}
              <div className="grid grid-cols-2 gap-3">
                <Button variant="outline" className="h-12 text-base" onClick={deny}>
                  {t('deny')}
                </Button>
                <Button className="h-12 text-base" disabled={picked.length === 0 || approve.isPending} onClick={() => approve.mutate()}>
                  {approve.isPending && <Loader2 className="animate-spin" />}
                  {t('allow')}
                </Button>
              </div>
            </>
          )}
        </>
      )}
    </main>
  )
}
