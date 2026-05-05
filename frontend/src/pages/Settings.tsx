import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ExternalLink, FolderOpen, Loader2 } from 'lucide-react'
import {
  openFileDialog,
  saveFileDialog,
  settingsExportToFile,
  settingsImportFromFile,
  settingsOpenDataDir,
  settingsPaths,
  systemInfo,
} from '@/lib/wails'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { useI18n } from '@/lib/i18n'

type Theme = 'light' | 'dark' | 'system'

const THEME_KEY = 'prism.theme'

function applyTheme(t: Theme) {
  const root = document.documentElement
  const resolved = t === 'system' ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light') : t
  root.classList.remove('dark', 'light')
  root.classList.add(resolved)
}

export function SettingsPage() {
  const t = useI18n((s) => s.t)
  const pathsQ = useQuery({ queryKey: ['prism-paths'], queryFn: settingsPaths })
  const infoQ = useQuery({ queryKey: ['prism-info'], queryFn: systemInfo })

  const [theme, setTheme] = useState<Theme>(() => {
    const stored = localStorage.getItem(THEME_KEY) as Theme | null
    return stored ?? 'dark'
  })
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    applyTheme(theme)
    localStorage.setItem(THEME_KEY, theme)
  }, [theme])

  const paths = pathsQ.data
  const info = infoQ.data

  const onExport = async () => {
    const path = await saveFileDialog(
      t('settings.exportTitle'),
      `prism-${new Date().toISOString().slice(0, 10)}.json`
    )
    if (!path) return
    setBusy(true)
    try {
      await settingsExportToFile(path)
      toast.success(t('settings.exportOk') + ' ' + path)
    } catch (e) {
      toast.error(t('settings.exportFailed') + ': ' + String(e))
    } finally {
      setBusy(false)
    }
  }

  const onImport = async (replace: boolean) => {
    const path = await openFileDialog(t('settings.importTitle'))
    if (!path) return
    if (replace && !confirm(t('settings.import.confirm'))) return
    setBusy(true)
    try {
      await settingsImportFromFile(path, replace)
      toast.success(t('settings.importOk'))
    } catch (e) {
      toast.error(t('settings.importFailed') + ': ' + String(e))
    } finally {
      setBusy(false)
    }
  }

  const themeLabel = (v: Theme) =>
    v === 'light' ? t('settings.theme.light') : v === 'dark' ? t('settings.theme.dark') : t('settings.theme.system')

  return (
    <div className="flex h-full flex-col gap-4">
      <div>
        <h2 className="text-xl font-semibold">{t('settings.title')}</h2>
        <p className="text-sm text-muted-foreground">{t('settings.desc')}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('settings.appearance')}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">{t('settings.theme')}</p>
              <p className="text-xs text-muted-foreground">{t('settings.themeHint')}</p>
            </div>
            <div className="flex gap-0.5 rounded-md border border-border/70 bg-muted/40 p-0.5">
              {(['light', 'dark', 'system'] as Theme[]).map((v) => (
                <button
                  key={v}
                  className={
                    'rounded-[5px] px-3 py-1 text-xs font-medium transition-colors ' +
                    (theme === v
                      ? 'bg-background text-foreground shadow-sm ring-1 ring-border/70'
                      : 'text-muted-foreground hover:text-foreground')
                  }
                  onClick={() => setTheme(v)}
                >
                  {themeLabel(v)}
                </button>
              ))}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('settings.data')}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3 text-sm">
          <div className="flex items-center justify-between gap-2">
            <div className="min-w-0 flex-1">
              <Label>{t('settings.dataDir')}</Label>
              <p className="break-all font-mono text-xs text-muted-foreground">{paths?.dataDir ?? '—'}</p>
            </div>
            <Button variant="outline" size="sm" className="shrink-0" onClick={() => settingsOpenDataDir()}>
              <FolderOpen className="mr-1 h-4 w-4" /> {t('settings.open')}
            </Button>
          </div>
          <div className="min-w-0">
            <Label>{t('settings.logDir')}</Label>
            <p className="break-all font-mono text-xs text-muted-foreground">{paths?.logDir ?? '—'}</p>
          </div>
          <div className="min-w-0">
            <Label>{t('settings.configFile')}</Label>
            <p className="break-all font-mono text-xs text-muted-foreground">{paths?.configFile ?? '—'}</p>
          </div>

          <div className="mt-2 flex flex-wrap gap-2">
            <Button onClick={onExport} disabled={busy}>
              {busy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('settings.export')}
            </Button>
            <Button variant="outline" onClick={() => onImport(false)} disabled={busy}>
              {t('settings.import.merge')}
            </Button>
            <Button variant="outline" onClick={() => onImport(true)} disabled={busy}>
              {t('settings.import.replace')}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('settings.about')}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2 text-sm">
          <div className="flex justify-between">
            <span className="text-muted-foreground">{t('settings.about.version')}</span>
            <span className="font-mono">{info?.version ?? '—'}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">{t('settings.about.core')}</span>
            <span className="font-mono text-xs">{info?.coreCommit ?? '—'}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">{t('settings.about.localHttp')}</span>
            <span className="font-mono text-xs">{info?.httpAddr ?? '—'}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">{t('settings.about.platform')}</span>
            <span className="font-mono text-xs">
              {paths ? `${paths.os}/${paths.arch}` : '—'}
            </span>
          </div>
          <div className="mt-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => window.open('https://github.com/MartialBE/one-api', '_blank')}
            >
              <ExternalLink className="mr-1 h-4 w-4" /> {t('settings.about.coreFork')}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
