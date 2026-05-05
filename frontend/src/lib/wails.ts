// Thin, typed wrappers around the Wails-generated JS bindings. We import
// these *statically* so Vite bundles them into the production assets folder.
// Dynamic imports would try to load `../../wailsjs/...` at runtime, but only
// `frontend/dist/` is embedded into the Wails binary, so those requests 404
// and every binding silently fails (manifests as "core offline" in the UI).

import * as SystemAPIRaw from '../../wailsjs/go/bindings/SystemAPI'
import * as TunnelAPIRaw from '../../wailsjs/go/bindings/TunnelAPI'
import * as ChannelsAPIRaw from '../../wailsjs/go/bindings/ChannelsAPI'
import * as TokensAPIRaw from '../../wailsjs/go/bindings/TokensAPI'
import * as LogsAPIRaw from '../../wailsjs/go/bindings/LogsAPI'
import * as TracesAPIRaw from '../../wailsjs/go/bindings/TracesAPI'
import * as SettingsAPIRaw from '../../wailsjs/go/bindings/SettingsAPI'
import * as WailsRuntime from '../../wailsjs/runtime/runtime'

// The generated bindings delegate to `window.go.bindings.X.Y(...)`. Wails
// injects that object at runtime but it may not be ready at first render.
async function whenReady(timeoutMs = 8000): Promise<boolean> {
  const start = Date.now()
  while (Date.now() - start < timeoutMs) {
    const g = (globalThis as unknown as { go?: { bindings?: Record<string, unknown> } }).go
    if (g && g.bindings) return true
    await new Promise((r) => setTimeout(r, 40))
  }
  return false
}

async function safe<T>(fn: () => Promise<T> | T, fallback: T): Promise<T> {
  if (!(await whenReady())) return fallback
  try {
    return await fn()
  } catch {
    return fallback
  }
}

async function strict<T>(fn: () => Promise<T> | T): Promise<T> {
  if (!(await whenReady())) throw new Error('wails runtime not ready')
  return await fn()
}

// Generated bindings have strict required-field types while the app mostly
// passes partial filter objects. Cast sites through `anyCall` rather than
// spraying `any` at every call.
const anyCall = <T>(fn: (...args: unknown[]) => Promise<T> | T, ...args: unknown[]) =>
  (fn as unknown as (...a: unknown[]) => Promise<T>)(...args)

export async function onEvent<T>(event: string, handler: (data: T) => void): Promise<() => void> {
  if (!(await whenReady())) return () => {}
  return WailsRuntime.EventsOn(event, (raw: unknown) => handler(raw as T))
}

