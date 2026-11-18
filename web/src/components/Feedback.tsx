import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useLocation } from 'react-router-dom'
import { ArrowLeft, Bug, Lightbulb, Loader2, MessageSquareHeart, Send } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Chip } from '@/components/bits'
import { api, type FeedbackKind, type FeedbackThread, type FeedbackSummary } from '@/lib/api'
import { formatTime } from '@/lib/dates'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'

const textareaClass =
  'min-h-28 w-full resize-y rounded-lg border border-input bg-transparent px-3 py-2 text-base outline-none placeholder:text-muted-foreground/60 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 md:text-sm dark:bg-input/30'

export function KindChip({ kind }: { kind: FeedbackKind }) {
  const { t } = useI18n()
  return (
    <Chip tone={kind === 'bug' ? 'watermelon' : 'honeydew'}>
      {kind === 'bug' ? <Bug /> : <Lightbulb />}
      {t(kind === 'bug' ? 'feedbackBug' : 'feedbackIdea')}
    </Chip>
  )
}

/** One summary row in a list of threads. */
export function ThreadRow({ thread: f, title, onOpen }: { thread: FeedbackSummary; title?: string; onOpen: () => void }) {
  const { t, locale, plural } = useI18n()
  return (
    <button
      type="button"
      onClick={onOpen}
      className="grid w-full gap-1 rounded-xl border bg-card p-3 text-left transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
    >
      <span className="flex min-w-0 items-center gap-2">
        <KindChip kind={f.kind} />
        {title && <span className="truncate text-sm font-medium">{title}</span>}
        <span className="flex-1" />
        {f.unread && (
          <span className="flex items-center gap-1 text-xs font-medium text-honeydew-ink">
            <span className="size-2 rounded-full bg-honeydew" />
            {t('feedbackUnread')}
          </span>
        )}
        {f.status === 'closed' && <span className="text-xs text-muted-foreground">{t('feedbackClosed')}</span>}
      </span>
      <span className="line-clamp-2 text-sm">{f.preview}</span>
      <span className="text-xs text-muted-foreground">
        {new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short' }).format(new Date(f.updatedAt))} · {formatTime(f.updatedAt, locale)} · {plural('feedbackCount', f.messages)}
      </span>
    </button>
  )
}

/**
 * A thread as a chat: your own messages on the right, the other side's on the
 * left, and a box to answer. Used by the user and by the admins.
 */
export function ThreadView({ thread, side, otherName, onSend, sending }: { thread: FeedbackThread; side: 'user' | 'admin'; otherName: string; onSend: (text: string) => Promise<unknown>; sending: boolean }) {
  const { t, locale } = useI18n()
  const [text, setText] = useState('')
  return (
    <div className="grid min-w-0 gap-3">
      <ol className="grid gap-2">
        {thread.messages.map((m) => {
          const mine = (side === 'admin') === m.fromAdmin
          return (
            <li key={m.id} className={cn('grid max-w-[85%] gap-0.5', mine ? 'justify-self-end' : 'justify-self-start')}>
              <span className={cn('px-1 text-xs text-muted-foreground', mine && 'text-right')}>
                {mine ? t('feedbackYou') : otherName} · {new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short' }).format(new Date(m.at))} {formatTime(m.at, locale)}
              </span>
              <p className={cn('rounded-2xl px-3.5 py-2 text-sm break-words whitespace-pre-wrap', mine ? 'rounded-br-md bg-honeydew-tint text-honeydew-ink' : 'rounded-bl-md bg-muted')}>{m.body}</p>
            </li>
          )
        })}
      </ol>
      <form
        className="grid gap-2"
        onSubmit={async (e) => {
          e.preventDefault()
          if (!text.trim()) return
          await onSend(text.trim())
          setText('')
        }}
      >
        <textarea value={text} onChange={(e) => setText(e.target.value)} maxLength={4000} placeholder={t('feedbackReplyPlaceholder')} aria-label={t('feedbackReply')} className={cn(textareaClass, 'min-h-20')} />
        <Button type="submit" className="h-11 justify-self-end" disabled={!text.trim() || sending}>
          {sending ? <Loader2 className="animate-spin" /> : <Send />}
          {t('feedbackSend')}
        </Button>
      </form>
    </div>
  )
}

type View = { kind: 'list' } | { kind: 'new' } | { kind: 'thread'; id: string }

/** The round button at the bottom right, and the dialog it opens. */
export function FeedbackButton() {
  const { t, error } = useI18n()
  const { session, active } = useSession()
  const { pathname } = useLocation()
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [view, setView] = useState<View>({ kind: 'list' })
  const [kind, setKind] = useState<FeedbackKind>('idea')
  const [text, setText] = useState('')

  // Only inside the app proper, never while an admin looks at Melon as someone else.
  const shown = !!active && !session.impersonating && (pathname === '/' || pathname === '/settings')
  const mine = useQuery({ queryKey: ['feedback'], queryFn: api.feedback.list, enabled: shown, refetchInterval: 2 * 60_000, refetchOnWindowFocus: true })
  const thread = useQuery({ queryKey: ['feedback', view.kind === 'thread' ? view.id : ''], queryFn: () => api.feedback.get((view as { id: string }).id), enabled: open && view.kind === 'thread' })
  const refresh = () => qc.invalidateQueries({ queryKey: ['feedback'] })

  const create = useMutation({
    mutationFn: () => api.feedback.create(kind, text.trim(), pathname),
    onSuccess: (th) => {
      setText('')
      toast.success(t('feedbackSent'))
      qc.setQueryData(['feedback', th.thread.id], th)
      setView({ kind: 'thread', id: th.thread.id })
      refresh()
    },
    onError: (e) => toast.error(error(e)),
  })
  const reply = useMutation({
    mutationFn: (body: string) => api.feedback.reply((view as { id: string }).id, body),
    onSuccess: (th) => {
      qc.setQueryData(['feedback', th.thread.id], th)
      refresh()
    },
    onError: (e) => toast.error(error(e)),
  })

  if (!shown) return null
  const unread = mine.data?.some((f) => f.unread) ?? false
  const openDialog = () => {
    setView(mine.data && mine.data.length > 0 ? { kind: 'list' } : { kind: 'new' })
    setOpen(true)
  }

  return (
    <>
      <button
        type="button"
        onClick={openDialog}
        aria-label={unread ? `${t('feedbackTitle')} · ${t('feedbackUnread')}` : t('feedbackTitle')}
        className="fixed right-4 bottom-[max(1rem,env(safe-area-inset-bottom))] z-30 grid size-12 place-items-center rounded-full border bg-primary text-primary-foreground shadow-lg transition-transform hover:scale-105 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none active:scale-95 motion-reduce:transition-none md:right-6 md:bottom-6"
      >
        <MessageSquareHeart className="size-5" />
        {unread && <span className="absolute top-0.5 right-0.5 size-3 rounded-full bg-watermelon ring-2 ring-background" />}
      </button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {view.kind !== 'list' && mine.data && mine.data.length > 0 && (
                <Button variant="ghost" size="icon" className="-ml-2 size-9" aria-label={t('back')} onClick={() => setView({ kind: 'list' })}>
                  <ArrowLeft />
                </Button>
              )}
              {t('feedbackTitle')}
            </DialogTitle>
            <DialogDescription>{t('feedbackIntro')}</DialogDescription>
          </DialogHeader>

          {view.kind === 'list' && (
            <div className="grid gap-2">
              <Button className="h-11" onClick={() => setView({ kind: 'new' })}>
                {t('feedbackNew')}
              </Button>
              {mine.data?.map((f) => <ThreadRow key={f.id} thread={f} onOpen={() => setView({ kind: 'thread', id: f.id })} />)}
            </div>
          )}

          {view.kind === 'new' && (
            <form
              className="grid gap-3"
              onSubmit={(e) => {
                e.preventDefault()
                if (text.trim()) create.mutate()
              }}
            >
              <div role="radiogroup" aria-label={t('feedbackKind')} className="grid grid-cols-2 gap-2">
                {(['idea', 'bug'] as const).map((k) => (
                  <button
                    key={k}
                    type="button"
                    role="radio"
                    aria-checked={kind === k}
                    onClick={() => setKind(k)}
                    className={cn(
                      'flex h-11 items-center justify-center gap-2 rounded-xl border text-sm font-medium transition-colors focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none [&>svg]:size-4',
                      kind === k ? (k === 'bug' ? 'border-watermelon bg-watermelon-tint text-watermelon-ink' : 'border-honeydew bg-honeydew-tint text-honeydew-ink') : 'hover:bg-muted',
                    )}
                  >
                    {k === 'bug' ? <Bug /> : <Lightbulb />}
                    {t(k === 'bug' ? 'feedbackBug' : 'feedbackIdea')}
                  </button>
                ))}
              </div>
              <textarea value={text} onChange={(e) => setText(e.target.value)} maxLength={4000} autoFocus placeholder={t(kind === 'bug' ? 'feedbackPlaceholderBug' : 'feedbackPlaceholderIdea')} aria-label={t('feedbackNew')} className={textareaClass} />
              <Button type="submit" className="h-11" disabled={!text.trim() || create.isPending}>
                {create.isPending ? <Loader2 className="animate-spin" /> : <Send />}
                {t('feedbackSend')}
              </Button>
            </form>
          )}

          {view.kind === 'thread' &&
            (thread.data ? (
              <div className="grid gap-3">
                <div className="flex items-center gap-2">
                  <KindChip kind={thread.data.thread.kind} />
                  {thread.data.thread.status === 'closed' && <span className="text-xs text-muted-foreground">{t('feedbackClosedHint')}</span>}
                </div>
                <ThreadView thread={thread.data} side="user" otherName="Melon" onSend={(b) => reply.mutateAsync(b)} sending={reply.isPending} />
              </div>
            ) : (
              <Loader2 className="mx-auto my-6 animate-spin text-muted-foreground" />
            ))}
        </DialogContent>
      </Dialog>
    </>
  )
}
