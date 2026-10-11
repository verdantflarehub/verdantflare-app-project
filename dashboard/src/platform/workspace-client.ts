import type { AppSDK, Context, Json, Unsubscribe } from './types.ts'

const id = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const handleID = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const hash = /^[0-9a-f]{64}$/
const identity = (c: Readonly<Context>) => JSON.stringify([c.userId, c.organizationId, c.stationId, c.appId, c.installationId, c.contextRevision])
const retained = new Map<string, string>()
const invalid = (): never => { throw new Error('INVALID_WORKSPACE_RESPONSE') }
const object = (v: unknown): Record<string, unknown> => v && typeof v === 'object' && !Array.isArray(v) ? v as Record<string, unknown> : invalid()
const array = (v: unknown): unknown[] => Array.isArray(v) && v.length <= 10000 ? v : invalid()
const text = (v: unknown): string => typeof v === 'string' ? v : invalid()
const uuid = (v: unknown): string => id.test(text(v)) ? v as string : invalid()
const relative = (v: unknown): string => {
  const p = text(v)
  if (!p || p.startsWith('/') || /[:\\\u0000]/.test(p) || p.split('/').some(s => !s || s === '.' || s === '..')) invalid()
  return p
}
export interface WorkspaceSnapshot {
  workspace_handle: string
  state: { project_id: string; base_revision_id: string }
  manifest: { name: string; files: { file_id: string; path: string; role: string }[] }
  files: { file_id: string; path: string; state: string }[]
}
export interface ConflictPreview {
  project_id: string; commit_id: string; request_sha256: string; base_revision_id: string; head_revision_id: string
  files: { index: number; pending: { file_id?: string; path: string; role: string; text?: string }; remote_changed: boolean; can_keep_pending: boolean }[]
}
export interface Resolution {
  commit_id: string; request_sha256: string; head_revision_id: string
  choices: { index: number; choice: 'head' | 'pending' | 'text'; text?: string }[]
}
type Simple = 'status' | 'previewConflict' | 'resume' | 'switchToHead' | 'close'
export type WorkspaceRequest =
  | { operation: 'openDirectory'; project_id: string }
  | { operation: Simple; workspace_handle: string }
  | { operation: 'fetch'; workspace_handle: string; file_id: string; max_bytes?: number }
  | { operation: 'saveTexts' | 'saveFiles'; workspace_handle: string; file_ids: string[] }
  | { operation: 'importFiles'; workspace_handle: string; role: string }
  | { operation: 'resolveConflict'; workspace_handle: string; resolution: Resolution }

