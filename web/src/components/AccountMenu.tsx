import { useState } from 'react'
import { Check, ChevronsUpDown, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Avatar } from '@/components/bits'
import { LinkDialog } from '@/components/LinkForm'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'

/** Shows whose schedule this is; tap to switch or add an account. */
export function AccountMenu() {
  const { t } = useI18n()
  const { accounts, active, setActive } = useSession()
  const [linking, setLinking] = useState(false)
  if (!active) return null
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<Button variant="ghost" className="h-11 max-w-full min-w-0 justify-start gap-2.5 px-1.5" aria-label={t('switchAccount')} />}
        >
          <Avatar name={active.name} />
          <span className="grid min-w-0 text-left leading-tight">
            <span className="truncate text-sm font-medium">{active.name}</span>
            <span className="truncate text-xs font-normal text-muted-foreground">{active.school}</span>
          </span>
          <ChevronsUpDown className="size-4 shrink-0 text-muted-foreground" />
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-72 p-1.5">
          <DropdownMenuGroup>
            <DropdownMenuLabel>{t('accounts')}</DropdownMenuLabel>
            {accounts.map((a) => (
              <DropdownMenuItem key={a.id} className="gap-2.5 px-2 py-2" onClick={() => setActive(a.id)}>
                <Avatar name={a.name} />
                <span className="grid min-w-0 flex-1 leading-tight">
                  <span className="truncate font-medium">{a.name}</span>
                  <span className="truncate text-xs text-muted-foreground">
                    {a.school} · {t(a.role)}
                  </span>
                </span>
                {a.id === active.id && <Check className="size-4 text-honeydew-ink" />}
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem className="gap-2.5 px-2 py-2.5" onClick={() => setLinking(true)}>
            <span className="inline-flex size-8 items-center justify-center rounded-full border border-dashed">
              <Plus className="size-4" />
            </span>
            {t('linkAnother')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <LinkDialog open={linking} onOpenChange={setLinking} initialSchool={active.school === 'demo' ? '' : active.school} />
    </>
  )
}
