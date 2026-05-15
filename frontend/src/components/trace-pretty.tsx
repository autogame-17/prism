// trace-pretty.tsx — 把 prism_traces 抓到的请求/响应原始 JSON 渲染成
// "人类视角"的对话视图：按 role 分块，content 数组按 type 拆分（text /
// image / tool_use / tool_result），SSE chunks 合并成一个 assistant turn。
//
// 设计目标：
// - 兼容 OpenAI Chat Completions、Anthropic Messages、以及两者混合
//   （Cursor 的 cursor-opus-4-7 等模型使用 Anthropic content array 风格）。
// - 解析失败永不抛错：任何无法识别的字段降级为 raw JSON 块继续展示。
// - 纯渲染组件，不依赖 React Query 或 stores，方便复用。
import { useMemo } from 'react'
import { useI18n } from '@/lib/i18n'

// ChatMessage 是给渲染层的统一中间态。原始数据可能是 OpenAI / Anthropic /
// 部分字段缺失，这里都规整成一致形态。
export type ChatPart =
  | { kind: 'text'; text: string }
  | { kind: 'image'; alt?: string; url?: string }
  | { kind: 'tool_use'; name: string; id?: string; input: unknown }
  | { kind: 'tool_result'; toolUseId?: string; content: string; isError?: boolean }
  | { kind: 'unknown'; raw: unknown }

export type ChatMessage = {
  role: 'system' | 'user' | 'assistant' | 'tool' | 'developer' | string
  name?: string
  parts: ChatPart[]
  // OpenAI 工具调用：assistant 发起的 function call。
  toolCalls?: Array<{ id?: string; name: string; arguments: string }>
  // tool 角色消息回填 tool_call_id。
  toolCallId?: string
  // 解析失败时的兜底原文。
  rawFallback?: string
}

export type ParsedRequest = {
  system?: string
  messages: ChatMessage[]
  tools?: Array<{ name: string; description?: string; schema?: unknown }>
  // 用户原文里出现但我们没归类的额外字段（如 temperature / max_tokens），
  // 折叠展示在底部。
  extras?: Record<string, unknown>
}

export type ParsedResponse = {
  message: ChatMessage
  finishReason?: string
  // 解析时收到了 SSE chunks 数 / 是否 [DONE] 干净结束。便于和后端的诊断列对齐。
  sseChunkCount?: number
  sseTerminated?: boolean
}

// ---- request parsing -------------------------------------------------

export function parseRequestBody(raw: string): ParsedRequest | null {
  if (!raw) return null
  let body: unknown
  try {
    body = JSON.parse(raw)
  } catch {
    return null
  }
  if (!isObject(body)) return null

  const messages: ChatMessage[] = []
  const rawMessages = (body as Record<string, unknown>).messages
  if (Array.isArray(rawMessages)) {
    for (const m of rawMessages) {
      const msg = parseMessage(m)
      if (msg) messages.push(msg)
    }
  }

  // Anthropic 顶层 system 字段（与 OpenAI 用 messages[0].role='system' 不同）。
  let systemText: string | undefined
  const sys = (body as Record<string, unknown>).system
  if (typeof sys === 'string') systemText = sys
  else if (Array.isArray(sys)) systemText = collectText(sys)

  const tools = parseTools((body as Record<string, unknown>).tools)

  // 收集额外字段：除我们已消费的 keys 之外的非空原始值。
  const consumed = new Set(['messages', 'system', 'tools', 'model', 'stream'])
  const extras: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(body as Record<string, unknown>)) {
    if (consumed.has(k)) continue
    if (v === null || v === undefined || v === '') continue
    extras[k] = v
  }

  return {
    system: systemText,
    messages,
    tools,
    extras: Object.keys(extras).length ? extras : undefined,
  }
}

