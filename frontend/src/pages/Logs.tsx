import { type ReactNode, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, Loader2, RefreshCw, Terminal } from 'lucide-react'
import {
  onEvent,
  requestLogs,
  startSystemLogStream,
  systemLogs,
  tracesGet,
  tracesList,
  tracesPurgeOlderThan,
  type RequestLogRow,
  type SystemLogEntry,
  type TraceDetail,
  type TraceSummary,
} from '@/lib/wails'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { PrettyRequest, PrettyResponse } from '@/components/trace-pretty'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from 'sonner'
import { useI18n } from '@/lib/i18n'

type Tab = 'traces' | 'request' | 'system'

export function LogsPage() {
  const t = useI18n((s) => s.t)
  const [tab, setTab] = useState<Tab>('traces')

  return (
    <div className="flex h-full flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">{t('logs.title')}</h2>
          <p className="text-sm text-muted-foreground">{t('logs.desc')}</p>
        </div>
        <div className="flex items-center gap-2 rounded-md border bg-card p-1">
          <Button
            size="sm"
            variant={tab === 'traces' ? 'default' : 'ghost'}
            onClick={() => setTab('traces')}
          >
            {t('logs.tab.traces')}
          </Button>
          <Button
            size="sm"
            variant={tab === 'request' ? 'default' : 'ghost'}
            onClick={() => setTab('request')}
          >
            {t('logs.tab.request')}
          </Button>
          <Button
            size="sm"
            variant={tab === 'system' ? 'default' : 'ghost'}
            onClick={() => setTab('system')}
          >
            {t('logs.tab.system')}
          </Button>
        </div>
      </div>
      {tab === 'traces' ? <TracesView /> : tab === 'request' ? <RequestLogsView /> : <SystemLogsView />}
    </div>
  )
}

