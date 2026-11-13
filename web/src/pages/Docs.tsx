import { Link } from 'react-router-dom'
import { useTheme } from 'next-themes'
import { ApiReferenceReact } from '@scalar/api-reference-react'
import '@scalar/api-reference-react/style.css'
import { ArrowLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Logo } from '@/components/Logo'
import './docs.css'

/** Interactive API reference, rendered from the server's own OpenAPI file. */
export default function Docs() {
  const { resolvedTheme } = useTheme()
  return (
    <div className="min-h-dvh">
      <header className="flex h-14 items-center gap-2 border-b px-3">
        <Button variant="ghost" size="icon" className="size-11" aria-label="Melon" render={<Link to="/" />}>
          <ArrowLeft className="size-5" />
        </Button>
        <Logo className="text-2xl" withName />
        <span className="text-muted-foreground">API</span>
      </header>
      <ApiReferenceReact
        key={resolvedTheme}
        configuration={{
          url: '/api/openapi.yaml',
          darkMode: resolvedTheme === 'dark',
          forceDarkModeState: resolvedTheme === 'dark' ? 'dark' : 'light',
          hideDarkModeToggle: true,
          hideClientButton: true,
          withDefaultFonts: false,
          telemetry: false,
          showDeveloperTools: 'never',
          agent: { disabled: true },
          mcp: { disabled: true },
          persistAuth: false,
        }}
      />
    </div>
  )
}