function parseMessage(m: unknown): ChatMessage | null {
  if (!isObject(m)) return null
  const role = String((m as Record<string, unknown>).role ?? 'unknown')
  const name = (m as Record<string, unknown>).name
  const out: ChatMessage = {
    role,
    name: typeof name === 'string' ? name : undefined,
    parts: [],
  }

  const content = (m as Record<string, unknown>).content
  if (typeof content === 'string') {
    if (content) out.parts.push({ kind: 'text', text: content })
  } else if (Array.isArray(content)) {
    for (const p of content) {
      const part = parseContentPart(p)
      if (part) out.parts.push(part)
    }
  } else if (content !== null && content !== undefined) {
    out.parts.push({ kind: 'unknown', raw: content })
  }

  // OpenAI 风格的 tool_calls 在 assistant 消息上。
  const tc = (m as Record<string, unknown>).tool_calls
  if (Array.isArray(tc)) {
    out.toolCalls = []
    for (const c of tc) {
      if (!isObject(c)) continue
      const fn = (c as Record<string, unknown>).function
      if (!isObject(fn)) continue
      out.toolCalls.push({
        id: typeof (c as Record<string, unknown>).id === 'string' ? ((c as Record<string, unknown>).id as string) : undefined,
        name: String((fn as Record<string, unknown>).name ?? ''),
        arguments:
          typeof (fn as Record<string, unknown>).arguments === 'string'
            ? ((fn as Record<string, unknown>).arguments as string)
            : JSON.stringify((fn as Record<string, unknown>).arguments ?? {}),
      })
    }
  }

  const tcId = (m as Record<string, unknown>).tool_call_id
  if (typeof tcId === 'string') out.toolCallId = tcId

  return out
}

function parseContentPart(p: unknown): ChatPart | null {
  if (typeof p === 'string') {
    return p ? { kind: 'text', text: p } : null
  }
  if (!isObject(p)) return null
  const type = (p as Record<string, unknown>).type
  if (type === 'text' || type === 'input_text' || type === 'output_text') {
    const text = (p as Record<string, unknown>).text
    return typeof text === 'string' ? { kind: 'text', text } : null
  }
  if (type === 'image' || type === 'image_url' || type === 'input_image') {
    // OpenAI: image_url:{url}; Anthropic: source:{data, media_type}
    const url = (p as Record<string, unknown>).image_url
    if (isObject(url) && typeof (url as Record<string, unknown>).url === 'string') {
      return { kind: 'image', url: (url as Record<string, unknown>).url as string }
    }
    return { kind: 'image', alt: 'image' }
  }
  if (type === 'tool_use') {
    return {
      kind: 'tool_use',
      name: String((p as Record<string, unknown>).name ?? ''),
      id: typeof (p as Record<string, unknown>).id === 'string' ? ((p as Record<string, unknown>).id as string) : undefined,
      input: (p as Record<string, unknown>).input,
    }
  }
  if (type === 'tool_result') {
    const c = (p as Record<string, unknown>).content
    let text = ''
    if (typeof c === 'string') text = c
    else if (Array.isArray(c)) text = collectText(c)
    else if (c !== undefined) text = JSON.stringify(c, null, 2)
    return {
      kind: 'tool_result',
      toolUseId:
        typeof (p as Record<string, unknown>).tool_use_id === 'string'
          ? ((p as Record<string, unknown>).tool_use_id as string)
          : undefined,
      content: text,
      isError: Boolean((p as Record<string, unknown>).is_error),
    }
  }
  return { kind: 'unknown', raw: p }
}

function parseTools(t: unknown): ParsedRequest['tools'] {
  if (!Array.isArray(t)) return undefined
  const out: NonNullable<ParsedRequest['tools']> = []
  for (const tool of t) {
    if (!isObject(tool)) continue
    // OpenAI: {type:'function', function:{name, description, parameters}}
    const fn = (tool as Record<string, unknown>).function
    if (isObject(fn)) {
      out.push({
        name: String((fn as Record<string, unknown>).name ?? ''),
        description:
          typeof (fn as Record<string, unknown>).description === 'string'
            ? ((fn as Record<string, unknown>).description as string)
            : undefined,
        schema: (fn as Record<string, unknown>).parameters,
      })
      continue
    }
    // Anthropic: {name, description, input_schema}
    if (typeof (tool as Record<string, unknown>).name === 'string') {
      out.push({
        name: (tool as Record<string, unknown>).name as string,
        description:
          typeof (tool as Record<string, unknown>).description === 'string'
            ? ((tool as Record<string, unknown>).description as string)
            : undefined,
        schema: (tool as Record<string, unknown>).input_schema,
      })
    }
  }
  return out.length ? out : undefined
}