export async function copyToClipboard(text: string): Promise<boolean> {
  if (await whenReady()) {
    try {
      return await WailsRuntime.ClipboardSetText(text)
    } catch {
      // fall through to navigator.clipboard
    }
  }
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

export async function openExternal(url: string): Promise<void> {
  if (await whenReady()) {
    WailsRuntime.BrowserOpenURL(url)
    return
  }
  window.open(url, '_blank', 'noreferrer,noopener')
}

// We use Go-side file dialogs (SettingsAPI.PickExportPath / PickImportPath)
// because the generated runtime JS does not expose dialog helpers.
export async function openFileDialog(_title = 'Open', _patterns = '*.json'): Promise<string | null> {
  void _title
  void _patterns
  if (!(await whenReady())) return null
  try {
    const p = await SettingsAPIRaw.PickImportPath()
    return p || null
  } catch {
    return null
  }
}

export async function saveFileDialog(
  _title = 'Save',
  defaultName = 'prism-export.json',
  _patterns = '*.json'
): Promise<string | null> {
  void _title
  void _patterns
  if (!(await whenReady())) return null
  try {
    const p = await SettingsAPIRaw.PickExportPath(defaultName)
    return p || null
  } catch {
    return null
  }
}

// ------------------------------ System -----------------------------------

export type SystemInfo = {
  version: string
  coreCommit: string
  dataDir: string
  logDir: string
  httpAddr: string
  startedAt: number
  totalChannels: number
  totalTokens: number
}

export function systemInfo(): Promise<SystemInfo | null> {
  return safe(async () => (await SystemAPIRaw.Info()) as unknown as SystemInfo, null)
}

// ------------------------------ Tunnel -----------------------------------

export type TunnelSnapshot = {
  status: 'stopped' | 'starting' | 'running' | 'error'
  url: string
  startedAt: number
  localPort: number
  lastError?: string
  binaryPath: string
}

export function tunnelStatus(): Promise<TunnelSnapshot | null> {
  return safe(async () => (await TunnelAPIRaw.Status()) as unknown as TunnelSnapshot, null)
}

export function tunnelStart(): Promise<string> {
  return strict(async () => (await TunnelAPIRaw.Start()) as string)
}

export async function tunnelStop(): Promise<void> {
  await safe(async () => {
    await TunnelAPIRaw.Stop()
    return undefined
  }, undefined)
}

export function tunnelRotate(): Promise<string> {
  return strict(async () => (await TunnelAPIRaw.Rotate()) as string)
}

export function tunnelBaseURL(): Promise<string> {
  return safe(async () => (await TunnelAPIRaw.BaseURL()) as string, '')
}

export function tunnelLogs(): Promise<string[]> {
  return safe(async () => (await TunnelAPIRaw.Logs()) as string[], [])
}

// ------------------------------ Channels ---------------------------------

export type ChannelSummary = {
  id: number
  type: number
  name: string
  status: number
  priority: number
  weight: number
  models: string
  group: string
  baseURL: string
  testModel: string
  proxy: string
  responseTime: number
  balance: number
  usedQuota: number
  createdTime: number
}

export type ChannelPayload = {
  id?: number
  type: number
  name: string
  key: string
  baseURL: string
  other: string
  models: string
  group: string
  testModel: string
  proxy: string
  priority: number
  weight: number
  status: number
}

export type ProviderMeta = { type: number; name: string }

export function channelsList(req: {
  page?: number
  pageSize?: number
  name?: string
  type?: number
  status?: number
  orderBy?: string
  sortBy?: string
}): Promise<{ items: ChannelSummary[]; total: number; page: number; pageSize: number }> {
  return safe(
    async () =>
      (await anyCall(ChannelsAPIRaw.List as never, req)) as {
        items: ChannelSummary[]
        total: number
        page: number
        pageSize: number
      },
    { items: [], total: 0, page: 1, pageSize: 20 }
  )
}

export function channelsCreate(p: ChannelPayload): Promise<unknown> {
  return strict(() => anyCall(ChannelsAPIRaw.Create as never, p))
}

export function channelsUpdate(p: ChannelPayload): Promise<unknown> {
  return strict(() => anyCall(ChannelsAPIRaw.Update as never, p))
}

export function channelsDelete(id: number): Promise<unknown> {
  return strict(() => ChannelsAPIRaw.Delete(id))
}

export function channelsToggle(id: number): Promise<number> {
  return strict(async () => (await ChannelsAPIRaw.Toggle(id)) as number)
}

export function channelsTest(
  id: number,
  modelName = ''
): Promise<{ success: boolean; responseTime: number; message: string }> {
  return strict(
    async () =>
      (await ChannelsAPIRaw.Test(id, modelName)) as unknown as {
        success: boolean
        responseTime: number
        message: string
      }
  )
}

export function providerTypes(): Promise<ProviderMeta[]> {
  return safe(async () => (await ChannelsAPIRaw.ListProviderTypes()) as unknown as ProviderMeta[], [])
}

// ------------------------------ Tokens -----------------------------------

export type TokenSummary = {
  id: number
  name: string
  key: string
  status: number
  group: string
  createdTime: number
  expiredTime: number
  unlimitedQuota: boolean
  remainQuota: number
  usedQuota: number
}

export function tokensList(req: {
  page?: number
  pageSize?: number
  keyword?: string
}): Promise<{ items: TokenSummary[]; total: number; page: number; pageSize: number }> {
  return safe(
    async () =>
      (await anyCall(TokensAPIRaw.List as never, req)) as {
        items: TokenSummary[]
        total: number
        page: number
        pageSize: number
      },
    { items: [], total: 0, page: 1, pageSize: 20 }
  )
}

export function tokensCreate(req: {
  name: string
  group?: string
  unlimitedQuota?: boolean
  remainQuota?: number
  expiredTime?: number
}): Promise<TokenSummary> {
  return strict(async () => (await anyCall(TokensAPIRaw.Create as never, req)) as TokenSummary)
}

export function tokensDelete(id: number): Promise<unknown> {
  return strict(() => TokensAPIRaw.Delete(id))
}

export function tokensToggle(id: number): Promise<number> {
  return strict(async () => (await TokensAPIRaw.Toggle(id)) as number)
}

export function tokensRename(id: number, name: string): Promise<unknown> {
  return strict(() => TokensAPIRaw.Rename(id, name))
}

export function tokensSnippet(req: {
  tokenId: number
  baseURL: string
  client: 'cursor' | 'cline' | 'cherry-studio' | 'openai-sdk'
}): Promise<{ client: string; format: string; body: string }> {
  return strict(
    async () =>
      (await anyCall(TokensAPIRaw.ClientSnippet as never, req)) as {
        client: string
        format: string
        body: string
      }
  )
}

// ------------------------------ Logs -------------------------------------

export type SystemLogEntry = { timestamp: number; level: string; message: string }

export function systemLogs(n = 200): Promise<SystemLogEntry[]> {
  return safe(async () => (await LogsAPIRaw.SystemLogs(n)) as unknown as SystemLogEntry[], [])
}

export async function startSystemLogStream(): Promise<void> {
  await safe(async () => {
    await LogsAPIRaw.StartSystemLogStream()
    return undefined
  }, undefined)
}

export type RequestLogRow = {
  id: number
  createdAt: number
  type: number
  username: string
  modelName: string
  tokenName: string
  channelId: number
  channelName: string
  quota: number
  promptTokens: number
  completionTokens: number
  requestTime: number
  isStream: boolean
  sourceIp: string
  content: string
}

export function requestLogs(req: {
  page?: number
  pageSize?: number
  logType?: number
  modelName?: string
  username?: string
  tokenName?: string
  channelId?: number
  startTimestamp?: number
  endTimestamp?: number
}): Promise<{ items: RequestLogRow[]; total: number; page: number; pageSize: number }> {
  return safe(
    async () =>
      (await anyCall(LogsAPIRaw.RequestLogs as never, req)) as {
        items: RequestLogRow[]
        total: number
        page: number
        pageSize: number
      },
    { items: [], total: 0, page: 1, pageSize: 20 }
  )
}

// ------------------------------ Traces -----------------------------------

export type TraceSummary = {
  id: number
  createdAt: number
  method: string
  path: string
  status: number
  durationMs: number
  isStream: boolean
  channelId: number
  channelName: string
  tokenName: string
  model: string
  clientIp: string
  contentType: string
  requestBytes: number
  responseBytes: number
  errorMessage: string
}

export type TraceDetail = TraceSummary & {
  requestBody: string
  responseBody: string
}

export function tracesList(req: {
  page?: number
  pageSize?: number
  keyword?: string
  channelId?: number
  model?: string
  tokenName?: string
  onlyErrors?: boolean
  startUnix?: number
  endUnix?: number
}): Promise<{ items: TraceSummary[]; total: number; page: number; pageSize: number }> {
  return safe(
    async () =>
      (await anyCall(TracesAPIRaw.List as never, req)) as {
        items: TraceSummary[]
        total: number
        page: number
        pageSize: number
      },
    { items: [], total: 0, page: 1, pageSize: 50 }
  )
}

export function tracesGet(id: number): Promise<TraceDetail | null> {
  return safe(async () => (await TracesAPIRaw.Get(id)) as unknown as TraceDetail, null)
}

export function tracesPurgeOlderThan(olderThanUnix: number): Promise<number> {
  return strict(async () => (await TracesAPIRaw.PurgeOlderThan(olderThanUnix)) as number)
}

export function tracesCount(): Promise<number> {
  return safe(async () => (await TracesAPIRaw.Count()) as number, 0)
}

// ------------------------------ Settings ---------------------------------

export type PrismPaths = {
  dataDir: string
  logDir: string
  configFile: string
  os: string
  arch: string
}

export function settingsPaths(): Promise<PrismPaths | null> {
  return safe(async () => (await SettingsAPIRaw.GetPaths()) as unknown as PrismPaths, null)
}

export async function settingsOpenDataDir(): Promise<void> {
  await safe(async () => {
    await SettingsAPIRaw.OpenDataDir()
    return undefined
  }, undefined)
}

export function settingsExport(): Promise<unknown> {
  return strict(() => SettingsAPIRaw.Export())
}

export async function settingsExportToFile(path: string): Promise<void> {
  await strict(() => SettingsAPIRaw.ExportToFile(path))
}

export async function settingsImportFromFile(path: string, replace: boolean): Promise<void> {
  await strict(() => SettingsAPIRaw.ImportFromFile(path, replace))
}
