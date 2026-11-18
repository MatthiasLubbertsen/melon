import { Link } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { LanguageToggle } from '@/components/LanguageToggle'
import { Logo } from '@/components/Logo'
import { useI18n } from '@/lib/i18n'
import { Markdown } from '@/lib/markdown'
// The page text lives in these files, so it can be edited without touching code.
import en from '@/content/info.en.md?raw'
import nl from '@/content/info.nl.md?raw'

/** "Over Melon": how Melon works and what it does with your data. */
export default function Info() {
  const { t, lang } = useI18n()
  return (
    <div className="mx-auto min-h-dvh w-full max-w-2xl px-4 pt-[max(0.5rem,env(safe-area-inset-top))] pb-[max(2.5rem,env(safe-area-inset-bottom))] md:px-6">
      <header className="flex h-14 items-center gap-1">
        <Button variant="ghost" size="icon" className="size-11" aria-label={t('back')} render={<Link to="/" />}>
          <ArrowLeft className="size-5" />
        </Button>
        <Logo className="text-2xl" />
        <span className="flex-1" />
        <LanguageToggle />
      </header>
      <article
        className={[
          'grid gap-4 pt-2 text-base leading-relaxed',
          '[&_h1]:text-3xl [&_h1]:font-semibold [&_h1]:tracking-tight',
          '[&_h2]:mt-4 [&_h2]:text-xl [&_h2]:font-semibold',
          '[&_h3]:mt-2 [&_h3]:text-base [&_h3]:font-semibold',
          '[&_ul]:grid [&_ul]:list-disc [&_ul]:gap-2 [&_ul]:pl-5 [&_ol]:grid [&_ol]:list-decimal [&_ol]:gap-2 [&_ol]:pl-5',
          '[&_li]:marker:text-muted-foreground [&_strong]:font-semibold',
          '[&_a]:font-medium [&_a]:text-honeydew-ink [&_a]:underline [&_a]:underline-offset-2',
          '[&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:text-sm',
          '[&_blockquote]:rounded-xl [&_blockquote]:bg-honeydew-tint [&_blockquote]:px-4 [&_blockquote]:py-3 [&_blockquote]:text-honeydew-ink',
          '[&_hr]:my-2',
          '[&_p]:text-pretty',
        ].join(' ')}
      >
        <Markdown source={lang === 'en' ? en : nl} />
      </article>
    </div>
  )
}
