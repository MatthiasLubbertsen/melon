import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useLocation } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Logo } from '@/components/Logo'
import { releases, type Release } from '@/content/releases'
import { api, type Session } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'

/** What a profile from before release notes existed gets to see once. */
const FIRST_TIME = 3

/** The release notes a profile has not been shown yet, newest first. */
function unseen(seen: string | null): Release[] {
  if (seen === null) return releases.slice(0, FIRST_TIME)
  const i = releases.findIndex((r) => r.id === seen)
  // '' is a new profile; an unknown id is a note that was taken out. Either
  // way there is nothing to catch up on.
  return i === -1 ? [] : releases.slice(0, i)
}

/** Groups notes by the day they went live, keeping the order. */
function byDay(notes: Release[]): [string, Release[]][] {
  const days: [string, Release[]][] = []
  for (const r of notes) {
    const last = days[days.length - 1]
    if (last && last[0] === r.date) last[1].push(r)
    else days.push([r.date, [r]])
  }
  return days
}

/**
 * Shows what is new in Melon once, the next time someone opens their
 * schedule, on whichever device. Everything released since they last saw
 * the notes is listed together.
 */
export function ReleaseNotes() {
  const { t, lang, locale } = useI18n()
  const { session, active } = useSession()
  const { pathname } = useLocation()
  const qc = useQueryClient()
  const [notes, setNotes] = useState<Release[]>([])
  const [open, setOpen] = useState(false)
  // What this tab already marked, so a session refetch that lands before
  // the save does cannot bring the notes back.
  const marked = useRef<string | null>(null)
  // Focus lands on the dialog, not the button: it opens by itself, so a ring
  // on the button would point at something nobody reached for.
  const popup = useRef<HTMLDivElement>(null)

  const newest = releases[0]?.id
  const seen = session.releasesSeen

  const markSeen = useCallback(() => {
    if (!newest || marked.current === newest) return
    marked.current = newest
    qc.setQueryData<Session>(['session'], (s) => (s ? { ...s, releasesSeen: newest } : s))
    api.releasesSeen(newest).catch(() => {
      /* offline: they see the notes once more next time, which is fine */
    })
  }, [newest, qc])

  useEffect(() => {
    // An admin looking at Melon as someone else leaves their notes alone.
    if (!active || session.impersonating || !newest || seen === newest || marked.current === newest || open) return
    const list = unseen(seen)
    if (list.length === 0) {
      markSeen()
      return
    }
    if (pathname !== '/') return
    setNotes(list)
    setOpen(true)
  }, [active, session.impersonating, seen, newest, pathname, open, markSeen])

  const close = () => {
    markSeen()
    setOpen(false)
  }

  const day = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'long', timeZone: 'UTC' })
  return (
    <Dialog open={open} onOpenChange={(next) => !next && close()}>
      <DialogContent ref={popup} initialFocus={popup} className="flex max-h-[85dvh] flex-col sm:max-w-md">
        <DialogHeader className="items-center pt-2 text-center">
          <Logo className="text-4xl" />
          <DialogTitle className="text-lg font-semibold tracking-tight">{t('releasesTitle')}</DialogTitle>
          <DialogDescription>{notes.length > 1 ? t('releasesIntroMany', { n: notes.length }) : t('releasesIntroOne')}</DialogDescription>
        </DialogHeader>
        {/* Only the list scrolls, so a long catch-up keeps its title and button in view. */}
        <div className="-mx-4 min-h-0 flex-1 overflow-y-auto px-4">
          {byDay(notes).map(([date, list]) => (
            <section key={date} className="grid gap-2 not-first:mt-4">
              <h3 className="px-1 text-xs font-medium text-muted-foreground">
                <time dateTime={date}>{day.format(new Date(`${date}T00:00:00Z`))}</time>
              </h3>
              <ul className="grid gap-2">
                {list.map((r) => (
                  <li key={r.id} className="flex items-center gap-3 rounded-xl bg-muted/60 p-3">
                    <span className="grid size-8 shrink-0 place-items-center rounded-full bg-honeydew-tint text-honeydew-ink">
                      <r.icon className="size-4" aria-hidden="true" />
                    </span>
                    <span className="min-w-0 text-sm leading-snug">{lang === 'en' ? r.en : r.nl}</span>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
        <Button className="h-11 shrink-0" onClick={close}>
          {t('releasesDone')}
        </Button>
      </DialogContent>
    </Dialog>
  )
}
