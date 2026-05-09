import { useEffect, useRef, useState } from 'react'
import { HashRouter, Route, Routes, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { Sidebar } from '@/components/layout/sidebar'
import { Topbar } from '@/components/layout/topbar'
import { OnboardingDialog } from '@/components/onboarding'
import { TooltipProvider } from '@/components/ui/tooltip'
import { DashboardPage } from '@/pages/Dashboard'
import { ChannelsPage } from '@/pages/Channels'
import { TokensPage } from '@/pages/Tokens'
import { TunnelPage } from '@/pages/Tunnel'
import { LogsPage } from '@/pages/Logs'
import { SettingsPage } from '@/pages/Settings'
import { copyToClipboard, onEvent, systemInfo } from '@/lib/wails'
import { useI18n } from '@/lib/i18n'

// TrayNavigationBridge subscribes to the "prism:navigate" event emitted from
// Go (menu bar menu items) and hops the React Router to the requested path.
// It has to live inside <HashRouter> so that useNavigate() resolves.
function TrayNavigationBridge() {
  const navigate = useNavigate()
  useEffect(() => {
    let off: (() => void) | undefined
    onEvent<string>('prism:navigate', (path) => {
      if (typeof path === 'string' && path.length > 0) navigate(path)
    }).then((unsub) => {
      off = unsub
    })
    return () => {
      if (off) off()
    }
  }, [navigate])
  return null
}

// TunnelUrlChangeWatcher reacts to every "tunnel.url" emit from Go. The
// trycloudflare URL changes whenever cloudflared restarts (e.g. after Prism
// is relaunched, the network drops, or Cloudflare recycles the domain),
// which means external clients (Cursor) will see HTTP 530 / "origin
// unregistered" the next time they call the old URL.
//
// To minimise that surprise, every time the URL changes we:
//   1. write the OpenAI-compatible base URL (`<url>/v1`) to the clipboard, so
//      the user can paste it straight into Cursor without finding the app
//   2. raise a sticky toast that surfaces the new URL and a manual "Copy"
//      affordance — useful when another paste happened in between
//
// First-ever URL after cold boot is intentionally NOT announced — the app
// is brand-new in the user's mind and a toast on launch would be noise.
function TunnelUrlChangeWatcher() {
  const t = useI18n((s) => s.t)
  // Last URL we observed. Stored in a ref so that the event handler
  // closure always sees the most recent value without re-subscribing.
  const lastUrlRef = useRef<string>('')

  useEffect(() => {
    let off: (() => void) | undefined
    onEvent<string>('tunnel.url', async (url) => {
      if (typeof url !== 'string' || url.length === 0) return
      const previous = lastUrlRef.current
      lastUrlRef.current = url
      // Skip the cold-boot announcement — only react to genuine rotations.
      if (previous === '' || previous === url) return
      const baseURL = `${url}/v1`
      const ok = await copyToClipboard(baseURL)
      const title = t('tunnel.urlChanged', 'Tunnel URL changed')
      const desc = ok
        ? t('tunnel.urlChangedCopiedDesc', 'New base URL copied to clipboard.')
        : t('tunnel.urlChangedDesc', 'Update your client to use the new URL.')
      toast(`${title}: ${baseURL}`, {
        description: desc,
        duration: 15000,
        action: {
          label: t('common.copy'),
          onClick: () => void copyToClipboard(baseURL),
        },
      })
    }).then((unsub) => {
      off = unsub
    })
    return () => {
      if (off) off()
    }
  }, [t])
  return null
}

export function App() {
  const [status, setStatus] = useState<'up' | 'down' | 'starting'>('starting')

  useEffect(() => {
    let cancelled = false
    async function check() {
      const info = await systemInfo()
      if (cancelled) return
      setStatus(info && info.httpAddr ? 'up' : 'down')
    }
    check()
    const id = setInterval(check, 5000)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [])

  return (
    <HashRouter>
      <TooltipProvider delayDuration={150}>
      <TrayNavigationBridge />
      <TunnelUrlChangeWatcher />
      <div className="flex h-screen overflow-hidden bg-background text-foreground selection:bg-primary/30">
        {/* Subtle background noise/gradient for premium feel */}
        <div className="pointer-events-none fixed inset-0 z-0 bg-[radial-gradient(ellipse_80%_80%_at_50%_-20%,rgba(120,119,198,0.15),rgba(255,255,255,0))]" />
        
        <div className="z-10 flex h-full w-full">
          <Sidebar />
          <div className="flex flex-1 flex-col overflow-hidden relative">
            <Topbar status={status} />
            <main className="flex-1 overflow-y-auto p-8 prism-scroll">
              <div className="mx-auto max-w-6xl">
                <Routes>
                  <Route path="/" element={<DashboardPage />} />
                  <Route path="/channels" element={<ChannelsPage />} />
                  <Route path="/tokens" element={<TokensPage />} />
                  <Route path="/tunnel" element={<TunnelPage />} />
                  <Route path="/logs" element={<LogsPage />} />
                  <Route path="/settings" element={<SettingsPage />} />
                </Routes>
              </div>
            </main>
          </div>
        </div>
        <OnboardingDialog />
      </div>
      </TooltipProvider>
    </HashRouter>
  )
}
