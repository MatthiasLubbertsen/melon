import { Link, useSearchParams } from 'react-router-dom'
import { ArrowLeft, Hammer, NotebookText, SquareKanban } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Board } from '@/components/Board'
import { NotesList } from '@/components/NotesList'
import { useI18n } from '@/lib/i18n'
import { useSession } from '@/lib/session'

const tabs = ['notes', 'kanban', 'hackban'] as const
type Tab = (typeof tabs)[number]

/** Notes (general and per subject) and two boards: a kanban and a hackban. */
export function Notes() {
  const { t } = useI18n()
  const { active } = useSession()
  const account = active!
  // The tab and subject live in the URL, so a link can open a subject's notes.
  const [params, setParams] = useSearchParams()
  const tab: Tab = tabs.find((x) => x === params.get('tab')) ?? 'notes'
  const subject = params.has('subject') ? (params.get('subject') ?? '') : null
  const set = (next: { tab?: Tab; subject?: string | null }) => {
    const p = new URLSearchParams(params)
    if (next.tab) p.set('tab', next.tab)
    if (next.subject !== undefined) {
      if (next.subject === null) p.delete('subject')
      else p.set('subject', next.subject)
    }
    if (p.get('tab') === 'notes') p.delete('tab')
    setParams(p, { replace: true })
  }

  return (
    <div className="mx-auto min-h-dvh w-full max-w-6xl px-3 pt-[max(0.5rem,env(safe-area-inset-top))] pb-[max(2rem,env(safe-area-inset-bottom))] md:px-6">
      <header className="flex h-14 items-center gap-1">
        <Button variant="ghost" size="icon" className="size-11" aria-label={t('back')} render={<Link to="/" />}>
          <ArrowLeft className="size-5" />
        </Button>
        <h1 className="flex-1 text-xl font-semibold">{t('notesAndBoards')}</h1>
      </header>
      <Tabs value={tab} onValueChange={(v) => set({ tab: v as Tab })} className="gap-4">
        <TabsList className="h-11 w-full md:max-w-md">
          <TabsTrigger value="notes">
            <NotebookText />
            {t('notes')}
          </TabsTrigger>
          <TabsTrigger value="kanban">
            <SquareKanban />
            {t('kanban')}
          </TabsTrigger>
          <TabsTrigger value="hackban">
            <Hammer />
            {t('hackban')}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="notes">
          <NotesList accountId={account.id} subject={subject} onSubject={(s) => set({ subject: s })} />
        </TabsContent>
        <TabsContent value="kanban">
          <Board accountId={account.id} board="kanban" />
        </TabsContent>
        <TabsContent value="hackban">
          <Board accountId={account.id} board="hackban" />
        </TabsContent>
      </Tabs>
    </div>
  )
}
