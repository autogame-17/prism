import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  Check,
  ClipboardCopy,
  Eye,
  EyeOff,
  Loader2,
  Plus,
  Power,
  RefreshCw,
  Terminal,
  Trash2,
  Wand2,
} from 'lucide-react'
import {
  copyToClipboard,
  systemInfo,
  tokensCreate,
  tokensDelete,
  tokensList,
  tokensRename,
  tokensSnippet,
  tokensToggle,
  tunnelBaseURL,
  type TokenSummary,
} from '@/lib/wails'
import { useI18n } from '@/lib/i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Select } from '@/components/ui/select'

const STATUS_KEYS: Record<number, { key: string; variant: 'default' | 'secondary' | 'destructive' }> = {
  1: { key: 'channels.status.enabled', variant: 'default' },
  2: { key: 'channels.status.disabled', variant: 'secondary' },
  3: { key: 'tokens.status.expired', variant: 'destructive' },
  4: { key: 'tokens.status.depleted', variant: 'destructive' },
}

const QUOTA_PER_DOLLAR = 500_000

export function TokensPage() {
  const qc = useQueryClient()
  const t = useI18n((s) => s.t)
  const [keyword, setKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [createOpen, setCreateOpen] = useState(false)
  const [snippetFor, setSnippetFor] = useState<TokenSummary | null>(null)
  const [visibleKeys, setVisibleKeys] = useState<Record<number, boolean>>({})

  const listQ = useQuery({
    queryKey: ['tokens', page, keyword],
    queryFn: () => tokensList({ page, pageSize: 20, keyword }),
  })

  const baseURLQ = useQuery({
    queryKey: ['tunnel-base-url'],
    queryFn: tunnelBaseURL,
    refetchInterval: 5_000,
  })

  const createMu = useMutation({
    mutationFn: (req: { name: string; unlimitedQuota: boolean; quotaDollars: number }) =>
      tokensCreate({
        name: req.name,
        unlimitedQuota: req.unlimitedQuota,
        remainQuota: req.unlimitedQuota ? 0 : Math.round(req.quotaDollars * QUOTA_PER_DOLLAR),
        expiredTime: -1,
        group: 'default',
      }),
    onSuccess: () => {
      toast.success(t('tokens.createOk'))
      qc.invalidateQueries({ queryKey: ['tokens'] })
      setCreateOpen(false)
    },
    onError: (e: unknown) => toast.error(t('tokens.createFailed') + ': ' + String(e)),
  })

  const deleteMu = useMutation({
    mutationFn: (id: number) => tokensDelete(id),
    onSuccess: () => {
      toast.success(t('tokens.deleteOk'))
      qc.invalidateQueries({ queryKey: ['tokens'] })
    },
    onError: (e: unknown) => toast.error(t('tokens.deleteFailed') + ': ' + String(e)),
  })

  const toggleMu = useMutation({
    mutationFn: (id: number) => tokensToggle(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tokens'] }),
    onError: (e: unknown) => toast.error(t('tokens.toggleFailed') + ': ' + String(e)),
  })

  const renameMu = useMutation({
    mutationFn: (v: { id: number; name: string }) => tokensRename(v.id, v.name),
    onSuccess: () => {
      toast.success(t('tokens.renameOk'))
      qc.invalidateQueries({ queryKey: ['tokens'] })
    },
    onError: (e: unknown) => toast.error(t('tokens.renameFailed') + ': ' + String(e)),
  })

  const rows = listQ.data?.items ?? []
  const total = listQ.data?.total ?? 0
  const pageSize = listQ.data?.pageSize ?? 20
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <div className="flex h-full flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">{t('tokens.title')}</h2>
          <p className="text-sm text-muted-foreground">{t('tokens.desc')}</p>
        </div>
        <div className="flex items-center gap-2">
          <Input
            value={keyword}
            onChange={(e) => {
              setKeyword(e.target.value)
              setPage(1)
            }}
            placeholder={t('common.search')}
            className="w-56"
          />
          <Button
            variant="outline"
            size="icon"
            onClick={() => qc.invalidateQueries({ queryKey: ['tokens'] })}
            title={t('common.refresh')}
          >
            <RefreshCw className="h-4 w-4" />
          </Button>
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-1 h-4 w-4" /> {t('common.new')}
          </Button>
        </div>
      </div>

      <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-12">{t('common.id')}</TableHead>
              <TableHead>{t('common.name')}</TableHead>
              <TableHead>{t('common.key')}</TableHead>
              <TableHead>{t('common.status')}</TableHead>
              <TableHead>{t('common.quota')}</TableHead>
              <TableHead className="text-right w-60">{t('common.actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {listQ.isLoading ? (
              <TableRow>
                <TableCell colSpan={6} className="h-40 text-center text-muted-foreground">
                  <Loader2 className="mr-2 inline h-4 w-4 animate-spin" /> {t('common.loading')}
                </TableCell>
              </TableRow>
            ) : rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="h-40 text-center text-muted-foreground">
                  {t('tokens.empty')}
                </TableCell>
              </TableRow>
            ) : (
              rows.map((tok) => {
                const s = STATUS_KEYS[tok.status] ?? { key: 'common.status', variant: 'secondary' as const }
                const visible = visibleKeys[tok.id]
                const displayKey = visible ? 'sk-' + tok.key : 'sk-' + maskKey(tok.key)
                const quotaLabel = tok.unlimitedQuota
                  ? t('tokens.unlimited')
                  : `$${(tok.remainQuota / QUOTA_PER_DOLLAR).toFixed(2)} ${t('tokens.quota.leftSuffix')}`
                return (
                  <TableRow key={tok.id}>
                    <TableCell className="font-mono text-xs text-muted-foreground">{tok.id}</TableCell>
                    <TableCell className="max-w-[180px] font-medium">
                      <InlineRename
                        value={tok.name}
                        onSave={(name) => {
                          if (name && name !== tok.name) renameMu.mutate({ id: tok.id, name })
                        }}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1 font-mono text-xs">
                        <span className="truncate max-w-[220px]">{displayKey}</span>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7"
                          onClick={() => setVisibleKeys((v) => ({ ...v, [tok.id]: !visible }))}
                        >
                          {visible ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7"
                          onClick={async () => {
                            const ok = await copyToClipboard('sk-' + tok.key)
                            if (ok) toast.success(t('tokens.keyCopied'))
                          }}
                        >
                          <ClipboardCopy className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant={s.variant}>{t(s.key)}</Badge>
                    </TableCell>
                    <TableCell className="text-sm">{quotaLabel}</TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="icon"
                          title={t('tokens.snippet')}
                          onClick={() => setSnippetFor(tok)}
                        >
                          <Wand2 className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          title={t('channels.toggle')}
                          onClick={() => toggleMu.mutate(tok.id)}
                        >
                          <Power className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          title={t('common.delete')}
                          onClick={() => {
                            if (confirm(t('tokens.confirmDelete'))) deleteMu.mutate(tok.id)
                          }}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })
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

      <CreateTokenDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        submitting={createMu.isPending}
        onSubmit={(v) => createMu.mutate(v)}
      />

      <SnippetDialog
        token={snippetFor}
        baseURL={baseURLQ.data ?? ''}
        onClose={() => setSnippetFor(null)}
      />
    </div>
  )
}

function InlineRename({ value, onSave }: { value: string; onSave: (v: string) => void }) {
  const t = useI18n((s) => s.t)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value)
  if (!editing) {
    return (
      <button
        className="rounded px-1 hover:bg-muted"
        onDoubleClick={() => {
          setDraft(value)
          setEditing(true)
        }}
        title={t('tokens.rename.hint')}
      >
        {value}
      </button>
    )
  }
  return (
    <div className="flex items-center gap-1">
      <Input
        autoFocus
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            onSave(draft.trim())
            setEditing(false)
          } else if (e.key === 'Escape') {
            setEditing(false)
          }
        }}
        className="h-7"
      />
      <Button
        variant="ghost"
        size="icon"
        className="h-7 w-7"
        onClick={() => {
          onSave(draft.trim())
          setEditing(false)
        }}
      >
        <Check className="h-3.5 w-3.5" />
      </Button>
    </div>
  )
}

