import { Button } from '@/components/ui/button'
import { useI18n } from '@/lib/i18n'

/** Shows the language you would switch to. */
export function LanguageToggle() {
  const { lang, setLang } = useI18n()
  const other = lang === 'nl' ? 'en' : 'nl'
  return (
    <Button variant="ghost" className="h-9 shrink-0 px-2.5" onClick={() => setLang(other)} lang={other}>
      {other === 'en' ? 'English' : 'Nederlands'}
    </Button>
  )
}
