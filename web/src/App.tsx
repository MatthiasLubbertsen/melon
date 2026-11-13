import { lazy, Suspense } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { ThemeProvider } from 'next-themes'
import { Toaster } from '@/components/ui/sonner'
import { Logo } from '@/components/Logo'
import { I18nProvider } from '@/lib/i18n'
import { SessionProvider, useSession } from '@/lib/session'
import { Consent } from '@/pages/Consent'
import { Schedule } from '@/pages/Schedule'
import { Settings } from '@/pages/Settings'
import { Welcome } from '@/pages/Welcome'

const Docs = lazy(() => import('@/pages/Docs'))

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 1 } } })

function Splash() {
  return (
    <div className="grid min-h-dvh place-items-center">
      <Logo className="animate-pulse text-6xl" />
    </div>
  )
}

function Home() {
  const { active } = useSession()
  return active ? <Schedule key={active.id} /> : <Welcome />
}

function NeedsAccount({ children }: { children: React.ReactNode }) {
  const { active } = useSession()
  return active ? <>{children}</> : <Navigate to="/" replace />
}

export default function App() {
  return (
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange>
      <QueryClientProvider client={queryClient}>
        <I18nProvider>
          <BrowserRouter>
            <SessionProvider fallback={<Splash />}>
              <Suspense fallback={<Splash />}>
                <Routes>
                  <Route path="/" element={<Home />} />
                  <Route path="/settings" element={<NeedsAccount><Settings /></NeedsAccount>} />
                  <Route path="/oauth/authorize" element={<Consent />} />
                  <Route path="/docs" element={<Docs />} />
                  <Route path="*" element={<Navigate to="/" replace />} />
                </Routes>
              </Suspense>
            </SessionProvider>
          </BrowserRouter>
          <Toaster position="top-center" />
        </I18nProvider>
      </QueryClientProvider>
    </ThemeProvider>
  )
}
