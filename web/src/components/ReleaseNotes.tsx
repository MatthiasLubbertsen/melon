import { useEffect, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Logo } from '@/components/Logo'
import { releases, type Release } from '@/content/releases'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'

const STORAGE = 'melon.releases.seen'
/** What someone who used Melon before release notes existed gets to see once. */
const FIRST_TIME = 3

function readSeen(): string | null {
  try {
    return localStorage.getItem(STORAGE)
  } catch {
    return null
  }
}

function markSeen() {
  try {
    if (releases[0]) localStorage.setItem(STORAGE, releases[0].id)
  } catch {
    /* private mode: they may see the notes again, which is fine */
  }
}

/** The releases this browser has not shown yet, newest first. */
function unseen(seen: string | null): Release[] {
  if (seen === null) return releases.slice(0, FIRST_TIME)
  const i = releases.findIndex((r) => r.id === seen)
  return i === -1 ? [] : releases.slice(0, i)
}

/**
 * Shows what is new in Melon once, the next time someone opens their
 * schedule. Everything released since their last visit is listed together.
 */
export function ReleaseNotes() {
  const { t, lang, locale } = useI18n()
  const { session, active } = useSession()
  const { pathname } = useLocation()
  const [notes, setNotes] = useState<Release[]>([])
  const [open, setOpen] = useState(false)

  useEffect(() => {
    if (session.impersonating) return
    const seen = readSeen()
    // Someone new to Melon has nothing to catch up on.
    if (!active) {
      if (seen === null) markSeen()
      return
    }
    if (pathname !== '/' || open) return
    const list = unseen(seen)
    if (list.length === 0) {
      if (seen !== releases[0]?.id) markSeen()
      return
    }
    setNotes(list)
    setOpen(true)
  }, [active, session.impersonating, pathname, open])

  const close = (next: boolean) => {
    if (next) return
    markSeen()
    setOpen(false)
  }

  const day = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'long', timeZone: 'UTC' })
  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-md">
        <DialogHeader className="items-center pt-2 text-center">
          <Logo className="text-4xl" />
          <DialogTitle className="text-lg font-semibold tracking-tight">{t('releasesTitle')}</DialogTitle>
          <DialogDescription>{notes.length > 1 ? t('releasesIntroMany', { n: notes.length }) : t('releasesIntroOne')}</DialogDescription>
        </DialogHeader>
        <ol className="grid gap-2">
          {notes.map((r) => (
            <li key={r.id} className="flex items-start gap-3 rounded-xl bg-muted/60 p-3">
              <span className="grid size-8 shrink-0 place-items-center rounded-full bg-honeydew-tint text-honeydew-ink">
                <r.icon className="size-4" aria-hidden="true" />
              </span>
              <span className="grid min-w-0 gap-0.5">
                <span className="text-sm leading-snug">{lang === 'en' ? r.en : r.nl}</span>
                <time dateTime={r.date} className="text-xs text-muted-foreground">
                  {day.format(new Date(`${r.date}T00:00:00Z`))}
                </time>
              </span>
            </li>
          ))}
        </ol>
        <Button className="h-11" onClick={() => close(false)}>
          {t('releasesDone')}
        </Button>
      </DialogContent>
    </Dialog>
  )
}
