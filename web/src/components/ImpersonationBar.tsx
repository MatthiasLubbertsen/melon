import { useMutation } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { Eye, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'

/** While an admin looks at Melon as someone else, says so on every page, with the way back. */
export function ImpersonationBar() {
  const { t, error } = useI18n()
  const { session, swap } = useSession()
  const navigate = useNavigate()
  const stop = useMutation({
    mutationFn: api.admin.stop,
    onSuccess: async (next) => {
      await swap(next)
      navigate(next.admin ? '/admin' : '/')
    },
    onError: (e) => toast.error(error(e)),
  })
  if (!session.impersonating) return null
  const name = session.accounts[0]?.name ?? '?'
  return (
    <>
      {/* Room at the end of the page, so the bar never covers the last thing on it. */}
      <div aria-hidden="true" className="h-16" />
      <div className="pointer-events-none fixed inset-x-0 bottom-0 z-40 flex justify-center px-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
        <div role="status" className="pointer-events-auto flex max-w-full items-center gap-3 rounded-full bg-cantaloupe-tint py-1.5 pr-1.5 pl-4 text-sm font-medium text-cantaloupe-ink shadow-lg ring-1 ring-cantaloupe/40">
          <Eye className="size-4 shrink-0" />
          <span className="truncate">{t('impersonating', { name })}</span>
          <Button className="h-9 shrink-0 rounded-full" disabled={stop.isPending} onClick={() => stop.mutate()}>
            {stop.isPending && <Loader2 className="animate-spin" />}
            {t('impersonateStop')}
          </Button>
        </div>
      </div>
    </>
  )
}
