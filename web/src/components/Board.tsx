import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, Circle, CircleCheck, CircleDot, GripVertical, Hammer, Lightbulb, Plus, Rocket, Trash2, type LucideIcon } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import { Input } from '@/components/ui/input'
import { Chip } from '@/components/bits'
import { MarkdownEditor } from '@/components/MarkdownEditor'
import { ChipPicker } from '@/components/NotesList'
import { api, lanes, type Board as BoardName, type Card } from '@/lib/api'
import { useI18n, type Key } from '@/lib/i18n'
import { Markdown, markdownClass, toggleTask } from '@/lib/markdown'
import { subjectOptions, useSubjects } from '@/lib/subjects'
import { cn } from '@/lib/utils'

type LaneLook = { icon: LucideIcon; dot: string; head: string }

// Kanban wears Melon's own colours; the Hackban, Hack Club's red, orange and green.
const looks: Record<string, LaneLook> = {
  todo: { icon: Circle, dot: 'text-muted-foreground', head: 'bg-muted/60' },
  doing: { icon: CircleDot, dot: 'text-cantaloupe-ink', head: 'bg-cantaloupe-tint' },
  done: { icon: CircleCheck, dot: 'text-honeydew-ink', head: 'bg-honeydew-tint' },
  idea: { icon: Lightbulb, dot: 'text-[#a633d6] dark:text-[#c78cf0]', head: 'bg-[#a633d6]/10' },
  hacking: { icon: Hammer, dot: 'text-[#ec3750] dark:text-[#ff6b81]', head: 'bg-[#ec3750]/10' },
  shipped: { icon: Rocket, dot: 'text-[#1f9d7a] dark:text-[#33d6a6]', head: 'bg-[#33d6a6]/15' },
}

type Drag = { card: Card; x: number; y: number; dx: number; dy: number; width: number; height: number; lane: string | null; index: number; started: boolean; sx: number; sy: number }

/**
 * A board with lanes of cards. Cards drag between lanes with a mouse, a
 * finger (on the grip) or a pen; the arrow on each card moves it one lane on.
 */
