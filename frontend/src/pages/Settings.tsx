import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Copy, ExternalLink, FolderOpen, Loader2 } from 'lucide-react'
import {
  openFileDialog,
  saveFileDialog,
  settingsExportToFile,
  settingsGetListenAddr,
  settingsImportFromFile,
  settingsOpenDataDir,
  settingsPaths,
  settingsSetListenAddr,
  systemInfo,
} from '@/lib/wails'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useI18n } from '@/lib/i18n'
import { applyTheme, getStoredTheme, setStoredTheme, trackSystemTheme, type Theme } from '@/lib/theme'

export function SettingsPage() {
  const t = useI18n((s) => s.t)
  const qc = useQueryClient()
  const pathsQ = useQuery({ queryKey: ['prism-paths'], queryFn: settingsPaths })
  const infoQ = useQuery({ queryKey: ['prism-info'], queryFn: systemInfo })
  const listenQ = useQuery({ queryKey: ['prism-listen-addr'], queryFn: settingsGetListenAddr })

  const [theme, setTheme] = useState<Theme>(() => getStoredTheme())
  const [busy, setBusy] = useState(false)
  // Draft value of the listen-addr input. Synced from the query whenever the
  // backend responds, but kept locally so typing isn't blown away by refetches.
  const [listenDraft, setListenDraft] = useState<string>('')
  const [listenSaving, setListenSaving] = useState(false)

  useEffect(() => {
    if (listenQ.data) {
      setListenDraft(listenQ.data.configured || listenQ.data.default || '')
    }
  }, [listenQ.data])

  useEffect(() => {
    applyTheme(theme)
    setStoredTheme(theme)
    trackSystemTheme(theme === 'system')
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

  const onSaveListen = async () => {
    const value = listenDraft.trim()
    setListenSaving(true)
    try {
      await settingsSetListenAddr(value)
      await qc.invalidateQueries({ queryKey: ['prism-listen-addr'] })
      toast.success(t('settings.listen.saved', 'Saved. Restart Prism to take effect.'))
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      toast.error(t('settings.listen.saveFailed', 'Save failed') + ': ' + msg)
    } finally {
      setListenSaving(false)
    }
  }

  const copyToClipboard = async (text: string, okMsg: string) => {
    try {
      await navigator.clipboard.writeText(text)
      toast.success(okMsg)
    } catch {
      toast.error(t('settings.listen.copyFailed', 'Copy failed'))
    }
  }

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
          <CardTitle className="text-base">{t('settings.listen.title', 'Local server')}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3 text-sm">
          <div className="grid gap-1">
            <Label>{t('settings.listen.address', 'Listen address')}</Label>
            <p className="text-xs text-muted-foreground">
              {t(
                'settings.listen.hint',
                'host:port the embedded HTTP server binds to. Keep the port stable so external clients (e.g. Cursor via Cloudflare tunnel) hold the same URL across restarts.'
              )}
            </p>
            <div className="flex gap-2">
              <Input
                value={listenDraft}
                placeholder={listenQ.data?.default ?? '127.0.0.1:39527'}
                onChange={(e) => setListenDraft(e.target.value)}
                spellCheck={false}
                className="font-mono"
              />
              <Button onClick={onSaveListen} disabled={listenSaving}>
                {listenSaving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {t('settings.listen.save', 'Save')}
              </Button>
            </div>
          </div>
          <div className="grid gap-1">
            <Label>{t('settings.listen.actual', 'Currently bound')}</Label>
            <div className="flex items-center gap-2">
              <p className="break-all font-mono text-xs text-muted-foreground">
                {listenQ.data?.actual || '—'}
              </p>
              {listenQ.data?.actual && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() =>
                    copyToClipboard(
                      `http://${listenQ.data!.actual}/v1`,
                      t('settings.listen.copied', 'Copied')
                    )
                  }
                  title={t('settings.listen.copyBaseUrl', 'Copy base URL')}
                >
                  <Copy className="h-3.5 w-3.5" />
                </Button>
              )}
            </div>
            {listenQ.data &&
              listenQ.data.configured &&
              listenQ.data.configured !== listenQ.data.actual && (
                <p className="text-xs text-amber-600 dark:text-amber-400">
                  {t(
                    'settings.listen.mismatch',
                    'Configured address differs from the running one (port likely was busy at startup). Restart Prism to retry.'
                  )}
                </p>
              )}
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
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              onClick={async () => {
                try {
                  await settingsOpenDataDir()
                } catch (err) {
                  const msg = err instanceof Error ? err.message : String(err)
                  toast.error(t('settings.openFailed', 'Open failed') + (msg ? `: ${msg}` : ''))
                }
              }}
            >
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
