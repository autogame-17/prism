import { useEffect, useState } from 'react'
import { Copy, ExternalLink, Play, RefreshCw, Square } from 'lucide-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { copyToClipboard, openExternal, systemInfo } from '@/lib/wails'
import { useTunnel } from '@/hooks/useTunnel'
import { useI18n } from '@/lib/i18n'
import { toast } from 'sonner'

export function DashboardPage() {
  const [info, setInfo] = useState<Awaited<ReturnType<typeof systemInfo>>>(null)
  const tunnel = useTunnel()
  const t = useI18n((s) => s.t)

  useEffect(() => {
    let cancelled = false
    async function pull() {
      const i = await systemInfo()
      if (!cancelled) setInfo(i)
    }
    pull()
    const id = setInterval(pull, 5000)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [])

  const publicUrl = tunnel.snap?.url ?? ''
  const baseURL = publicUrl ? `${publicUrl}/v1` : ''

  async function copyBase() {
    if (!baseURL) return
    const ok = await copyToClipboard(baseURL)
    toast[ok ? 'success' : 'error'](ok ? t('dashboard.baseCopied') : t('dashboard.copyFailed'))
  }

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader className="flex-row items-start justify-between gap-2">
            <div>
              <CardTitle>{t('dashboard.publicEndpoint')}</CardTitle>
              <CardDescription>{t('dashboard.publicDesc')}</CardDescription>
            </div>
            <TunnelBadge status={tunnel.snap?.status ?? 'stopped'} />
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="rounded-lg border bg-muted/40 p-4">
              <div className="text-xs uppercase tracking-wider text-muted-foreground">
                {t('dashboard.baseUrlLabel')}
              </div>
              <div className="mt-2 flex items-center gap-2">
                <code className="flex-1 truncate font-mono text-sm">
                  {baseURL || t('dashboard.tunnelNotStarted')}
                </code>
                <Button variant="outline" size="sm" onClick={copyBase} disabled={!baseURL}>
                  <Copy className="h-3.5 w-3.5" /> {t('common.copy')}
                </Button>
                {baseURL && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => openExternal(publicUrl)}
                  >
                    <ExternalLink className="h-3.5 w-3.5" />
                  </Button>
                )}
              </div>
            </div>

            <div className="flex flex-wrap gap-2">
              {tunnel.snap?.status === 'running' ? (
                <>
                  <Button variant="secondary" onClick={tunnel.rotate} disabled={tunnel.busy}>
                    <RefreshCw className="h-4 w-4" /> {t('dashboard.rotate')}
                  </Button>
                  <Button variant="outline" onClick={tunnel.stop} disabled={tunnel.busy}>
                    <Square className="h-4 w-4" /> {t('dashboard.stop')}
                  </Button>
                </>
              ) : (
                <Button onClick={tunnel.start} disabled={tunnel.busy}>
                  <Play className="h-4 w-4" /> {t('dashboard.start')}
                </Button>
              )}
            </div>

            {tunnel.snap?.lastError && (
              <div className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
                {tunnel.snap.lastError}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('dashboard.core')}</CardTitle>
            <CardDescription>{t('dashboard.coreDesc')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <Row label={t('dashboard.http')} value={info?.httpAddr ?? '—'} mono />
            <Row label={t('dashboard.localPort')} value={String(tunnel.snap?.localPort ?? 0)} />
            <Row label={t('dashboard.channels')} value={String(info?.totalChannels ?? 0)} />
            <Row label={t('dashboard.tokens')} value={String(info?.totalTokens ?? 0)} />
            <Row label={t('dashboard.version')} value={info?.version ?? '—'} />
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t('dashboard.nextSteps')}</CardTitle>
          <CardDescription>{t('dashboard.nextStepsDesc')}</CardDescription>
        </CardHeader>
      </Card>
    </div>
  )
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className={mono ? 'font-mono text-xs' : 'font-medium'}>{value}</span>
    </div>
  )
}

function TunnelBadge({ status }: { status: string }) {
  const t = useI18n((s) => s.t)
  if (status === 'running') return <Badge variant="success">{t('common.running')}</Badge>
  if (status === 'starting') return <Badge variant="warning">{t('common.starting')}</Badge>
  if (status === 'error') return <Badge variant="destructive">{t('common.error')}</Badge>
  return <Badge variant="outline">{t('common.stopped')}</Badge>
}
