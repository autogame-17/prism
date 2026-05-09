import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Copy, ExternalLink, FolderOpen, Loader2 } from 'lucide-react'
import {
  openFileDialog,
  saveFileDialog,
  settingsExportToFile,
  settingsGetCloudflareTunnel,
  settingsGetListenAddr,
  settingsImportFromFile,
  settingsOpenDataDir,
  settingsPaths,
  settingsSetCloudflareTunnel,
  settingsSetListenAddr,
  systemInfo,
} from '@/lib/wails'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { useI18n } from '@/lib/i18n'
import { applyTheme, getStoredTheme, setStoredTheme, trackSystemTheme, type Theme } from '@/lib/theme'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

export function SettingsPage() {
  const t = useI18n((s) => s.t)
  const qc = useQueryClient()
  const pathsQ = useQuery({ queryKey: ['prism-paths'], queryFn: settingsPaths })
  const infoQ = useQuery({ queryKey: ['prism-info'], queryFn: systemInfo })
  const listenQ = useQuery({ queryKey: ['prism-listen-addr'], queryFn: settingsGetListenAddr })
  const tunnelQ = useQuery({ queryKey: ['prism-cf-tunnel'], queryFn: settingsGetCloudflareTunnel })

  const [theme, setTheme] = useState<Theme>(() => getStoredTheme())
  const [busy, setBusy] = useState(false)
  const [listenDraft, setListenDraft] = useState<string>('')
  const [listenSaving, setListenSaving] = useState(false)
  // Named-tunnel inputs. tokenDraft starts empty even when one is
  // already configured so the actual secret never lives in DOM /
  // React state — it stays on disk in prism.yaml. The user types
  // a fresh token to rotate, or clears both fields to disable.
  const [tunnelTokenDraft, setTunnelTokenDraft] = useState<string>('')
  const [tunnelHostDraft, setTunnelHostDraft] = useState<string>('')
  const [tunnelSaving, setTunnelSaving] = useState(false)
  const [confirmAction, setConfirmAction] = useState<'import-replace' | 'clear-tunnel' | null>(null)
  const [importPath, setImportPath] = useState<string | null>(null)

  useEffect(() => {
    if (listenQ.data) {
      setListenDraft(listenQ.data.configured || listenQ.data.default || '')
    }
  }, [listenQ.data])

  useEffect(() => {
    if (tunnelQ.data) {
      setTunnelHostDraft(tunnelQ.data.hostname || '')
    }
  }, [tunnelQ.data])

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
    if (replace) {
      setImportPath(path)
      setConfirmAction('import-replace')
      return
    }
    await doImport(path, false)
  }

  const doImport = async (path: string, replace: boolean) => {
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

  // Save / clear named-tunnel config. Two modes:
  //   - Both fields filled  → switch to named-tunnel mode
  //   - Both fields empty   → revert to trycloudflare
  // Anything else is rejected backend-side; we surface the error.
  // We pass tokenDraft directly even when the user only re-typed the
  // hostname: the backend treats "" as "leave alone" via the field
  // identity rule (token+hostname must be both set or both empty),
  // so the UI forces re-entry of the token whenever rotating either
  // field. That's a tiny UX cost but keeps DOM free of secrets.
  const onSaveTunnel = async () => {
    const token = tunnelTokenDraft.trim()
    const host = tunnelHostDraft.trim()
    setTunnelSaving(true)
    try {
      await settingsSetCloudflareTunnel(token, host)
      await qc.invalidateQueries({ queryKey: ['prism-cf-tunnel'] })
      // Wipe the token from React state immediately after a
      // successful save so React DevTools / a screen recording
      // can't pick it up later.
      setTunnelTokenDraft('')
      toast.success(t('settings.tunnel.saved', 'Saved. Restart Prism to take effect.'))
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      toast.error(t('settings.tunnel.saveFailed', 'Save failed') + ': ' + msg)
    } finally {
      setTunnelSaving(false)
    }
  }

  const onClearTunnel = () => {
    setConfirmAction('clear-tunnel')
  }

  const doClearTunnel = async () => {
    setTunnelSaving(true)
    try {
      await settingsSetCloudflareTunnel('', '')
      await qc.invalidateQueries({ queryKey: ['prism-cf-tunnel'] })
      setTunnelTokenDraft('')
      setTunnelHostDraft('')
      toast.success(t('settings.tunnel.cleared', 'Cleared. Restart Prism to take effect.'))
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      toast.error(t('settings.tunnel.saveFailed', 'Save failed') + ': ' + msg)
    } finally {
      setTunnelSaving(false)
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
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() =>
                        copyToClipboard(
                          `http://${listenQ.data!.actual}/v1`,
                          t('settings.listen.copied', 'Copied')
                        )
                      }
                    >
                      <Copy className="h-3.5 w-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('settings.listen.copyBaseUrl', 'Copy base URL')}</TooltipContent>
                </Tooltip>
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
          <CardTitle className="text-base">
            {t('settings.tunnel.title', 'Cloudflare named tunnel (stable URL)')}
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3 text-sm">
          <p className="text-xs text-muted-foreground">
            {t(
              'settings.tunnel.hint',
              'Trycloudflare gives a fresh random URL on every restart. Switch to a named tunnel to keep one stable hostname forever — requires a Cloudflare account, a domain, and a tunnel token from the Zero Trust dashboard.'
            )}
          </p>
          <div className="grid gap-1">
            <Label>{t('settings.tunnel.hostname', 'Public hostname')}</Label>
            <Input
              placeholder="prism.example.com"
              value={tunnelHostDraft}
              onChange={(e) => setTunnelHostDraft(e.target.value)}
              spellCheck={false}
              className="font-mono"
            />
            <p className="text-[11px] text-muted-foreground">
              {t(
                'settings.tunnel.hostnameHint',
                'Bare host, no scheme. Must match the public hostname you assigned to the tunnel in Cloudflare.'
              )}
            </p>
          </div>
          <div className="grid gap-1">
            <Label>{t('settings.tunnel.token', 'Tunnel token')}</Label>
            <Input
              type="password"
              placeholder={
                tunnelQ.data?.hasToken
                  ? t('settings.tunnel.tokenPlaceholderConfigured', 'Token already configured — type a new one to rotate')
                  : t('settings.tunnel.tokenPlaceholder', 'Paste the long token from cloudflared')
              }
              value={tunnelTokenDraft}
              onChange={(e) => setTunnelTokenDraft(e.target.value)}
              autoComplete="off"
              spellCheck={false}
              className="font-mono"
            />
            <p className="text-[11px] text-muted-foreground">
              {t(
                'settings.tunnel.tokenHint',
                'Token is stored in prism.yaml and never echoed back to this UI in cleartext.'
              )}
            </p>
          </div>
          <div className="flex flex-wrap gap-2 pt-1">
            <Button onClick={onSaveTunnel} disabled={tunnelSaving}>
              {tunnelSaving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('settings.tunnel.save', 'Save')}
            </Button>
            {tunnelQ.data?.hasToken && (
              <Button variant="outline" onClick={onClearTunnel} disabled={tunnelSaving} className="text-amber-600 hover:text-amber-700 hover:bg-amber-500/10 border-amber-500/30 dark:text-amber-400">
                {t('settings.tunnel.clear', 'Disable named tunnel')}
              </Button>
            )}
            <span className="ml-auto self-center text-xs text-muted-foreground">
              {tunnelQ.data?.hasToken
                ? t('settings.tunnel.statusConfigured', 'Named tunnel configured')
                : t('settings.tunnel.statusOff', 'Using trycloudflare (random URL)')}
            </span>
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
            <Button variant="outline" onClick={() => onImport(true)} disabled={busy} className="text-amber-600 hover:text-amber-700 hover:bg-amber-500/10 border-amber-500/30 dark:text-amber-400">
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
      {/* Confirm dialog: import replace */}
      <AlertDialog open={confirmAction === 'import-replace'} onOpenChange={(open) => { if (!open) setConfirmAction(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('settings.import.replace')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('settings.import.confirm')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('settings.cancel', 'Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={async () => {
                setConfirmAction(null)
                if (importPath) await doImport(importPath, true)
              }}
            >
              {t('settings.import.replace')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Confirm dialog: clear tunnel config */}
      <AlertDialog open={confirmAction === 'clear-tunnel'} onOpenChange={(open) => { if (!open) setConfirmAction(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('settings.tunnel.clear', 'Disable named tunnel')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('settings.tunnel.clearConfirm', 'This will remove the named tunnel configuration and revert to trycloudflare (random URL). Are you sure?')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('settings.cancel', 'Cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => { setConfirmAction(null); doClearTunnel() }}>
              {t('settings.tunnel.clear', 'Disable named tunnel')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