export function Board({ accountId, board }: { accountId: string; board: BoardName }) {
  const { t, error } = useI18n()
  const qc = useQueryClient()
  const key = ['cards', accountId, board]
  const cards = useQuery({ queryKey: key, queryFn: () => api.cards.list(accountId, board) })
  const { names, list: subjects } = useSubjects(accountId)
  const [editing, setEditing] = useState<Card | { lane: string } | null>(null)
  const [drag, setDrag] = useState<Drag | null>(null)
  const dragRef = useRef<Drag | null>(null)
  const laneRefs = useRef(new Map<string, HTMLElement>())
  const justDragged = useRef(false)
  const laneNames = lanes[board]

  const all = cards.data ?? []
  const byLane = (lane: string) => all.filter((c) => c.lane === lane).sort((a, b) => a.position - b.position)

  const move = useMutation({
    mutationFn: ({ id, ...change }: { id: string; lane?: string; index?: number; text?: string }) => api.cards.update(accountId, id, change),
    onMutate: async ({ id, lane, index, text }) => {
      await qc.cancelQueries({ queryKey: key })
      const before = qc.getQueryData<Card[]>(key)
      qc.setQueryData<Card[]>(key, (old) => {
        if (!old) return old
        const card = old.find((c) => c.id === id)
        if (!card) return old
        const moved = { ...card, lane: lane ?? card.lane, text: text ?? card.text }
        if (lane === undefined && index === undefined) return old.map((c) => (c.id === id ? moved : c))
        // Lay the target lane out again, with the card at its new place.
        const rest = old.filter((c) => c.id !== id)
        const target = rest.filter((c) => c.lane === moved.lane).sort((a, b) => a.position - b.position)
        const at = index === undefined || index < 0 || index > target.length ? target.length : index
        target.splice(at, 0, moved)
        const positions = new Map(target.map((c, i) => [c.id, i]))
        return [...rest.filter((c) => c.lane !== moved.lane), ...target].map((c) => (positions.has(c.id) ? { ...c, position: positions.get(c.id)! } : c))
      })
      return { before }
    },
    onError: (e, _v, ctx) => {
      if (ctx?.before) qc.setQueryData(key, ctx.before)
      toast.error(error(e))
    },
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  })

  // Where a pointer at (x, y) would drop: the lane under it and the place in that lane.
  const target = (x: number, y: number, id: string): { lane: string | null; index: number } => {
    for (const [lane, el] of laneRefs.current) {
      const r = el.getBoundingClientRect()
      if (x < r.left || x > r.right || y < r.top - 24 || y > r.bottom + 24) continue
      const items = [...el.querySelectorAll<HTMLElement>('[data-card]')].filter((c) => c.dataset.card !== id)
      let index = items.length
      for (let i = 0; i < items.length; i++) {
        const b = items[i].getBoundingClientRect()
        if (y < b.top + b.height / 2) {
          index = i
          break
        }
      }
      return { lane, index }
    }
    return { lane: null, index: -1 }
  }

  const update = (d: Drag | null) => {
    dragRef.current = d
    setDrag(d)
  }

  const onPointerDown = (e: ReactPointerEvent<HTMLElement>, card: Card) => {
    if (e.button !== 0) return
    const fromGrip = !!(e.target as HTMLElement).closest('[data-grip]')
    // Touch drags only from the grip, so the page still scrolls under a finger.
    if (e.pointerType === 'touch' && !fromGrip) return
    if (!fromGrip && (e.target as HTMLElement).closest('button:not([data-card-open]), a, label')) return
    const el = (e.currentTarget.closest('[data-card]') as HTMLElement) ?? e.currentTarget
    const r = el.getBoundingClientRect()
    el.setPointerCapture?.(e.pointerId)
    update({ card, x: e.clientX, y: e.clientY, sx: e.clientX, sy: e.clientY, dx: e.clientX - r.left, dy: e.clientY - r.top, width: r.width, height: r.height, lane: card.lane, index: -1, started: fromGrip && e.pointerType === 'touch' })
  }

  useEffect(() => {
    if (!drag) return
    let frame = 0
    // Scroll the page while a card is held near the top or bottom edge.
    const scroll = () => {
      const d = dragRef.current
      if (d?.started) {
        const edge = 72
        if (d.y < edge) window.scrollBy(0, -Math.ceil((edge - d.y) / 6))
        else if (d.y > window.innerHeight - edge) window.scrollBy(0, Math.ceil((d.y - window.innerHeight + edge) / 6))
      }
      frame = requestAnimationFrame(scroll)
    }
    frame = requestAnimationFrame(scroll)
    const onMove = (e: PointerEvent) => {
      const d = dragRef.current
      if (!d) return
      const started = d.started || Math.hypot(e.clientX - d.sx, e.clientY - d.sy) > 6
      if (started) e.preventDefault()
      const at = started ? target(e.clientX, e.clientY, d.card.id) : { lane: d.lane, index: d.index }
      update({ ...d, x: e.clientX, y: e.clientY, started, ...at })
    }
    const onUp = () => {
      const d = dragRef.current
      update(null)
      if (!d?.started) return
      justDragged.current = true
      setTimeout(() => (justDragged.current = false), 0)
      if (!d.lane) return
      const current = byLane(d.card.lane).filter((c) => c.id !== d.card.id)
      const now = byLane(d.card.lane).findIndex((c) => c.id === d.card.id)
      if (d.lane === d.card.lane && (d.index === now || (d.index === current.length && now === current.length))) return
      move.mutate({ id: d.card.id, lane: d.lane, index: d.index })
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && update(null)
    window.addEventListener('pointermove', onMove, { passive: false })
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onUp)
    window.addEventListener('keydown', onKey)
    return () => {
      cancelAnimationFrame(frame)
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onUp)
      window.removeEventListener('keydown', onKey)
    }
    // Only (re)attach when a drag starts or ends.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [!!drag])

  const dragging = drag?.started ? drag : null
  const hackban = board === 'hackban'

  return (
    <div className={cn('grid grid-cols-[minmax(0,1fr)] gap-3', dragging && 'cursor-grabbing select-none')}>
      <p className={cn('text-sm', hackban ? 'rounded-xl border-l-4 border-[#ec3750] bg-[#ec3750]/8 px-3 py-2 font-medium text-[#b3122d] dark:text-[#ff8b9c]' : 'text-muted-foreground')}>
        {t(hackban ? 'boardIntroHackban' : 'boardIntroKanban')}
      </p>
      <div className="grid gap-4 md:grid-cols-3 md:items-start">
        {laneNames.map((lane, li) => {
          const look = looks[lane]
          const list = byLane(lane)
          const shown = dragging ? list.filter((c) => c.id !== dragging.card.id) : list
          const over = dragging?.lane === lane
          const laneLabel = t(`lane_${lane}` as Key)
          const next = laneNames[li + 1]
          return (
            <section
              key={lane}
              ref={(el) => {
                if (el) laneRefs.current.set(lane, el)
                else laneRefs.current.delete(lane)
              }}
              data-lane={lane}
              aria-label={laneLabel}
              className={cn('grid gap-2 rounded-2xl border bg-muted/30 p-2 transition-colors', over && 'border-ring bg-muted/60')}
            >
              <header className={cn('flex h-10 items-center gap-2 rounded-xl px-3', look.head)}>
                <look.icon className={cn('size-4', look.dot)} />
                <h3 className="flex-1 text-sm font-semibold">{laneLabel}</h3>
                <span className="text-xs text-muted-foreground tabular-nums">{list.length}</span>
                <Button variant="ghost" size="icon-sm" className="size-8" aria-label={t('cardNewIn', { lane: laneLabel })} title={t('cardNewIn', { lane: laneLabel })} onClick={() => setEditing({ lane })}>
                  <Plus />
                </Button>
              </header>
              <ul className="grid min-h-16 gap-2">
                {shown.map((c, i) => (
                  <li key={c.id} className="grid gap-2">
                    {over && dragging.index === i && <Placeholder height={dragging.height} />}
                    <CardView
                      card={c}
                      subject={c.subject ? (names.get(c.subject) ?? c.subject.toUpperCase()) : ''}
                      next={next ? { lane: next, label: t(`lane_${next}` as Key) } : null}
                      onPointerDown={(e) => onPointerDown(e, c)}
                      onOpen={() => !justDragged.current && setEditing(c)}
                      onNext={() => next && move.mutate({ id: c.id, lane: next })}
                      onToggle={(n) => move.mutate({ id: c.id, text: toggleTask(c.text, n) })}
                    />
                  </li>
                ))}
                {over && dragging.index >= shown.length && <Placeholder height={dragging.height} />}
                {shown.length === 0 && !over && (
                  <li className="grid h-16 place-items-center rounded-xl border border-dashed text-xs text-muted-foreground">{t('laneEmpty')}</li>
                )}
              </ul>
            </section>
          )
        })}
      </div>

      {dragging && (
        <div className="pointer-events-none fixed top-0 left-0 z-50" style={{ width: dragging.width, transform: `translate(${dragging.x - dragging.dx}px, ${dragging.y - dragging.dy}px) rotate(2deg)` }}>
          <CardView card={dragging.card} subject={dragging.card.subject ? (names.get(dragging.card.subject) ?? '') : ''} next={null} lifted />
        </div>
      )}

      <CardEditor
        key={editing && 'id' in editing ? editing.id : (editing?.lane ?? 'none')}
        accountId={accountId}
        board={board}
        editing={editing}
        subjects={subjects}
        names={names}
        onClose={() => setEditing(null)}
      />
    </div>
  )
}

function Placeholder({ height }: { height: number }) {
  return <div aria-hidden="true" className="rounded-xl border-2 border-dashed border-ring/60 bg-background/60" style={{ height }} />
}

function CardView({
  card,
  subject,
  next,
  lifted,
  onPointerDown,
  onOpen,
  onNext,
  onToggle,
}: {
  card: Card
  subject: string
  next: { lane: string; label: string } | null
  lifted?: boolean
  onPointerDown?: (e: ReactPointerEvent<HTMLElement>) => void
  onOpen?: () => void
  onNext?: () => void
  onToggle?: (n: number) => void
}) {
  const { t } = useI18n()
  return (
    <article
      data-card={card.id}
      onPointerDown={onPointerDown}
      className={cn('group relative flex gap-1 rounded-xl border bg-card py-2.5 pr-2 pl-1 shadow-xs', lifted ? 'shadow-xl ring-2 ring-ring' : 'hover:border-ring/60 md:cursor-grab')}
    >
      <span data-grip className="relative z-10 flex w-6 shrink-0 cursor-grab touch-none items-start justify-center pt-0.5 text-muted-foreground/60" aria-hidden="true" title={t('cardDrag')}>
        <GripVertical className="size-4" />
      </span>
      <div className="grid min-w-0 flex-1 gap-1.5">
        <button type="button" data-card-open className="text-left text-sm leading-snug font-medium after:absolute after:inset-0 after:rounded-xl focus-visible:outline-none focus-visible:after:ring-3 focus-visible:after:ring-ring/50" onClick={onOpen}>
          {card.title}
        </button>
        {card.text && (
          <div className={cn(markdownClass, 'pointer-events-none relative max-h-40 overflow-hidden text-[0.8125rem] text-muted-foreground [mask-image:linear-gradient(to_bottom,black_70%,transparent)] [&_a]:pointer-events-auto [&_label]:pointer-events-auto')}>
            <Markdown source={card.text} headingIds={false} onToggleTask={onToggle} />
          </div>
        )}
        {subject && (
          <div>
            <Chip tone="honeydew">{subject}</Chip>
          </div>
        )}
      </div>
      {next && (
        <Button
          variant="ghost"
          size="icon-sm"
          className="relative size-8 shrink-0 self-start text-muted-foreground opacity-100 hover:text-foreground md:opacity-0 md:group-hover:opacity-100 md:focus-visible:opacity-100"
          aria-label={t('cardMoveTo', { lane: next.label })}
          title={t('cardMoveTo', { lane: next.label })}
          onClick={onNext}
        >
          <ArrowRight />
        </Button>
      )}
    </article>
  )
}

function CardEditor({
  accountId,
  board,
  editing,
  subjects,
  names,
  onClose,
}: {
  accountId: string
  board: BoardName
  editing: Card | { lane: string } | null
  subjects: { code: string; name: string }[]
  names: Map<string, string>
  onClose: () => void
}) {
  const { t, error } = useI18n()
  const qc = useQueryClient()
  const existing = editing && 'id' in editing ? editing : null
  const [title, setTitle] = useState(existing?.title ?? '')
  const [text, setText] = useState(existing?.text ?? '')
  const [subject, setSubject] = useState(existing?.subject ?? '')
  const [lane, setLane] = useState(editing?.lane ?? lanes[board][0])
  const done = () => {
    void qc.invalidateQueries({ queryKey: ['cards', accountId, board] })
    onClose()
  }
  const save = useMutation({
    mutationFn: () => {
      const body = { title, text, subject, ...(existing && existing.lane === lane ? {} : { lane }) }
      return existing ? api.cards.update(accountId, existing.id, body) : api.cards.add(accountId, board, body)
    },
    onSuccess: done,
    onError: (e) => toast.error(error(e)),
  })
  const remove = useMutation({
    mutationFn: () => api.cards.remove(accountId, existing!.id),
    onSuccess: () => {
      toast.success(t('cardDeleted'))
      done()
    },
    onError: (e) => toast.error(error(e)),
  })
  const canSave = title.trim() !== '' && !save.isPending
  return (
    <Drawer open={editing !== null} onOpenChange={(o) => !o && onClose()} showSwipeHandle>
      <DrawerContent className="mx-auto max-w-2xl">
        <form
          className="grid gap-4 overflow-y-auto px-5 pt-2 pb-[max(1.5rem,env(safe-area-inset-bottom))]"
          onSubmit={(e) => {
            e.preventDefault()
            if (canSave) save.mutate()
          }}
        >
          <DrawerHeader className="px-0 text-left">
            <DrawerTitle className="text-xl">{existing ? t('cardEdit') : t('cardNew')}</DrawerTitle>
          </DrawerHeader>
          <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder={t('cardTitlePlaceholder')} aria-label={t('cardTitle')} maxLength={200} className="h-11 text-base font-medium" autoFocus={!existing} />
          <div className="grid gap-1.5">
            <span className="text-sm font-medium">{t('cardLane')}</span>
            <ChipPicker label={t('cardLane')} value={lane} options={lanes[board].map((l) => ({ value: l, label: t(`lane_${l}` as Key) }))} onChange={setLane} />
          </div>
          <div className="grid gap-1.5">
            <span className="text-sm font-medium">{t('noteSubject')}</span>
            <ChipPicker label={t('noteSubject')} value={subject} options={[{ value: '', label: t('noSubject') }, ...subjectOptions(subjects, names, subject)]} onChange={setSubject} />
          </div>
          <MarkdownEditor value={text} onChange={setText} label={t('cardText')} maxLength={5000} onSubmit={() => canSave && save.mutate()} />
          <div className="flex gap-2">
            {existing && (
              <Button type="button" variant="ghost" className="h-11 text-watermelon-ink" disabled={remove.isPending} onClick={() => remove.mutate()}>
                <Trash2 />
                {t('remove')}
              </Button>
            )}
            <span className="flex-1" />
            <Button type="button" variant="outline" className="h-11" onClick={onClose}>
              {t('cancel')}
            </Button>
            <Button type="submit" className="h-11" disabled={!canSave}>
              {t('save')}
            </Button>
          </div>
        </form>
      </DrawerContent>
    </Drawer>
  )
}