function TracesView() {
  const t = useI18n((s) => s.t)
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [model, setModel] = useState('')
  const [onlyErrors, setOnlyErrors] = useState(false)
  const [selectedId, setSelectedId] = useState<number | null>(null)

  const q = useQuery({
    queryKey: ['traces', page, keyword, model, onlyErrors],
    queryFn: () =>
      tracesList({
        page,
        pageSize: 30,
        keyword: keyword || undefined,
        model: model || undefined,
        onlyErrors: onlyErrors || undefined,
      }),
    refetchInterval: 5_000,
  })

  const rows = q.data?.items ?? []
  const total = q.data?.total ?? 0
  const pageSize = q.data?.pageSize ?? 30
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  const purgeAll = async () => {
    if (!confirm(t('logs.traces.purgeConfirm'))) return
    try {
      const n = await tracesPurgeOlderThan(0)
      toast.success(t('logs.traces.purgeDone').replace('{n}', String(n)))
      // Optimistically blank the visible list so the user sees the wipe
      // immediately, even if a chatty client (e.g. Cursor) is still
      // generating new traces and the 5s refetchInterval would otherwise
      // refill the table before they could verify the purge worked.
      qc.setQueriesData({ queryKey: ['traces'] }, (old: unknown) => {
        const prev = (old ?? {}) as { items?: unknown[]; total?: number; page?: number; pageSize?: number }
        return { ...prev, items: [], total: 0 }
      })
      qc.invalidateQueries({ queryKey: ['traces'] })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          value={keyword}
          onChange={(e) => {
            setKeyword(e.target.value)
            setPage(1)
          }}
          placeholder={t('logs.traces.keyword')}
          className="w-64"
        />
        <Input
          value={model}
          onChange={(e) => {
            setModel(e.target.value)
            setPage(1)
          }}
          placeholder={t('logs.filter.model')}
          className="w-48"
        />
        <label className="flex cursor-pointer items-center gap-1 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={onlyErrors}
            onChange={(e) => {
              setOnlyErrors(e.target.checked)
              setPage(1)
            }}
          />
          {t('logs.traces.onlyErrors')}
        </label>
        <Button
          variant="outline"
          size="icon"
          onClick={() => qc.invalidateQueries({ queryKey: ['traces'] })}
          title={t('common.refresh')}
        >
          <RefreshCw className="h-4 w-4" />
        </Button>
        <div className="flex-1" />
        <Button size="sm" variant="outline" onClick={purgeAll}>
          {t('logs.traces.purgeAll')}
        </Button>
      </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-28">{t('logs.col.time')}</TableHead>
              <TableHead className="w-16">{t('logs.traces.status')}</TableHead>
              <TableHead>{t('logs.traces.path')}</TableHead>
              <TableHead>{t('logs.col.model')}</TableHead>
              <TableHead>{t('logs.col.channel')}</TableHead>
              <TableHead className="text-right">{t('logs.col.duration')}</TableHead>
              <TableHead className="text-right">{t('logs.traces.size')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {q.isLoading ? (
              <TableRow>
                <TableCell colSpan={7} className="h-40 text-center text-muted-foreground">
                  <Loader2 className="mr-2 inline h-4 w-4 animate-spin" /> {t('common.loading')}
                </TableCell>
              </TableRow>
            ) : rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="h-40 text-center text-muted-foreground">
                  {t('logs.traces.empty')}
                </TableCell>
              </TableRow>
            ) : (
              rows.map((r) => <TraceRowItem key={r.id} row={r} onOpen={setSelectedId} />)
            )}
          </TableBody>
        </Table>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>
          {total} · {page} / {totalPages}
        </span>
        <div className="flex gap-1">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            {t('common.prev')}
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
          >
            {t('common.next')}
          </Button>
        </div>
      </div>
      <TraceDetailDialog id={selectedId} onClose={() => setSelectedId(null)} />
    </div>
  )
}

function TraceRowItem({ row, onOpen }: { row: TraceSummary; onOpen: (id: number) => void }) {
  const when = useMemo(() => new Date(row.createdAt * 1000).toLocaleTimeString(), [row.createdAt])
  const ok = row.status >= 200 && row.status < 400
  return (
    <TableRow className="cursor-pointer" onClick={() => onOpen(row.id)}>
      <TableCell className="font-mono text-xs text-muted-foreground">{when}</TableCell>
      <TableCell>
        <Badge variant={ok ? 'secondary' : 'destructive'} className="font-mono text-[10px]">
          {row.status || 'ERR'}
        </Badge>
      </TableCell>
      <TableCell className="max-w-[360px] truncate font-mono text-xs text-muted-foreground" title={`${row.method} ${row.path}`}>
        <span className="text-foreground">{row.method}</span> {row.path}
        {row.isStream && <span className="ml-2 text-[10px] text-cyan-400">STREAM</span>}
      </TableCell>
      <TableCell className="font-mono text-xs">{row.model || '—'}</TableCell>
      <TableCell className="text-xs">{row.channelName || row.channelId || '—'}</TableCell>
      <TableCell className="text-right text-xs">{row.durationMs}ms</TableCell>
      <TableCell className="text-right text-xs text-muted-foreground">
        {formatBytes(row.requestBytes)} / {formatBytes(row.responseBytes)}
      </TableCell>
    </TableRow>
  )
}

function TraceDetailDialog({ id, onClose }: { id: number | null; onClose: () => void }) {
  const t = useI18n((s) => s.t)
  const q = useQuery({
    queryKey: ['trace', id],
    queryFn: () => (id ? tracesGet(id) : Promise.resolve(null)),
    enabled: id !== null,
  })
  const d: TraceDetail | null = q.data ?? null
  // Pretty 模式开关：默认开（同事场景里 cursor-opus-4-7 那种 200K+ 的请求体
  // 直接看 raw 完全没法读）。两个开关相互独立，各自记进 localStorage 让用户
  // 下次打开 Prism 还是同样的偏好；用 try/catch 保护因为某些 wails 嵌入环境
  // 里 localStorage 可能受限。
  const [reqPretty, setReqPretty] = useState<boolean>(() => readPref('prism.tracePretty.request', true))
  const [respPretty, setRespPretty] = useState<boolean>(() => readPref('prism.tracePretty.response', true))
  useEffect(() => {
    writePref('prism.tracePretty.request', reqPretty)
  }, [reqPretty])
  useEffect(() => {
    writePref('prism.tracePretty.response', respPretty)
  }, [respPretty])

  const copy = (s: string) => {
    navigator.clipboard.writeText(s).then(
      () => toast.success(t('common.copied')),
      () => toast.error(t('common.copyFailed'))
    )
  }
  return (
    <Dialog open={id !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="w-[min(90vw,960px)] max-w-[90vw] overflow-hidden">
        <DialogHeader>
          <DialogTitle>{t('logs.traces.detailTitle')}</DialogTitle>
        </DialogHeader>
        {!d ? (
          <div className="py-8 text-center text-muted-foreground">
            <Loader2 className="mr-2 inline h-4 w-4 animate-spin" /> {t('common.loading')}
          </div>
        ) : (
          <div className="flex max-h-[70vh] flex-col gap-3 overflow-y-auto prism-scroll">
            <div className="grid grid-cols-2 gap-2 text-xs">
              <Info label={t('logs.col.time')} value={new Date(d.createdAt * 1000).toLocaleString()} />
              <Info label={t('logs.traces.status')} value={String(d.status)} />
              <Info label={t('logs.traces.path')} value={`${d.method} ${d.path}`} />
              <Info label={t('logs.col.duration')} value={`${d.durationMs}ms`} />
              <Info label={t('logs.col.model')} value={d.model || '—'} />
              <Info label={t('logs.col.channel')} value={d.channelName || String(d.channelId) || '—'} />
              <Info label={t('logs.col.token')} value={d.tokenName || '—'} />
              <Info label="Stream" value={d.isStream ? 'yes' : 'no'} />
            </div>
            {d.errorMessage && (
              <div className="rounded-md border border-red-500/40 bg-red-500/10 p-2 text-xs text-red-300">
                {d.errorMessage}
              </div>
            )}
            <BodyBlock
              title={t('logs.traces.request')}
              body={d.requestBody}
              pretty={reqPretty}
              onPrettyChange={setReqPretty}
              renderPretty={(raw) => <PrettyRequest raw={raw} />}
              onCopy={() => copy(d.requestBody)}
            />
            <BodyBlock
              title={t('logs.traces.response')}
              body={d.responseBody}
              pretty={respPretty}
              onPrettyChange={setRespPretty}
              renderPretty={(raw) => <PrettyResponse raw={raw} />}
              onCopy={() => copy(d.responseBody)}
            />
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('common.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function readPref(key: string, fallback: boolean): boolean {
  try {
    const v = localStorage.getItem(key)
    if (v === '0') return false
    if (v === '1') return true
  } catch {
    /* noop */
  }
  return fallback
}

function writePref(key: string, value: boolean) {
  try {
    localStorage.setItem(key, value ? '1' : '0')
  } catch {
    /* noop */
  }
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5 rounded-md border bg-muted/40 p-2">
      <span className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</span>
      <span className="truncate font-mono text-xs" title={value}>{value}</span>
    </div>
  )
}

function BodyBlock({
  title,
  body,
  pretty,
  onPrettyChange,
  renderPretty,
  onCopy,
}: {
  title: string
  body: string
  pretty: boolean
  onPrettyChange: (v: boolean) => void
  renderPretty: (raw: string) => ReactNode
  onCopy: () => void
}) {
  const t = useI18n((s) => s.t)
  const formatted = useMemo(() => prettyJSONOrRaw(body), [body])
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center justify-between gap-2 text-xs font-medium">
        <span>
          {title}{' '}
          <span className="text-muted-foreground">({formatBytes(body.length)})</span>
        </span>
        <div className="flex items-center gap-2">
          <label className="flex cursor-pointer items-center gap-1.5 text-[11px] text-muted-foreground">
            <Switch checked={pretty} onCheckedChange={onPrettyChange} />
            <span>{t('logs.traces.pretty.toggle')}</span>
          </label>
          <Button size="sm" variant="outline" onClick={onCopy}>
            Copy
          </Button>
        </div>
      </div>
      {pretty ? (
        body ? (
          <div className="prism-selectable prism-scroll max-h-[40vh] overflow-auto rounded-md border border-border/60 bg-[#0b0e14] p-2 text-slate-200">
            {renderPretty(body)}
          </div>
        ) : (
          <div className="rounded-md border border-border/60 bg-[#0b0e14] p-2 text-[11px] text-muted-foreground">
            —
          </div>
        )
      ) : (
        <pre className="prism-selectable prism-scroll max-h-[28vh] overflow-auto whitespace-pre-wrap break-all rounded-md border border-border/60 bg-[#0b0e14] p-2 font-mono text-[11px] leading-5 text-slate-200">
          {formatted || '—'}
        </pre>
      )}
    </div>
  )
}

function formatBytes(n: number): string {
  if (!n) return '0B'
  if (n < 1024) return `${n}B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)}K`
  return `${(n / 1024 / 1024).toFixed(1)}M`
}

function prettyJSONOrRaw(s: string): string {
  if (!s) return ''
  try {
    const v = JSON.parse(s)
    return JSON.stringify(v, null, 2)
  } catch {
    return s
  }
}

function RequestLogsView() {
  const t = useI18n((s) => s.t)
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [modelName, setModelName] = useState('')
  const [tokenName, setTokenName] = useState('')

  const q = useQuery({
    queryKey: ['request-logs', page, modelName, tokenName],
    queryFn: () =>
      requestLogs({ page, pageSize: 20, modelName: modelName || undefined, tokenName: tokenName || undefined }),
    refetchInterval: 15_000,
  })

  const rows = q.data?.items ?? []
  const total = q.data?.total ?? 0
  const pageSize = q.data?.pageSize ?? 20
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-end gap-2">
        <Input
          value={modelName}
          onChange={(e) => {
            setModelName(e.target.value)
            setPage(1)
          }}
          placeholder={t('logs.filter.model')}
          className="w-48"
        />
        <Input
          value={tokenName}
          onChange={(e) => {
            setTokenName(e.target.value)
            setPage(1)
          }}
          placeholder={t('logs.filter.token')}
          className="w-48"
        />
        <Button
          variant="outline"
          size="icon"
          onClick={() => qc.invalidateQueries({ queryKey: ['request-logs'] })}
          title={t('common.refresh')}
        >
          <RefreshCw className="h-4 w-4" />
        </Button>
      </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-28">{t('logs.col.time')}</TableHead>
              <TableHead>{t('logs.col.model')}</TableHead>
              <TableHead>{t('logs.col.channel')}</TableHead>
              <TableHead>{t('logs.col.token')}</TableHead>
              <TableHead className="text-right">{t('logs.col.prompt')}</TableHead>
              <TableHead className="text-right">{t('logs.col.completion')}</TableHead>
              <TableHead className="text-right">{t('logs.col.duration')}</TableHead>
              <TableHead className="text-right">{t('logs.col.quota')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {q.isLoading ? (
              <TableRow>
                <TableCell colSpan={8} className="h-40 text-center text-muted-foreground">
                  <Loader2 className="mr-2 inline h-4 w-4 animate-spin" /> {t('common.loading')}
                </TableCell>
              </TableRow>
            ) : rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={8} className="h-40 text-center text-muted-foreground">
                  {t('logs.request.emptyFull')}
                </TableCell>
              </TableRow>
            ) : (
              rows.map((r) => (
                <RequestLogRowItem key={r.id} row={r} />
              ))
            )}
          </TableBody>
        </Table>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>
          {total} · {page} / {totalPages}
        </span>
        <div className="flex gap-1">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            {t('common.prev')}
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
          >
            {t('common.next')}
          </Button>
        </div>
      </div>
    </div>
  )
}

function RequestLogRowItem({ row }: { row: RequestLogRow }) {
  const [open, setOpen] = useState(false)
  const when = useMemo(() => new Date(row.createdAt * 1000).toLocaleTimeString(), [row.createdAt])
  return (
    <>
      <TableRow
        className="cursor-pointer"
        onClick={() => setOpen((v) => !v)}
      >
        <TableCell className="font-mono text-xs text-muted-foreground">{when}</TableCell>
        <TableCell className="font-mono text-xs">{row.modelName || '—'}</TableCell>
        <TableCell className="text-xs">{row.channelName || row.channelId}</TableCell>
        <TableCell className="text-xs">{row.tokenName || '—'}</TableCell>
        <TableCell className="text-right text-xs">{row.promptTokens}</TableCell>
        <TableCell className="text-right text-xs">{row.completionTokens}</TableCell>
        <TableCell className="text-right text-xs">{row.requestTime}ms</TableCell>
        <TableCell className="text-right text-xs">{row.quota}</TableCell>
      </TableRow>
      {open && row.content && (
        <TableRow>
          <TableCell colSpan={8} className="bg-muted/30">
            <pre className="prism-selectable prism-scroll max-h-48 overflow-auto whitespace-pre-wrap break-all text-xs text-muted-foreground">
              {row.content}
            </pre>
          </TableCell>
        </TableRow>
      )}
    </>
  )
}

function SystemLogsView() {
  const t = useI18n((s) => s.t)
  const [lines, setLines] = useState<SystemLogEntry[]>([])
  const [paused, setPaused] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let cancelled = false
    let unsub: (() => void) | null = null
    ;(async () => {
      const initial = await systemLogs(200)
      if (cancelled) return
      setLines(initial)
      await startSystemLogStream()
      unsub = await onEvent<SystemLogEntry>('log.system', (e) => {
        setLines((prev) => {
          const next = [...prev, e]
          if (next.length > 1000) next.splice(0, next.length - 1000)
          return next
        })
      })
    })()
    return () => {
      cancelled = true
      if (unsub) unsub()
    }
  }, [])

  useEffect(() => {
    if (paused) return
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [lines, paused])

  // Render the buffered log lines into a single plain-text blob so the
  // "Copy all" button hands the user something useful regardless of which
  // text the renderer happens to have selected. Memoised because `lines`
  // can grow to 1000 entries and toString-ing it on every keystroke
  // elsewhere would be wasteful.
  const plainText = useMemo(
    () =>
      lines
        .map((l) => `${new Date(l.timestamp).toLocaleTimeString()} ${l.level} ${l.message}`)
        .join('\n'),
    [lines]
  )

  const copyAll = async () => {
    if (!plainText) {
      toast.error(t('logs.system.empty'))
      return
    }
    try {
      await navigator.clipboard.writeText(plainText)
      toast.success(
        t('logs.system.copyDone').replace('{n}', String(lines.length))
      )
    } catch {
      toast.error(t('common.copyFailed'))
    }
  }

  return (
    <div className="flex flex-1 flex-col gap-3 overflow-hidden">
      <div className="flex items-center gap-2">
        <Terminal className="h-4 w-4 text-muted-foreground" />
        <Badge variant="outline">
          {lines.length} {t('logs.system.lines')}
        </Badge>
        <Button size="sm" variant="outline" onClick={() => setPaused((p) => !p)}>
          {paused ? t('logs.system.resume') : t('logs.system.pause')}
        </Button>
        <Button size="sm" variant="outline" onClick={() => setLines([])}>
          {t('logs.system.clear')}
        </Button>
        <div className="ml-auto">
          <Button size="sm" variant="outline" onClick={copyAll} disabled={lines.length === 0}>
            <Copy className="mr-1 h-3.5 w-3.5" />
            {t('logs.system.copyAll')}
          </Button>
        </div>
      </div>
      <div
        ref={scrollRef}
        className="prism-selectable prism-scroll flex-1 overflow-auto rounded-md border border-border/60 bg-[#0b0e14] p-3 font-mono text-xs text-green-200/90"
      >
        {lines.length === 0 ? (
          <div className="text-muted-foreground">{t('logs.system.empty')}</div>
        ) : (
          lines.map((l, i) => (
            <div key={i} className="whitespace-pre-wrap">
              <span className="text-muted-foreground">
                {new Date(l.timestamp).toLocaleTimeString()}
              </span>{' '}
              <span
                className={
                  l.level === 'ERR' || l.level === 'FATAL'
                    ? 'text-red-400'
                    : l.level === 'WARN'
                      ? 'text-yellow-300'
                      : 'text-cyan-300'
                }
              >
                {l.level}
              </span>{' '}
              <span>{l.message}</span>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
