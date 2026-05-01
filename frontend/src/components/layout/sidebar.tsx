import { NavLink } from 'react-router-dom'
import {
  LayoutDashboard,
  Plug,
  KeyRound,
  Waypoints,
  ScrollText,
  Settings as SettingsIcon,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useI18n } from '@/lib/i18n'
import logoUrl from '@/assets/logo.png'

type NavEntry = {
  to: string
  labelKey: string
  icon: React.ComponentType<{ className?: string }>
}

const entries: NavEntry[] = [
  { to: '/', labelKey: 'nav.dashboard', icon: LayoutDashboard },
  { to: '/channels', labelKey: 'nav.channels', icon: Plug },
  { to: '/tokens', labelKey: 'nav.tokens', icon: KeyRound },
  { to: '/tunnel', labelKey: 'nav.tunnel', icon: Waypoints },
  { to: '/logs', labelKey: 'nav.logs', icon: ScrollText },
  { to: '/settings', labelKey: 'nav.settings', icon: SettingsIcon },
]

export function Sidebar() {
  const t = useI18n((s) => s.t)
  return (
    <aside
      className={cn(
        'flex h-full w-[240px] flex-col border-r border-border/60',
        'bg-white/70 supports-[backdrop-filter]:bg-white/55 backdrop-blur-xl',
        'dark:bg-black/30 dark:supports-[backdrop-filter]:bg-black/20'
      )}
    >
      <div className="h-8 w-full" style={{ '--wails-draggable': 'drag' } as any} />

      <div
        className="flex items-center gap-3 px-5 py-4"
        style={{ '--wails-draggable': 'drag' } as any}
      >
        <img
          src={logoUrl}
          alt="Prism"
          className="h-14 w-14 rounded-[16px] shadow-[0_6px_20px_rgba(0,0,0,0.14)] ring-1 ring-black/10 dark:ring-white/10"
          draggable={false}
        />
        <div className="flex flex-col leading-tight">
          <span className="text-base font-semibold tracking-tight text-foreground">Prism</span>
          <span className="mt-1 text-[10px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
            Gateway
          </span>
        </div>
      </div>

      <nav className="flex-1 space-y-0.5 px-3 py-2">
        {entries.map((e) => (
          <NavLink
            key={e.to}
            to={e.to}
            end={e.to === '/'}
            className={({ isActive }) =>
              cn(
                'group flex items-center gap-3 rounded-lg px-3 py-2 text-[13px] font-medium transition-colors duration-150',
                isActive
                  ? cn(
                      'bg-foreground/[0.06] text-foreground',
                      'dark:bg-white/10 dark:text-foreground',
                      'shadow-[inset_0_0_0_1px_rgba(0,0,0,0.04)]',
                      'dark:shadow-[inset_0_0_0_1px_rgba(255,255,255,0.06)]'
                    )
                  : 'text-muted-foreground hover:bg-foreground/[0.04] hover:text-foreground dark:hover:bg-white/5'
              )
            }
          >
            {({ isActive }) => (
              <>
                <e.icon
                  className={cn(
                    'h-4 w-4 transition-colors',
                    isActive ? 'text-foreground' : 'opacity-60 group-hover:opacity-90'
                  )}
                />
                {t(e.labelKey)}
              </>
            )}
          </NavLink>
        ))}
      </nav>

      <div className="p-5 text-[10px] font-medium text-muted-foreground/60">
        v0.1.0 · one-hub core
      </div>
    </aside>
  )
}
