import { Fragment, type ReactNode } from 'react'
import { ChevronDown, CircleHelp } from 'lucide-react'
import { useI18n } from '@/lib/i18n'
// The steps live in these files, so text and screenshots can change without touching code.
// An image is a path under web/public, e.g. "/help/step-2.png".
import en from '@/content/token-help.en.json'
import nl from '@/content/token-help.nl.json'

type Help = { title: string; steps: { title: string; text: string; image: string | null }[] }

/** **bold** is the only markup the steps use. */
function rich(text: string): ReactNode {
  return text.split(/\*\*(.+?)\*\*/g).map((part, i) => (i % 2 ? <strong key={i} className="font-semibold text-foreground">{part}</strong> : <Fragment key={i}>{part}</Fragment>))
}

/** A closed-by-default explainer on getting a Zermelo link code into Melon. */
export function TokenHelp() {
  const { lang } = useI18n()
  const help: Help = lang === 'en' ? en : nl
  return (
    <details className="group rounded-xl border bg-card text-sm open:pb-1">
      <summary className="flex min-h-11 cursor-pointer list-none items-center gap-2 rounded-xl px-3 py-2 font-medium select-none hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none [&::-webkit-details-marker]:hidden">
        <CircleHelp className="size-4 shrink-0 text-honeydew-ink" />
        <span className="flex-1">{help.title}</span>
        <ChevronDown className="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180 motion-reduce:transition-none" />
      </summary>
      <ol className="grid gap-3 px-3 pt-1 pb-2">
        {help.steps.map((s, i) => (
          <li key={i} className="grid grid-cols-[1.75rem_minmax(0,1fr)] gap-x-2 gap-y-1.5">
            <span aria-hidden="true" className="grid size-6 place-items-center rounded-full bg-honeydew-tint text-xs font-semibold text-honeydew-ink tabular-nums">
              {i + 1}
            </span>
            <div className="grid gap-0.5">
              <p className="font-medium">{s.title}</p>
              <p className="text-muted-foreground">{rich(s.text)}</p>
            </div>
            {s.image && <img src={s.image} alt={s.title} loading="lazy" className="col-start-2 w-full rounded-lg border" />}
          </li>
        ))}
      </ol>
    </details>
  )
}