function collectText(arr: unknown[]): string {
  const parts: string[] = []
  for (const p of arr) {
    if (typeof p === 'string') parts.push(p)
    else if (isObject(p) && typeof (p as Record<string, unknown>).text === 'string') {
      parts.push((p as Record<string, unknown>).text as string)
    }
  }
  return parts.join('\n')
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

// ---- response parsing ------------------------------------------------

export function parseResponseBody(raw: string): ParsedResponse | null {
  if (!raw) return null
  const trimmed = raw.trimStart()
  if (trimmed.startsWith('data:')) return parseSSE(raw)
  if (trimmed.startsWith('{')) {
    try {
      const obj = JSON.parse(raw)
      return parseCompleteResponse(obj)
    } catch {
      return null
    }
  }
  return null
}

function parseCompleteResponse(obj: unknown): ParsedResponse | null {
  if (!isObject(obj)) return null
  // OpenAI: {choices:[{message:{role,content,tool_calls}, finish_reason}]}
  const choices = (obj as Record<string, unknown>).choices
  if (Array.isArray(choices) && choices.length > 0 && isObject(choices[0])) {
    const ch = choices[0] as Record<string, unknown>
    const msg = parseMessage(ch.message)
    if (msg) {
      return {
        message: msg,
        finishReason: typeof ch.finish_reason === 'string' ? ch.finish_reason : undefined,
      }
    }
  }
  // Anthropic: {role, content:[{type:'text',text}|{type:'tool_use'}], stop_reason}
  const role = (obj as Record<string, unknown>).role
  const content = (obj as Record<string, unknown>).content
  if (typeof role === 'string' && (Array.isArray(content) || typeof content === 'string')) {
    const msg = parseMessage(obj)
    if (msg) {
      return {
        message: msg,
        finishReason:
          typeof (obj as Record<string, unknown>).stop_reason === 'string'
            ? ((obj as Record<string, unknown>).stop_reason as string)
            : undefined,
      }
    }
  }
  return null
}

// parseSSE 把按 `data: <json>\n\n` 切分的流式事件合并成一个 assistant turn。
// 同时兼容 OpenAI delta 形态和 Anthropic event-stream 形态：
//   - OpenAI:    data: {"choices":[{"delta":{"content":"...","tool_calls":[...]}}]}
//   - Anthropic: data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"..."}}
//
// 任何无法识别的 chunk 直接忽略。
function parseSSE(raw: string): ParsedResponse {
  const textParts: string[] = []
  const toolCallsByIdx: Map<number, { id?: string; name: string; arguments: string }> = new Map()
  // Anthropic content blocks indexed by `index` field
  const anthropicBlocks: Map<number, { type?: string; text: string; toolName?: string; toolId?: string; input: string }> = new Map()
  let role = 'assistant'
  let finishReason: string | undefined
  let chunkCount = 0
  let terminated = false

  for (const line of raw.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (!trimmed.startsWith('data:')) continue
    const payload = trimmed.slice(5).trim()
    if (!payload) continue
    if (payload === '[DONE]') {
      terminated = true
      break
    }
    let chunk: unknown
    try {
      chunk = JSON.parse(payload)
    } catch {
      continue
    }
    chunkCount++

    if (!isObject(chunk)) continue

    // OpenAI delta path
    const choices = (chunk as Record<string, unknown>).choices
    if (Array.isArray(choices)) {
      for (const ch of choices) {
        if (!isObject(ch)) continue
        const delta = (ch as Record<string, unknown>).delta
        if (isObject(delta)) {
          if (typeof (delta as Record<string, unknown>).role === 'string') {
            role = (delta as Record<string, unknown>).role as string
          }
          const c = (delta as Record<string, unknown>).content
          if (typeof c === 'string') textParts.push(c)
          const tc = (delta as Record<string, unknown>).tool_calls
          if (Array.isArray(tc)) {
            for (const t of tc) {
              if (!isObject(t)) continue
              const idx = typeof (t as Record<string, unknown>).index === 'number' ? ((t as Record<string, unknown>).index as number) : 0
              const bucket =
                toolCallsByIdx.get(idx) ?? { name: '', arguments: '' }
              if (typeof (t as Record<string, unknown>).id === 'string') bucket.id = (t as Record<string, unknown>).id as string
              const fn = (t as Record<string, unknown>).function
              if (isObject(fn)) {
                if (typeof (fn as Record<string, unknown>).name === 'string') bucket.name = (fn as Record<string, unknown>).name as string
                if (typeof (fn as Record<string, unknown>).arguments === 'string') bucket.arguments += (fn as Record<string, unknown>).arguments as string
              }
              toolCallsByIdx.set(idx, bucket)
            }
          }
        }
        const fr = (ch as Record<string, unknown>).finish_reason
        if (typeof fr === 'string') finishReason = fr
      }
      continue
    }

    // Anthropic event-stream path
    const type = (chunk as Record<string, unknown>).type
    if (type === 'message_start') {
      const m = (chunk as Record<string, unknown>).message
      if (isObject(m) && typeof (m as Record<string, unknown>).role === 'string') {
        role = (m as Record<string, unknown>).role as string
      }
      continue
    }
    if (type === 'content_block_start') {
      const idx = numericField(chunk, 'index')
      const block = (chunk as Record<string, unknown>).content_block
      if (idx !== undefined && isObject(block)) {
        const b = anthropicBlocks.get(idx) ?? { text: '', input: '' }
        b.type = typeof (block as Record<string, unknown>).type === 'string' ? ((block as Record<string, unknown>).type as string) : b.type
        if (b.type === 'tool_use') {
          if (typeof (block as Record<string, unknown>).name === 'string') b.toolName = (block as Record<string, unknown>).name as string
          if (typeof (block as Record<string, unknown>).id === 'string') b.toolId = (block as Record<string, unknown>).id as string
        }
        anthropicBlocks.set(idx, b)
      }
      continue
    }
    if (type === 'content_block_delta') {
      const idx = numericField(chunk, 'index')
      const delta = (chunk as Record<string, unknown>).delta
      if (idx !== undefined && isObject(delta)) {
        const b = anthropicBlocks.get(idx) ?? { text: '', input: '' }
        const dt = (delta as Record<string, unknown>).type
        if (dt === 'text_delta' && typeof (delta as Record<string, unknown>).text === 'string') {
          b.text += (delta as Record<string, unknown>).text as string
        } else if (dt === 'input_json_delta' && typeof (delta as Record<string, unknown>).partial_json === 'string') {
          b.input += (delta as Record<string, unknown>).partial_json as string
        }
        anthropicBlocks.set(idx, b)
      }
      continue
    }
    if (type === 'message_delta') {
      const delta = (chunk as Record<string, unknown>).delta
      if (isObject(delta) && typeof (delta as Record<string, unknown>).stop_reason === 'string') {
        finishReason = (delta as Record<string, unknown>).stop_reason as string
      }
      continue
    }
    if (type === 'message_stop') {
      terminated = true
      continue
    }
  }

  // Build the final ChatMessage from whichever pipeline contributed.
  const parts: ChatPart[] = []
  // OpenAI text path
  const openAIText = textParts.join('')
  if (openAIText) parts.push({ kind: 'text', text: openAIText })
  // Anthropic blocks (preserve original index order)
  const indexes = [...anthropicBlocks.keys()].sort((a, b) => a - b)
  for (const i of indexes) {
    const b = anthropicBlocks.get(i)!
    if (b.type === 'tool_use') {
      let parsedInput: unknown = b.input
      if (b.input) {
        try {
          parsedInput = JSON.parse(b.input)
        } catch {
          /* leave as raw partial JSON */
        }
      }
      parts.push({ kind: 'tool_use', name: b.toolName ?? '', id: b.toolId, input: parsedInput })
    } else if (b.text) {
      parts.push({ kind: 'text', text: b.text })
    }
  }

  const toolCalls: ChatMessage['toolCalls'] = []
  for (const idx of [...toolCallsByIdx.keys()].sort((a, b) => a - b)) {
    toolCalls.push(toolCallsByIdx.get(idx)!)
  }

  const message: ChatMessage = {
    role,
    parts,
    toolCalls: toolCalls.length ? toolCalls : undefined,
  }
  return {
    message,
    finishReason,
    sseChunkCount: chunkCount,
    sseTerminated: terminated,
  }
}

function numericField(o: unknown, key: string): number | undefined {
  if (!isObject(o)) return undefined
  const v = (o as Record<string, unknown>)[key]
  return typeof v === 'number' ? v : undefined
}

// ---- React render ---------------------------------------------------

const ROLE_STYLES: Record<string, { label: string; cls: string }> = {
  system:    { label: 'system',    cls: 'border-amber-500/40 bg-amber-500/5 text-amber-100' },
  user:      { label: 'user',      cls: 'border-cyan-500/40 bg-cyan-500/5 text-cyan-100' },
  assistant: { label: 'assistant', cls: 'border-emerald-500/40 bg-emerald-500/5 text-emerald-100' },
  tool:      { label: 'tool',      cls: 'border-violet-500/40 bg-violet-500/5 text-violet-100' },
  developer: { label: 'developer', cls: 'border-slate-500/40 bg-slate-500/5 text-slate-100' },
}

function roleStyle(role: string): { label: string; cls: string } {
  return (
    ROLE_STYLES[role] ?? {
      label: role,
      cls: 'border-slate-500/40 bg-slate-500/5 text-slate-200',
    }
  )
}

export function PrettyRequest({ raw }: { raw: string }) {
  const t = useI18n((s) => s.t)
  const parsed = useMemo(() => parseRequestBody(raw), [raw])

  if (!parsed) {
    return (
      <div className="rounded-md border border-amber-500/40 bg-amber-500/5 p-2 text-[11px] text-amber-200">
        {t('logs.traces.pretty.parseFail')}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-2">
      {parsed.system && (
        <MessageBlock
          role="system"
          parts={[{ kind: 'text', text: parsed.system }]}
        />
      )}
      {parsed.messages.map((m, i) => (
        <MessageBlock
          key={i}
          role={m.role}
          name={m.name}
          parts={m.parts}
          toolCalls={m.toolCalls}
          toolCallId={m.toolCallId}
        />
      ))}
      {parsed.tools && parsed.tools.length > 0 && (
        <details className="rounded-md border border-border/60 bg-muted/30 p-2 text-[11px]">
          <summary className="cursor-pointer text-xs font-medium text-muted-foreground">
            {t('logs.traces.pretty.tools')} ({parsed.tools.length})
          </summary>
          <ul className="mt-2 flex flex-col gap-1.5">
            {parsed.tools.map((tool, i) => (
              <li key={i} className="border-l-2 border-violet-500/40 pl-2">
                <div className="font-mono text-violet-200">{tool.name}</div>
                {tool.description && (
                  <div className="text-muted-foreground">{tool.description}</div>
                )}
              </li>
            ))}
          </ul>
        </details>
      )}
      {parsed.extras && Object.keys(parsed.extras).length > 0 && (
        <details className="rounded-md border border-border/60 bg-muted/30 p-2 text-[11px]">
          <summary className="cursor-pointer text-xs font-medium text-muted-foreground">
            {t('logs.traces.pretty.params')}
          </summary>
          <pre className="prism-selectable mt-2 whitespace-pre-wrap break-all font-mono text-muted-foreground">
            {JSON.stringify(parsed.extras, null, 2)}
          </pre>
        </details>
      )}
    </div>
  )
}

export function PrettyResponse({ raw }: { raw: string }) {
  const t = useI18n((s) => s.t)
  const parsed = useMemo(() => parseResponseBody(raw), [raw])

  if (!parsed) {
    return (
      <div className="rounded-md border border-amber-500/40 bg-amber-500/5 p-2 text-[11px] text-amber-200">
        {t('logs.traces.pretty.parseFail')}
      </div>
    )
  }

  const { message, finishReason, sseChunkCount, sseTerminated } = parsed
  const truncated = sseChunkCount !== undefined && !sseTerminated && sseChunkCount > 0

  return (
    <div className="flex flex-col gap-2">
      <MessageBlock
        role={message.role || 'assistant'}
        parts={message.parts}
        toolCalls={message.toolCalls}
      />
      <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-muted-foreground">
        {finishReason && (
          <span className="rounded border border-border/60 px-1.5 py-0.5 font-mono">
            finish_reason: {finishReason}
          </span>
        )}
        {sseChunkCount !== undefined && (
          <span className="rounded border border-border/60 px-1.5 py-0.5 font-mono">
            chunks: {sseChunkCount}
          </span>
        )}
        {truncated && (
          <span className="rounded border border-amber-500/60 bg-amber-500/10 px-1.5 py-0.5 font-mono text-amber-200">
            {t('logs.traces.pretty.truncated')}
          </span>
        )}
      </div>
    </div>
  )
}

function MessageBlock({
  role,
  name,
  parts,
  toolCalls,
  toolCallId,
}: {
  role: string
  name?: string
  parts: ChatPart[]
  toolCalls?: ChatMessage['toolCalls']
  toolCallId?: string
}) {
  const style = roleStyle(role)
  return (
    <div className={`rounded-md border ${style.cls} p-2`}>
      <div className="mb-1.5 flex items-center gap-2 text-[10px] uppercase tracking-wide">
        <span className="font-semibold">{style.label}</span>
        {name && <span className="font-mono text-muted-foreground">{name}</span>}
        {toolCallId && (
          <span className="font-mono text-muted-foreground">tool_call_id={toolCallId}</span>
        )}
      </div>
      <div className="flex flex-col gap-1.5">
        {parts.map((p, i) => (
          <PartView key={i} part={p} />
        ))}
        {toolCalls?.map((tc, i) => (
          <div
            key={`tc-${i}`}
            className="rounded border border-violet-500/40 bg-violet-500/5 p-1.5 font-mono text-[11px]"
          >
            <div className="text-violet-200">
              {tc.name}({tc.id ? <span className="text-muted-foreground">{tc.id}</span> : null})
            </div>
            <pre className="prism-selectable mt-1 whitespace-pre-wrap break-all text-violet-100/80">
              {prettyJSONOrRaw(tc.arguments)}
            </pre>
          </div>
        ))}
      </div>
    </div>
  )
}

function PartView({ part }: { part: ChatPart }) {
  if (part.kind === 'text') {
    return (
      <div className="prism-selectable whitespace-pre-wrap break-words text-[12.5px] leading-relaxed">
        {part.text}
      </div>
    )
  }
  if (part.kind === 'image') {
    return (
      <div className="rounded border border-dashed border-border/60 bg-muted/30 p-1.5 text-[11px] text-muted-foreground">
        [image{part.url ? `: ${truncateMiddle(part.url, 60)}` : ''}]
      </div>
    )
  }
  if (part.kind === 'tool_use') {
    return (
      <div className="rounded border border-violet-500/40 bg-violet-500/5 p-1.5 font-mono text-[11px]">
        <div className="text-violet-200">
          tool_use: {part.name}
          {part.id && <span className="ml-2 text-muted-foreground">{part.id}</span>}
        </div>
        <pre className="prism-selectable mt-1 whitespace-pre-wrap break-all text-violet-100/80">
          {typeof part.input === 'string' ? part.input : JSON.stringify(part.input, null, 2)}
        </pre>
      </div>
    )
  }
  if (part.kind === 'tool_result') {
    return (
      <div
        className={`rounded border p-1.5 font-mono text-[11px] ${
          part.isError
            ? 'border-red-500/40 bg-red-500/5 text-red-200'
            : 'border-violet-500/40 bg-violet-500/5 text-violet-100/90'
        }`}
      >
        <div className="text-[10px] uppercase tracking-wide text-muted-foreground">
          tool_result{part.toolUseId ? ` (${part.toolUseId})` : ''}
        </div>
        <pre className="prism-selectable mt-1 whitespace-pre-wrap break-all">{part.content}</pre>
      </div>
    )
  }
  return (
    <pre className="prism-selectable whitespace-pre-wrap break-all rounded border border-dashed border-border/60 bg-muted/30 p-1.5 font-mono text-[11px] text-muted-foreground">
      {JSON.stringify(part.raw, null, 2)}
    </pre>
  )
}

function truncateMiddle(s: string, max: number): string {
  if (s.length <= max) return s
  const head = Math.floor((max - 1) / 2)
  const tail = max - 1 - head
  return `${s.slice(0, head)}…${s.slice(-tail)}`
}

function prettyJSONOrRaw(s: string): string {
  if (!s) return ''
  try {
    return JSON.stringify(JSON.parse(s), null, 2)
  } catch {
    return s
  }
}
