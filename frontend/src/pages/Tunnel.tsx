import { useEffect, useRef, useState } from 'react'
import { Copy, Play, RefreshCw, Square } from 'lucide-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { copyToClipboard, onEvent, tunnelLogs } from '@/lib/wails'
import { useTunnel } from '@/hooks/useTunnel'
import { useI18n } from '@/lib/i18n'
import { toast } from 'sonner'

export function TunnelPage() {
  const t = useI18n((s) => s.t)
  const tunnel = useTunnel()
  const [logs, setLogs] = useState<string[]>([])
  const boxRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    let disposed = false
    async function seed() {
      const initial = await tunnelLogs()
      if (!disposed) setLogs(initial)
    }
    seed()
    const offPromise = onEvent<string>('tunnel.log', (line) => {
      setLogs((prev) => {
        const next = [...prev, line.trimEnd()]
        if (next.length > 500) next.splice(0, next.length - 500)
        return next
      })
    })
    return () => {
      disposed = true
      offPromise.then((off) => off())
    }
  }, [])

  useEffect(() => {
    if (boxRef.current) {
      boxRef.current.scrollTop = boxRef.current.scrollHeight
    }
  }, [logs])

  const url = tunnel.snap?.url ?? ''

  async function copyUrl() {
    if (!url) return
    const ok = await copyToClipboard(url)
    toast[ok ? 'success' : 'error'](ok ? t('tunnel.urlCopied') : t('tunnel.copyFailed'))
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex-row items-start justify-between gap-2">
          <div>
            <CardTitle>{t('tunnel.cardTitle')}</CardTitle>
            <CardDescription>{t('tunnel.cardDesc')}</CardDescription>
          </div>
          <TunnelBadge status={tunnel.snap?.status ?? 'stopped'} />
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            <Field label={t('tunnel.publicUrl')} value={url || t('tunnel.notStarted')} mono />
            <Field label={t('tunnel.localPort')} value={String(tunnel.snap?.localPort ?? 0)} />
            <Field
              label={t('tunnel.binary')}
              value={tunnel.snap?.binaryPath || '—'}
              mono
              ellipsis
            />
          </div>
          <div className="flex flex-wrap gap-2">
            {tunnel.snap?.status === 'running' ? (
              <>
                <Button variant="secondary" onClick={tunnel.rotate} disabled={tunnel.busy}>
                  <RefreshCw className="h-4 w-4" /> {t('tunnel.rotate')}
                </Button>
                <Button variant="outline" onClick={tunnel.stop} disabled={tunnel.busy}>
                  <Square className="h-4 w-4" /> {t('tunnel.stop')}
                </Button>
                <Button variant="ghost" onClick={copyUrl}>
                  <Copy className="h-4 w-4" /> {t('tunnel.copy')}
                </Button>
              </>
            ) : (
              <Button onClick={tunnel.start} disabled={tunnel.busy}>
                <Play className="h-4 w-4" /> {t('tunnel.start')}
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
          <CardTitle>{t('tunnel.logsTitle')}</CardTitle>
          <CardDescription>{t('tunnel.logsDesc')}</CardDescription>
        </CardHeader>
        <CardContent>
          <div
            ref={boxRef}
            className="prism-scroll h-72 overflow-y-auto rounded-md border border-border/60 bg-[#0b0e14] p-3 font-mono text-[11px] leading-relaxed text-emerald-200"
          >
            {logs.length === 0 ? (
              <span className="text-white/40">{t('tunnel.noLogs')}</span>
            ) : (
              logs.map((line, idx) => (
                <div key={idx} className="whitespace-pre-wrap break-all">
                  {line}
                </div>
              ))
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

function Field({
  label,
  value,
  mono,
  ellipsis,
}: {
  label: string
  value: string
  mono?: boolean
  ellipsis?: boolean
}) {
  return (
    <div className="min-w-0 rounded-lg border border-border/60 bg-background p-3 shadow-[0_1px_2px_rgba(0,0,0,0.03)] dark:bg-white/[0.02]">
      <div className="text-[11px] uppercase tracking-wider text-muted-foreground">
        {label}
      </div>
      <div
        className={
          (mono ? 'font-mono text-xs ' : 'text-sm font-medium ') +
          (ellipsis ? 'truncate ' : 'break-all ') +
          'mt-1 text-foreground'
        }
        title={ellipsis ? value : undefined}
      >
        {value}
      </div>
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
