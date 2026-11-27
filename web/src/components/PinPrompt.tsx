import { useState } from 'react'
import { KeyRound } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { PinForm } from '@/components/PinForm'
import { notesPending } from '@/components/ReleaseNotes'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'

// "Later" holds until the page is loaded again, then the question comes back.
let postponed = false

/**
 * Asks a profile without a PIN to choose one, once its schedule is on
 * screen. Without a PIN the profile only opens in this browser, so this is
 * what makes it reachable from a phone or a second laptop.
 */
export function PinPrompt({ ready }: { ready: boolean }) {
  const { t } = useI18n()
  const { session } = useSession()
  const [closed, setClosed] = useState(postponed)

  // An admin looking at Melon as someone else does not get asked; the
  // "Nieuw in Melon" popup goes first so the two never stack.
  const open = ready && !closed && !session.login && !session.impersonating && !notesPending(session.releasesSeen)

  const later = () => {
    postponed = true
    setClosed(true)
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && later()}>
      <DialogContent showCloseButton={false} className="sm:max-w-md" overlayClassName="bg-background/30 backdrop-blur-md supports-backdrop-filter:backdrop-blur-md">
        <PinForm
          onSaved={() => setClosed(true)}
          secondary={
            <Button type="button" variant="outline" className="h-11" onClick={later}>
              {t('pinAskLater')}
            </Button>
          }
        >
          <DialogHeader className="items-center pt-2 text-center">
            <span className="grid size-12 place-items-center rounded-full bg-honeydew-tint text-honeydew-ink">
              <KeyRound className="size-6" aria-hidden="true" />
            </span>
            <DialogTitle className="text-lg font-semibold tracking-tight">{t('pinAskTitle')}</DialogTitle>
            <DialogDescription>{t('pinAskText')}</DialogDescription>
          </DialogHeader>
        </PinForm>
      </DialogContent>
    </Dialog>
  )
}
