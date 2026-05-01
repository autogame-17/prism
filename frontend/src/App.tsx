import { useEffect, useState } from 'react'
import { HashRouter, Route, Routes, useNavigate } from 'react-router-dom'
import { Sidebar } from '@/components/layout/sidebar'
import { Topbar } from '@/components/layout/topbar'
import { OnboardingDialog } from '@/components/onboarding'
import { DashboardPage } from '@/pages/Dashboard'
import { ChannelsPage } from '@/pages/Channels'
import { TokensPage } from '@/pages/Tokens'
import { TunnelPage } from '@/pages/Tunnel'
import { LogsPage } from '@/pages/Logs'
import { SettingsPage } from '@/pages/Settings'
import { systemInfo, onEvent } from '@/lib/wails'

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
      <TrayNavigationBridge />
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
    </HashRouter>
  )
}
