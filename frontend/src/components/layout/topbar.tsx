import { useLocation } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import { useI18n } from '@/lib/i18n'

const titleKeyMap: Record<string, string> = {
  '/': 'nav.dashboard',
  '/channels': 'nav.channels',
  '/tokens': 'nav.tokens',
  '/tunnel': 'nav.tunnel',
  '/logs': 'nav.logs',
  '/settings': 'nav.settings',
}

export function Topbar({ status }: { status: 'up' | 'down' | 'starting' }) {
  const { pathname } = useLocation()
  const { t, locale, setLocale } = useI18n()
  const title = t(titleKeyMap[pathname] ?? 'nav.dashboard')

  const pill = {
    up: <Badge variant="success">{t('core.up')}</Badge>,
    starting: <Badge variant="warning">{t('core.starting')}</Badge>,
    down: <Badge variant="destructive">{t('core.down')}</Badge>,
  }[status]

  const segBtn = (active: boolean) =>
    'rounded-[5px] px-2.5 py-0.5 text-[11px] font-medium transition-colors ' +
    (active
      ? 'bg-background text-foreground shadow-sm ring-1 ring-border/70'
      : 'text-muted-foreground hover:text-foreground')

  return (
    <header
      className="flex h-[52px] items-center justify-between border-b border-border/50 bg-background/80 px-6 backdrop-blur-md"
      style={{ '--wails-draggable': 'drag' } as any}
    >
      <h1 className="text-lg font-semibold tracking-tight">{title}</h1>
      <div className="flex items-center gap-3" style={{ '--wails-draggable': 'no-drag' } as any}>
        <div className="flex items-center gap-0.5 rounded-md border border-border/70 bg-muted/40 p-0.5">
          <button className={segBtn(locale === 'en')} onClick={() => setLocale('en')}>
            EN
          </button>
          <button className={segBtn(locale === 'zh')} onClick={() => setLocale('zh')}>
            中
          </button>
        </div>
        {pill}
      </div>
    </header>
  )
}