function maskKey(key: string): string {
  if (key.length <= 8) return '********'
  return key.slice(0, 4) + '••••••••' + key.slice(-4)
}

function CreateTokenDialog({
  open,
  onOpenChange,
  submitting,
  onSubmit,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
  submitting: boolean
  onSubmit: (v: { name: string; unlimitedQuota: boolean; quotaDollars: number }) => void
}) {
  const t = useI18n((s) => s.t)
  const [name, setName] = useState('')
  const [unlimited, setUnlimited] = useState(true)
  const [dollars, setDollars] = useState(10)

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) {
          setName('')
          setUnlimited(true)
          setDollars(10)
        }
        onOpenChange(v)
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('tokens.create.title')}</DialogTitle>
          <DialogDescription>{t('tokens.create.desc')}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="space-y-1">
            <Label>{t('common.name')}</Label>
            <Input
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="cursor-mac"
            />
          </div>
          <div className="flex items-center justify-between rounded-md border p-3">
            <div>
              <p className="text-sm font-medium">{t('tokens.create.unlimited')}</p>
              <p className="text-xs text-muted-foreground">{t('tokens.quota.unlimitedDesc')}</p>
            </div>
            <Switch checked={unlimited} onCheckedChange={setUnlimited} />
          </div>
          {!unlimited && (
            <div className="space-y-1">
              <Label>{t('tokens.quota.dollars')}</Label>
              <Input
                type="number"
                value={dollars}
                min={0}
                step={1}
                onChange={(e) => setDollars(Number(e.target.value))}
              />
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button
            disabled={submitting || !name.trim()}
            onClick={() =>
              onSubmit({ name: name.trim(), unlimitedQuota: unlimited, quotaDollars: dollars })
            }
          >
            {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {t('common.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

type SnippetClient = 'cursor' | 'cline' | 'cherry-studio' | 'openai-sdk'

function SnippetDialog({
  token,
  baseURL,
  onClose,
}: {
  token: TokenSummary | null
  baseURL: string
  onClose: () => void
}) {
  const t = useI18n((s) => s.t)
  const [client, setClient] = useState<SnippetClient>('cursor')
  const infoQ = useQuery({ enabled: !baseURL, queryKey: ['system-info-for-snippet'], queryFn: systemInfo })
  const localBase = infoQ.data?.httpAddr ? `http://${infoQ.data.httpAddr}/v1` : 'http://127.0.0.1:3002/v1'
  const effectiveBase = baseURL || localBase

  const snippetQ = useQuery({
    enabled: !!token,
    queryKey: ['snippet', token?.id, client, effectiveBase],
    queryFn: () =>
      tokensSnippet({ tokenId: token!.id, baseURL: effectiveBase, client }),
  })

  const hasTunnel = useMemo(() => !!baseURL, [baseURL])

  if (!token) return null

  return (
    <Dialog open={!!token} onOpenChange={(v) => (!v ? onClose() : null)}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            <Terminal className="mr-2 inline h-4 w-4" />
            {t('tokens.snippet.for')} — {token.name}
          </DialogTitle>
          <DialogDescription>
            {hasTunnel ? t('tokens.snippet.withTunnel') : t('tokens.snippet.noTunnel')}
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4">
          <div className="space-y-1">
            <Label>{t('tokens.snippet.client')}</Label>
            <Select value={client} onChange={(e) => setClient(e.target.value as SnippetClient)}>
              <option value="cursor">Cursor (Custom OpenAI API Key)</option>
              <option value="cline">Cline</option>
              <option value="cherry-studio">Cherry Studio</option>
              <option value="openai-sdk">OpenAI SDK env vars</option>
            </Select>
          </div>

          <div className="min-w-0 rounded-md border bg-muted/40">
            <div className="flex items-center justify-between gap-2 border-b px-3 py-2">
              <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground" title={effectiveBase}>
                {effectiveBase}
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="shrink-0"
                onClick={async () => {
                  const body = snippetQ.data?.body ?? ''
                  if (!body) return
                  const ok = await copyToClipboard(body)
                  if (ok) toast.success(t('tokens.snippet.copied'))
                }}
              >
                <ClipboardCopy className="mr-1 h-3.5 w-3.5" />
                {t('common.copy')}
              </Button>
            </div>
            <pre className="prism-scroll max-h-[300px] overflow-auto whitespace-pre-wrap break-all p-3 text-xs leading-relaxed">
              {snippetQ.isLoading ? t('common.loading') : snippetQ.data?.body ?? ''}
            </pre>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('common.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
