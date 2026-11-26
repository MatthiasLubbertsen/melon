import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pin, PinOff, Plus, StickyNote, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import { Input } from '@/components/ui/input'
import { Chip, ConfirmDialog } from '@/components/bits'
import { MarkdownEditor } from '@/components/MarkdownEditor'
import { api, type Note } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { Markdown, markdownClass, toggleTask } from '@/lib/markdown'
import { subjectOptions, useSubjects } from '@/lib/subjects'
import { cn } from '@/lib/utils'

const MAX = 20000

/** Round chips to pick one of a few options, like the subject of a note. */
export function ChipPicker({ label, value, options, onChange }: { label: string; value: string; options: { value: string; label: string }[]; onChange: (v: string) => void }) {
  return (
    <div role="radiogroup" aria-label={label} className="flex flex-wrap gap-1.5">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={value === o.value}
          onClick={() => onChange(o.value)}
          className={cn(
            'h-8 max-w-full truncate rounded-full border px-3 text-sm transition-colors focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
            value === o.value ? 'border-primary bg-primary text-primary-foreground' : 'hover:bg-muted',
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

/** General notes and notes per subject, in Markdown. */
export function NotesList({ accountId, subject, onSubject }: { accountId: string; subject: string | null; onSubject: (s: string | null) => void }) {
  const { t, error } = useI18n()
  const qc = useQueryClient()
  const key = ['notes', accountId]
  const notes = useQuery({ queryKey: key, queryFn: () => api.notes.list(accountId) })
  const { names, list: subjects } = useSubjects(accountId)
  const [editing, setEditing] = useState<Note | 'new' | null>(null)

  const update = useMutation({
    mutationFn: ({ id, ...change }: Partial<Note> & { id: string }) => api.notes.update(accountId, id, change),
    onMutate: async ({ id, ...change }) => {
      await qc.cancelQueries({ queryKey: key })
      qc.setQueryData<Note[]>(key, (old) => old?.map((n) => (n.id === id ? { ...n, ...change } : n)))
    },
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
    onError: (e) => toast.error(error(e)),
  })

  const all = useMemo(() => notes.data ?? [], [notes.data])
  // Subjects to filter on: the ones with notes, then the rest of the account's.
  const filters = useMemo(() => {
    const used = new Set(all.map((n) => n.subject).filter(Boolean))
    const codes = [...used, ...subjects.map((s) => s.code).filter((c) => !used.has(c))]
    return [
      { value: '*', label: t('notesAll') },
      { value: '', label: t('notesGeneral') },
      ...codes.map((c) => ({ value: c, label: names.get(c) ?? c.toUpperCase() })),
    ]
  }, [all, subjects, names, t])
  const shown = subject === null ? all : all.filter((n) => n.subject === subject)

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-4">
      <div className="flex min-w-0 items-start gap-2">
        <div className="-mx-3 min-w-0 flex-1 overflow-x-auto px-3 [scrollbar-width:none]">
          <div className="flex w-max gap-1.5 md:w-auto md:flex-wrap">
            <ChipPicker label={t('noteSubject')} value={subject ?? '*'} options={filters} onChange={(v) => onSubject(v === '*' ? null : v)} />
          </div>
        </div>
      </div>
      <Button className="h-11 justify-self-start" onClick={() => setEditing('new')}>
        <Plus />
        {t('noteNew')}
      </Button>

      {notes.isPending ? null : shown.length === 0 ? (
        <div className="grid place-items-center gap-2 rounded-2xl border border-dashed px-6 py-12 text-center text-sm text-muted-foreground">
          <StickyNote className="size-8 opacity-60" />
          {subject ? t('notesEmptySubject') : t('notesEmpty')}
        </div>
      ) : (
        <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {shown.map((n) => (
            <li key={n.id}>
              <article
                data-note={n.id}
                className="group relative flex h-full flex-col gap-2 rounded-2xl border bg-card p-4 text-left shadow-xs transition-colors hover:border-ring/60"
              >
                <button type="button" className="absolute inset-0 rounded-2xl focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none" aria-label={`${t('noteEdit')}: ${n.title || t('noteUntitled')}`} onClick={() => setEditing(n)} />
                <div className="flex items-start gap-2">
                  <h3 className="min-w-0 flex-1 font-semibold leading-snug">{n.title || <span className="text-muted-foreground">{t('noteUntitled')}</span>}</h3>
                  {n.pinned && <Pin className="mt-0.5 size-4 shrink-0 fill-current text-cantaloupe-ink" aria-label={t('notePinned')} />}
                </div>
                {n.text && (
                  // Checklists tick right here; the rest of the card opens the note.
                  <div className={cn(markdownClass, 'pointer-events-none relative max-h-56 overflow-hidden [mask-image:linear-gradient(to_bottom,black_75%,transparent)] [&_a]:pointer-events-auto [&_label]:pointer-events-auto')}>
                    <Markdown source={n.text} headingIds={false} onToggleTask={(i) => update.mutate({ id: n.id, text: toggleTask(n.text, i) })} />
                  </div>
                )}
                <div className="mt-auto pt-1">
                  <Chip tone={n.subject ? 'honeydew' : 'cantaloupe'}>{n.subject ? (names.get(n.subject) ?? n.subject.toUpperCase()) : t('notesGeneral')}</Chip>
                </div>
              </article>
            </li>
          ))}
        </ul>
      )}

      <NoteEditor key={editing === 'new' ? 'new' : (editing?.id ?? 'none')} accountId={accountId} note={editing} defaultSubject={subject ?? ''} subjects={subjects} names={names} onClose={() => setEditing(null)} />
    </div>
  )
}

function NoteEditor({
  accountId,
  note,
  defaultSubject,
  subjects,
  names,
  onClose,
}: {
  accountId: string
  note: Note | 'new' | null
  defaultSubject: string
  subjects: { code: string; name: string }[]
  names: Map<string, string>
  onClose: () => void
}) {
  const { t, error } = useI18n()
  const qc = useQueryClient()
  const existing = note && note !== 'new' ? note : null
  const [title, setTitle] = useState(existing?.title ?? '')
  const [text, setText] = useState(existing?.text ?? '')
  const [subject, setSubject] = useState(existing?.subject ?? defaultSubject)
  const [pinned, setPinned] = useState(existing?.pinned ?? false)
  const [confirm, setConfirm] = useState(false)
  const done = () => {
    void qc.invalidateQueries({ queryKey: ['notes', accountId] })
    onClose()
  }
  const save = useMutation({
    mutationFn: () => {
      const body = { title, text, subject, pinned }
      return existing ? api.notes.update(accountId, existing.id, body) : api.notes.add(accountId, body)
    },
    onSuccess: done,
    onError: (e) => toast.error(error(e)),
  })
  const remove = useMutation({
    mutationFn: () => api.notes.remove(accountId, existing!.id),
    onSuccess: () => {
      toast.success(t('noteDeleted'))
      setConfirm(false)
      done()
    },
    onError: (e) => toast.error(error(e)),
  })
  const canSave = (title.trim() || text.trim()) && !save.isPending
  return (
    <Drawer open={note !== null} onOpenChange={(o) => !o && onClose()} showSwipeHandle>
      <DrawerContent className="mx-auto max-w-2xl">
        <form
          className="grid gap-4 overflow-y-auto px-5 pt-2 pb-[max(1.5rem,env(safe-area-inset-bottom))]"
          onSubmit={(e) => {
            e.preventDefault()
            if (canSave) save.mutate()
          }}
        >
          <DrawerHeader className="flex-row items-center px-0 text-left">
            <DrawerTitle className="flex-1 text-xl">{existing ? t('noteEdit') : t('noteNew')}</DrawerTitle>
            <Button
              type="button"
              variant={pinned ? 'secondary' : 'ghost'}
              size="icon"
              className="size-10"
              aria-pressed={pinned}
              aria-label={t('notePin')}
              title={t('notePin')}
              onClick={() => setPinned((p) => !p)}
            >
              {pinned ? <Pin className="fill-current" /> : <PinOff />}
            </Button>
          </DrawerHeader>
          <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder={t('noteTitlePlaceholder')} aria-label={t('noteTitle')} maxLength={200} className="h-11 text-base font-medium" autoFocus={!existing} />
          <div className="grid gap-1.5">
            <span className="text-sm font-medium">{t('noteSubject')}</span>
            <ChipPicker label={t('noteSubject')} value={subject} options={[{ value: '', label: t('notesGeneral') }, ...subjectOptions(subjects, names, subject)]} onChange={setSubject} />
          </div>
          <MarkdownEditor value={text} onChange={setText} label={t('noteText')} placeholder={t('noteTextPlaceholder')} maxLength={MAX} onSubmit={() => canSave && save.mutate()} />
          <div className="flex gap-2">
            {existing && (
              <Button type="button" variant="ghost" className="h-11 text-watermelon-ink" onClick={() => setConfirm(true)}>
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
        <ConfirmDialog open={confirm} onOpenChange={setConfirm} title={t('noteDelete')} text={t('noteDeleteText')} confirmLabel={t('remove')} onConfirm={() => remove.mutate()} busy={remove.isPending} />
      </DrawerContent>
    </Drawer>
  )
}
