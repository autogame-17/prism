import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Loader2, Plus, RefreshCw, Pencil, Trash2, Play, Power } from 'lucide-react'
import {
  channelsCreate,
  channelsDelete,
  channelsFetchModels,
  channelsList,
  channelsTest,
  channelsToggle,
  channelsUpdate,
  providerTypes,
  type ChannelPayload,
  type ChannelSummary,
  type ProviderMeta,
} from '@/lib/wails'
import { useI18n } from '@/lib/i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
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

type ChannelFormState = ChannelPayload

const STATUS_KEYS: Record<number, { key: string; variant: 'default' | 'destructive' | 'secondary' | 'outline' }> = {
  1: { key: 'channels.status.enabled', variant: 'default' },
  2: { key: 'channels.status.disabled', variant: 'secondary' },
  3: { key: 'channels.status.autoOff', variant: 'destructive' },
  4: { key: 'channels.status.testing', variant: 'outline' },
}

function emptyForm(typeFallback: number): ChannelFormState {
  return {
    id: 0,
    type: typeFallback || 1,
    name: '',
    key: '',
    baseURL: '',
    other: '',
    models: '',
    group: 'default',
    testModel: '',
    proxy: '',
    priority: 0,
    weight: 0,
    status: 1,
  }
}

export function ChannelsPage() {
  const qc = useQueryClient()
  const t = useI18n((s) => s.t)
  const [keyword, setKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [editorOpen, setEditorOpen] = useState(false)
  const [form, setForm] = useState<ChannelFormState>(emptyForm(1))
  const [isEditing, setIsEditing] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<ChannelSummary | null>(null)

  const providersQ = useQuery({
    queryKey: ['provider-types'],
    queryFn: providerTypes,
    staleTime: 1000 * 60 * 60,
  })

  const providers: ProviderMeta[] = providersQ.data ?? []
  const providerMap = useMemo(() => new Map(providers.map((p) => [p.type, p.name])), [providers])

  const listQ = useQuery({
    queryKey: ['channels', page, keyword],
    queryFn: () => channelsList({ page, pageSize: 20, name: keyword }),
  })

  const createMu = useMutation({
    mutationFn: (payload: ChannelPayload) => channelsCreate(payload),
    onSuccess: () => {
      toast.success(t('channels.createOk'))
      qc.invalidateQueries({ queryKey: ['channels'] })
      setEditorOpen(false)
    },
    onError: (err: unknown) => toast.error(String(err)),
  })

  const updateMu = useMutation({
    mutationFn: (payload: ChannelPayload) => channelsUpdate(payload),
    onSuccess: () => {
      toast.success(t('channels.updateOk'))
      qc.invalidateQueries({ queryKey: ['channels'] })
      setEditorOpen(false)
    },
    onError: (err: unknown) => toast.error(String(err)),
  })

  const deleteMu = useMutation({
    mutationFn: (id: number) => channelsDelete(id),
    onSuccess: () => {
      toast.success(t('channels.deleteOk'))
      setDeleteTarget(null)
      if ((listQ.data?.items.length ?? 0) <= 1 && page > 1) {
        setPage((p) => Math.max(1, p - 1))
      }
      qc.invalidateQueries({ queryKey: ['channels'] })
    },
    onError: (err: unknown) => toast.error(String(err)),
  })

  const toggleMu = useMutation({
    mutationFn: (id: number) => channelsToggle(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['channels'] }),
    onError: (err: unknown) => toast.error(String(err)),
  })

  const testMu = useMutation({
    mutationFn: ({ id, model }: { id: number; model: string }) => channelsTest(id, model),
    onSuccess: (res) => {
      if (res.success) toast.success(res.message)
      else toast.error(res.message)
    },
    onError: (err: unknown) => toast.error(String(err)),
  })

  const onNew = () => {
    setIsEditing(false)
    setForm(emptyForm(providers[0]?.type ?? 1))
    setEditorOpen(true)
  }

  const onEdit = (c: ChannelSummary) => {
    setIsEditing(true)
    setForm({
      id: c.id,
      type: c.type,
      name: c.name,
      key: '',
      baseURL: c.baseURL,
      other: '',
      models: c.models,
      group: c.group,
      testModel: c.testModel,
      proxy: c.proxy,
      priority: c.priority,
      weight: c.weight,
      status: c.status,
    })
    setEditorOpen(true)
  }

  const onSubmit = () => {
    if (isEditing) updateMu.mutate(form)
    else createMu.mutate(form)
  }

  const rows = listQ.data?.items ?? []
  const total = listQ.data?.total ?? 0
  const pageSize = listQ.data?.pageSize ?? 20
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <div className="flex h-full flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">{t('channels.title')}</h2>
          <p className="text-sm text-muted-foreground">{t('channels.desc')}</p>
        </div>
        <div className="flex items-center gap-2">
          <Input
            value={keyword}
            onChange={(e) => {
              setKeyword(e.target.value)
              setPage(1)
            }}
            placeholder={t('channels.searchPlaceholder')}
            className="w-56"
          />
          <Button
            variant="outline"
            size="icon"
            onClick={() => qc.invalidateQueries({ queryKey: ['channels'] })}
            title={t('common.refresh')}
          >
            <RefreshCw className="h-4 w-4" />
          </Button>
          <Button onClick={onNew}>
            <Plus className="mr-1 h-4 w-4" /> {t('common.new')}
          </Button>
        </div>
      </div>

      <Table className="min-w-[980px]">
          <TableHeader>
            <TableRow>
              <TableHead className="w-12">{t('common.id')}</TableHead>
              <TableHead>{t('common.name')}</TableHead>
              <TableHead>{t('common.type')}</TableHead>
              <TableHead>{t('common.status')}</TableHead>
              <TableHead>{t('channels.col.models')}</TableHead>
              <TableHead className="text-right">{t('channels.col.priority')}</TableHead>
              <TableHead className="sticky right-0 z-10 w-44 min-w-44 bg-card/95 text-right backdrop-blur">
                {t('common.actions')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {listQ.isLoading ? (
              <TableRow>
                <TableCell colSpan={7} className="h-40 text-center text-muted-foreground">
                  <Loader2 className="mr-2 inline h-4 w-4 animate-spin" /> {t('common.loading')}
                </TableCell>
              </TableRow>
            ) : rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="h-40 text-center text-muted-foreground">
                  {t('channels.empty')}
                </TableCell>
              </TableRow>
            ) : (
              rows.map((c) => {
                const s = STATUS_KEYS[c.status] ?? { key: 'common.status', variant: 'outline' as const }
                return (
                  <TableRow key={c.id}>
                    <TableCell className="font-mono text-xs text-muted-foreground">{c.id}</TableCell>
                    <TableCell className="max-w-[160px] truncate font-medium" title={c.name}>{c.name}</TableCell>
                    <TableCell className="text-sm">{providerMap.get(c.type) ?? c.type}</TableCell>
                    <TableCell>
                      <Badge variant={s.variant}>{t(s.key)}</Badge>
                    </TableCell>
                    <TableCell className="max-w-[280px] truncate text-xs text-muted-foreground">
                      {c.models || '—'}
                    </TableCell>
                    <TableCell className="text-right text-sm">{c.priority}</TableCell>
                    <TableCell className="sticky right-0 z-10 w-44 min-w-44 bg-card/95">
                      <div className="flex min-w-[9.75rem] justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="icon"
                          title={t('channels.test')}
                          onClick={() => testMu.mutate({ id: c.id, model: c.testModel })}
                          disabled={testMu.isPending}
                        >
                          <Play className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          title={t('channels.toggle')}
                          onClick={() => toggleMu.mutate(c.id)}
                        >
                          <Power className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          title={t('common.edit')}
                          onClick={() => onEdit(c)}
                        >
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          title={t('common.delete')}
                          onClick={() => setDeleteTarget(c)}
                          disabled={deleteMu.isPending && deleteTarget?.id === c.id}
                        >
                          {deleteMu.isPending && deleteTarget?.id === c.id ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            <Trash2 className="h-4 w-4" />
                          )}
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

      <ChannelEditorDialog
        open={editorOpen}
        onOpenChange={setEditorOpen}
        form={form}
        setForm={setForm}
        providers={providers}
        isEditing={isEditing}
        submitting={createMu.isPending || updateMu.isPending}
        onSubmit={onSubmit}
      />

      <Dialog open={deleteTarget !== null} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <DialogContent className="w-[min(90vw,420px)]">
          <DialogHeader>
            <DialogTitle>{t('common.delete')}</DialogTitle>
            <DialogDescription className="break-words">
              {t('channels.confirmDelete')}
              {deleteTarget?.name ? ` (${deleteTarget.name})` : ''}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteTarget(null)}
              disabled={deleteMu.isPending}
            >
              {t('common.cancel')}
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleteTarget && deleteMu.mutate(deleteTarget.id)}
              disabled={deleteMu.isPending || deleteTarget === null}
            >
              {deleteMu.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

type EditorProps = {
  open: boolean
  onOpenChange: (v: boolean) => void
  form: ChannelFormState
  setForm: (f: ChannelFormState) => void
  providers: ProviderMeta[]
  isEditing: boolean
  submitting: boolean
  onSubmit: () => void
}

function ChannelEditorDialog({
  open,
  onOpenChange,
  form,
  setForm,
  providers,
  isEditing,
  submitting,
  onSubmit,
}: EditorProps) {
  const t = useI18n((s) => s.t)
  const hints = providerHints(form.type)
  const update = <K extends keyof ChannelFormState>(key: K, value: ChannelFormState[K]) =>
    setForm({ ...form, [key]: value })

  // fetchModelsMu probes the upstream's /v1/models for the *currently edited*
  // form (without saving the channel). On success we merge the pulled names
  // with whatever the user has already typed and dedupe — typing custom
  // aliases like "cursor-opus-4-7" alongside the upstream list is the
  // common case for Cursor / Codex / etc clients.
  const fetchModelsMu = useMutation({
    mutationFn: () => channelsFetchModels(form),
    onSuccess: (models) => {
      const existing = form.models
        .split(/[\s,]+/)
        .map((s) => s.trim())
        .filter(Boolean)
      const seen = new Set<string>()
      const merged: string[] = []
      for (const name of [...existing, ...models]) {
        if (!seen.has(name)) {
          seen.add(name)
          merged.push(name)
        }
      }
      setForm({ ...form, models: merged.join(',') })
      toast.success(t('channels.editor.fetchModelsDone').replace('{n}', String(models.length)))
    },
    onError: (err: unknown) => toast.error(err instanceof Error ? err.message : String(err)),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[min(90vw,640px)] max-w-[90vw] overflow-hidden">
        <DialogHeader className="min-w-0">
          <DialogTitle>
            {isEditing ? t('channels.editor.editTitle') : t('channels.editor.newTitle')}
          </DialogTitle>
          <DialogDescription className="break-words">{hints.description}</DialogDescription>
        </DialogHeader>

        <div className="grid max-h-[60vh] min-w-0 gap-4 overflow-y-auto overflow-x-hidden pr-2 prism-scroll">
          <div className="grid grid-cols-2 gap-3">
            <div className="min-w-0 space-y-1">
              <Label>{t('channels.editor.provider')}</Label>
              <Select
                value={String(form.type)}
                onChange={(e) => update('type', Number(e.target.value))}
                className="w-full"
              >
                {providers.map((p) => (
                  <option key={p.type} value={p.type}>
                    {p.name}
                  </option>
                ))}
              </Select>
            </div>
            <div className="min-w-0 space-y-1">
              <Label>{t('channels.editor.name')}</Label>
              <Input
                value={form.name}
                onChange={(e) => update('name', e.target.value)}
                placeholder="openai-prod"
                className="w-full"
              />
            </div>
          </div>

          <div className="min-w-0 space-y-1">
            <Label>{hints.keyLabel}</Label>
            <Input
              value={form.key}
              onChange={(e) => update('key', e.target.value)}
              placeholder={hints.keyPlaceholder}
              type="password"
              autoComplete="off"
              className="w-full"
            />
            <p className="break-words text-xs text-muted-foreground">{hints.keyHelp}</p>
          </div>

          {hints.showBaseURL && (
            <div className="min-w-0 space-y-1">
              <Label>{t('channels.editor.baseUrl')}</Label>
              <Input
                value={form.baseURL}
                onChange={(e) => update('baseURL', e.target.value)}
                placeholder={hints.baseURLPlaceholder}
                className="w-full"
              />
            </div>
          )}

          {hints.showOther && (
            <div className="min-w-0 space-y-1">
              <Label>{hints.otherLabel}</Label>
              <Input
                value={form.other}
                onChange={(e) => update('other', e.target.value)}
                placeholder={hints.otherPlaceholder}
                className="w-full"
              />
              <p className="break-words text-xs text-muted-foreground">{hints.otherHelp}</p>
            </div>
          )}

          <div className="min-w-0 space-y-1">
            <div className="flex items-center justify-between gap-2">
              <Label>{t('channels.editor.models')}</Label>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => fetchModelsMu.mutate()}
                disabled={fetchModelsMu.isPending}
              >
                {fetchModelsMu.isPending ? (
                  <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
                ) : (
                  <RefreshCw className="mr-1 h-3.5 w-3.5" />
                )}
                {t('channels.editor.fetchModels')}
              </Button>
            </div>
            <Textarea
              rows={3}
              value={form.models}
              onChange={(e) => update('models', e.target.value)}
              placeholder="gpt-4o-mini,gpt-4o"
              className="w-full"
            />
            <p className="text-xs text-muted-foreground">
              {t('channels.editor.modelsHelp')}
            </p>
          </div>

          <div className="grid grid-cols-3 gap-3">
            <div className="min-w-0 space-y-1">
              <Label>{t('channels.editor.group')}</Label>
              <Input
                value={form.group}
                onChange={(e) => update('group', e.target.value)}
                className="w-full"
              />
            </div>
            <div className="min-w-0 space-y-1">
              <Label>{t('channels.editor.priority')}</Label>
              <Input
                type="number"
                value={form.priority}
                onChange={(e) => update('priority', Number(e.target.value))}
                className="w-full"
              />
            </div>
            <div className="min-w-0 space-y-1">
              <Label>{t('channels.editor.weight')}</Label>
              <Input
                type="number"
                value={form.weight}
                onChange={(e) => update('weight', Number(e.target.value))}
                className="w-full"
              />
            </div>
          </div>

          <div className="min-w-0 space-y-1">
            <Label>{t('channels.editor.testModel')}</Label>
            <Input
              value={form.testModel}
              onChange={(e) => update('testModel', e.target.value)}
              placeholder="gpt-4o-mini"
              className="w-full"
            />
          </div>

          <div className="min-w-0 space-y-1 sm:col-span-2">
            <Label>{t('channels.editor.proxy')}</Label>
            <Input
              value={form.proxy}
              onChange={(e) => update('proxy', e.target.value)}
              placeholder="http://127.0.0.1:7890 or socks5://127.0.0.1:1080"
              className="w-full"
            />
            <p className="text-xs text-muted-foreground">{t('channels.editor.proxyHelp')}</p>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button onClick={onSubmit} disabled={submitting}>
            {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {isEditing ? t('common.save') : t('common.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

type ProviderHints = {
  description: string
  keyLabel: string
  keyPlaceholder: string
  keyHelp: string
  showBaseURL: boolean
  baseURLPlaceholder: string
  showOther: boolean
  otherLabel: string
  otherPlaceholder: string
  otherHelp: string
}

function providerHints(type: number): ProviderHints {
  switch (type) {
    case 25:
      return {
        description: 'Google Gemini uses an API key from AI Studio.',
        keyLabel: 'API key',
        keyPlaceholder: 'AIza...',
        keyHelp: 'Create at https://aistudio.google.com/app/apikey',
        showBaseURL: false,
        baseURLPlaceholder: '',
        showOther: false,
        otherLabel: '',
        otherPlaceholder: '',
        otherHelp: '',
      }
    case 14:
      return {
        description: 'Anthropic Claude direct API.',
        keyLabel: 'API key',
        keyPlaceholder: 'sk-ant-...',
        keyHelp: 'Create at https://console.anthropic.com/settings/keys',
        showBaseURL: false,
        baseURLPlaceholder: '',
        showOther: false,
        otherLabel: '',
        otherPlaceholder: '',
        otherHelp: '',
      }
    case 32:
      return {
        description:
          'Amazon Bedrock (AWS Signature V4 auth). Key format: region|AccessKeyID|SecretAccessKey (optionally append |SessionToken). Models must be the one-api Claude IDs.',
        keyLabel: 'Bedrock credentials',
        keyPlaceholder: 'us-east-1|AKIA...|wJalrXUt...',
        keyHelp:
          'Three segments separated by "|": AWS region, Access Key ID, Secret Access Key. Append a fourth segment for a temporary session token if needed.',
        showBaseURL: false,
        baseURLPlaceholder: '',
        showOther: false,
        otherLabel: '',
        otherPlaceholder: '',
        otherHelp: '',
      }
    case 3:
      return {
        description: 'Azure OpenAI deployment.',
        keyLabel: 'API key',
        keyPlaceholder: 'azure key',
        keyHelp: 'Find in Azure portal → Keys and endpoint.',
        showBaseURL: true,
        baseURLPlaceholder: 'https://<your-resource>.openai.azure.com',
        showOther: true,
        otherLabel: 'API version',
        otherPlaceholder: '2024-06-01',
        otherHelp: 'Azure OpenAI api-version value.',
      }
    case 42:
      return {
        description: 'Google Vertex AI service account.',
        keyLabel: 'Service-account JSON',
        keyPlaceholder: 'Paste the full JSON',
        keyHelp: 'From GCP → IAM → Service Accounts → Keys.',
        showBaseURL: false,
        baseURLPlaceholder: '',
        showOther: true,
        otherLabel: 'Region',
        otherPlaceholder: 'us-central1',
        otherHelp: 'Vertex region (e.g. us-central1).',
      }
    case 1:
      return {
        description: 'OpenAI official API.',
        keyLabel: 'API key',
        keyPlaceholder: 'sk-...',
        keyHelp: 'From platform.openai.com → API keys.',
        showBaseURL: false,
        baseURLPlaceholder: '',
        showOther: false,
        otherLabel: '',
        otherPlaceholder: '',
        otherHelp: '',
      }
    case 8:
    default:
      return {
        description: 'Generic OpenAI-compatible endpoint.',
        keyLabel: 'API key',
        keyPlaceholder: 'sk-...',
        keyHelp: 'Use the provider key. Leave Base URL empty for defaults.',
        showBaseURL: true,
        baseURLPlaceholder: 'https://api.provider.com',
        showOther: false,
        otherLabel: '',
        otherPlaceholder: '',
        otherHelp: '',
      }
  }
}