export function snapshot(value: unknown, projectID: string): WorkspaceSnapshot {
  const v = object(value), state = object(v.state), manifest = object(v.manifest)
  if (!handleID.test(text(v.workspace_handle)) || state.schema_version !== 1 || state.kind !== 'local-workspace' || uuid(state.project_id) !== projectID || manifest.project_id !== projectID) invalid()
  const files = array(manifest.files).map(raw => { const f = object(raw); return { file_id: uuid(f.file_id), path: relative(f.path), role: text(f.role) } })
  const byID = new Map(files.map(f => [f.file_id, f.path]))
  if (byID.size !== files.length) invalid()
  const statuses = array(v.files).map(raw => {
    const f = object(raw), fileID = uuid(f.file_id), path = relative(f.path), state = text(f.state)
    if (byID.get(fileID) !== path || !['clean', 'not_materialized', 'missing', 'conflict', 'modified'].includes(state)) invalid()
    return { file_id: fileID, path, state }
  })
  if (statuses.length !== files.length || new Set(statuses.map(f => f.file_id)).size !== files.length) invalid()
  return { workspace_handle: v.workspace_handle as string, state: { project_id: projectID, base_revision_id: uuid(state.base_revision_id) }, manifest: { name: text(manifest.name), files }, files: statuses }
}
export function conflictPreview(value: unknown, projectID: string): ConflictPreview {
  const v = object(value)
  if (uuid(v.project_id) !== projectID || !hash.test(text(v.request_sha256))) invalid()
  const files = array(v.files).map(raw => {
    const f = object(raw), p = object(f.pending)
    if (!Number.isSafeInteger(f.index) || (f.index as number) < 0 || typeof f.remote_changed !== 'boolean' || typeof f.can_keep_pending !== 'boolean') invalid()
    return { index: f.index as number, pending: { ...(p.file_id === undefined ? {} : { file_id: uuid(p.file_id) }), path: relative(p.path), role: text(p.role), ...(p.text === undefined ? {} : { text: text(p.text) }) }, remote_changed: f.remote_changed as boolean, can_keep_pending: f.can_keep_pending as boolean }
  })
  if (new Set(files.map(f => f.index)).size !== files.length) invalid()
  return { project_id: projectID, commit_id: uuid(v.commit_id), request_sha256: v.request_sha256 as string, base_revision_id: uuid(v.base_revision_id), head_revision_id: uuid(v.head_revision_id), files }
}
function input(request: WorkspaceRequest): Json {
  const shapes: Record<string, string[]> = { openDirectory: ['project_id'], status: [], previewConflict: [], resume: [], switchToHead: [], close: [], fetch: ['file_id', 'max_bytes'], saveTexts: ['file_ids'], saveFiles: ['file_ids'], importFiles: ['role'], resolveConflict: ['resolution'] }
  if (!Object.hasOwn(shapes, request.operation)) invalid()
  const allowed = ['operation', ...(request.operation === 'openDirectory' ? [] : ['workspace_handle']), ...shapes[request.operation]]
  if (Object.keys(request).some(k => !allowed.includes(k))) invalid()
  if (request.operation === 'openDirectory') uuid(request.project_id)
  else if (!handleID.test(request.workspace_handle)) invalid()
  if (request.operation === 'fetch') { uuid(request.file_id); if (request.max_bytes !== undefined && (!Number.isSafeInteger(request.max_bytes) || request.max_bytes < 0 || request.max_bytes > 2 ** 40)) invalid() }
  if (request.operation === 'saveTexts' || request.operation === 'saveFiles') { if (!request.file_ids.length || request.file_ids.length > 10000 || new Set(request.file_ids).size !== request.file_ids.length) invalid(); request.file_ids.forEach(uuid) }
  if (request.operation === 'importFiles' && !/^[a-z][a-z0-9_-]{0,63}$/.test(request.role)) invalid()
  if (request.operation === 'resolveConflict') {
    const r = request.resolution
    if (Object.keys(r).some(k => !['commit_id', 'request_sha256', 'head_revision_id', 'choices'].includes(k)) || !hash.test(r.request_sha256)) invalid()
    uuid(r.commit_id); uuid(r.head_revision_id)
    const indices = new Set<number>()
    for (const choice of r.choices) {
      if (Object.keys(choice).some(k => !['index', 'choice', 'text'].includes(k)) || !Number.isSafeInteger(choice.index) || choice.index < 0 || indices.has(choice.index) || !['head', 'pending', 'text'].includes(choice.choice) || (choice.choice === 'text' ? typeof choice.text !== 'string' : choice.text !== undefined)) invalid()
      indices.add(choice.index)
    }
  }
  return JSON.parse(JSON.stringify(request)) as Json
}
export function workspaceErrorCode(error: unknown): string {
  const e = error && typeof error === 'object' ? error as { code?: unknown; message?: unknown } : {}
  return typeof e.code === 'string' ? e.code : typeof e.message === 'string' ? e.message : ''
}
export function workspaceError(error: unknown): string {
  const messages: Record<string, string> = {
    DESKTOP_REQUIRED: '本机工作副本需要从 Studio 桌面应用打开。', CAPABILITY_UNAVAILABLE: '当前宿主未提供本机工作副本能力，请使用新版 Studio 桌面应用。',
    CAPABILITY_DENIED: '本机目录授权已失效，请重新选择目录。本机内容和待提交记录仍保留。', FORBIDDEN: '本机目录授权已失效，请重新选择目录。本机内容和待提交记录仍保留。', PERMISSION_DENIED: '没有此项目的操作权限，或本机授权已失效。请检查权限后重新选择目录。',
    PENDING_COMMIT: '存在未确认的提交，请使用“恢复原提交”，不要重复保存。', REVISION_CONFLICT: '远端项目已更新。请预览冲突并逐项选择处理方式。',
    LOCAL_FILE_CONFLICT: '本机文件存在冲突，未覆盖原文件。请检查文件状态。', WORKSPACE_BUSY: '该目录正由其他工作副本使用，请先关闭原工作副本。',
    WORKSPACE_CANCELLED: '操作已取消。若保存结果未确认，请恢复原提交。', WORKSPACE_CONTENT_MISMATCH: '本机恢复记录与内容不一致，已停止操作并保留原文件。',
    INVALID_WORKSPACE: '该目录不是此项目的有效工作副本，请重新选择。', INVALID_WORKSPACE_RESPONSE: '工作副本响应不完整，已保留原文件。',
  }
  return messages[workspaceErrorCode(error)] || '操作未完成，原文件和待提交记录仍保留。保存结果未确认时，请恢复原提交。'
}
export function createWorkspaceClient(sdk: AppSDK) {
  const scope = identity(sdk.context.get())
  if (sdk.context.get().appId !== 'project') throw new Error('CAPABILITY_DENIED')
  let disposed = false, hidden = false
  const pending = new Map<AbortController, boolean>(), subscriptions: Unsubscribe[] = []
  const cancelled = () => new DOMException('当前页面或授权已失效。', 'AbortError')
  function dispose() { if (disposed) return; disposed = true; pending.forEach((_read, c) => c.abort()); pending.clear(); subscriptions.splice(0).forEach(stop => stop()) }
  subscriptions.push(sdk.context.subscribe(c => { if (identity(c) !== scope) { for (const key of retained.keys()) if (key.startsWith(scope + ':')) retained.delete(key); dispose() } }))
  subscriptions.push(sdk.lifecycle.subscribe(event => { if (event === 'dispose') dispose(); else { hidden = event === 'hidden'; if (hidden) pending.forEach((read, c) => { if (read) c.abort() }) } }))
  async function call(request: WorkspaceRequest): Promise<Record<string, Json>> {
    if (disposed || hidden || identity(sdk.context.get()) !== scope) throw cancelled()
    const body = input(request), controller = new AbortController()
    pending.set(controller, request.operation === 'status' || request.operation === 'previewConflict')
    try {
      const response = await sdk.capabilities.call('workspace.local', body, { signal: controller.signal })
      if (disposed || controller.signal.aborted || identity(sdk.context.get()) !== scope) throw cancelled()
      return object(response) as Record<string, Json>
    } finally { pending.delete(controller) }
  }
  return { call, dispose, retained: (project: string) => retained.get(scope + ':' + project), remember: (project: string, handle?: string) => { const key = scope + ':' + project; if (handle) { if (!handleID.test(handle)) invalid(); retained.set(key, handle) } else retained.delete(key) } }
}
