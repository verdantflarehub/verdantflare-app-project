import type { AppSDK, Context, Json, Unsubscribe } from './types.ts'
import { commitID, type RecoveryStorage } from './asset-import.ts'

export const createCategories = [
  { key: 'character', label: '角色设计' }, { key: 'music', label: '音乐制作' }, { key: 'video', label: '创意视频' },
  { key: 'ai_short_drama', label: '短剧制作' }, { key: 'creative_video', label: '创意视频制作' },
  { key: 'image', label: 'image（历史）' }, { key: 'music-mv', label: 'music-mv（历史）' },
] as const
export const createBodyLimit = 1024 * 1024
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const hashPattern = /^[0-9a-f]{64}$/
const principal = (c: Readonly<Context>) => [c.userId, c.organizationId, c.stationId, c.appId, c.installationId]
const identity = (c: Readonly<Context>) => JSON.stringify([...principal(c), c.contextRevision])
const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T
const invalid = (): never => { throw new CreateError('INVALID_RESPONSE') }
const object = (v: unknown): Record<string, unknown> => v && typeof v === 'object' && !Array.isArray(v) ? v as Record<string, unknown> : invalid()
const id = (v: unknown): string => typeof v === 'string' && uuid.test(v) ? v : invalid()
export interface CreateInput { name: string; category: string; entry_path: string; entry_text: string }
export interface CreateRequest extends CreateInput { commit_id: string }
export interface CreateResult { project_id: string; revision_id: string }
export interface CreateRecord { schemaVersion: 1; request: CreateRequest; requestHash: string; result?: CreateResult }
export type CreateState = 'unknown' | 'preparing' | 'committed' | 'conflict' | 'failed'
export class CreateError extends Error { constructor(readonly code: string) { super(code) } }
export function createError(error: unknown): string {
  const code = error instanceof CreateError ? error.code : ''
  return ({
    INVALID_NAME: '请填写 1 至 256 个字符的项目名称。', INVALID_CATEGORY: '请选择支持的项目分类。',
    INVALID_PATH: '入口文档请使用项目内的相对路径，并以 .md 或 .txt 结尾。', INVALID_TEXT: '入口内容包含无效文字，请检查后再保存。',
    BODY_TOO_LARGE: '创建内容超过单次保存大小，请缩短入口文档后重试。',
    STORAGE_UNAVAILABLE: '无法读写创建恢复记录，请检查本机存储并保留原操作。',
    PENDING_CREATE: '还有一项创建结果待确认，请先恢复原操作。',
    PERMISSION_DENIED: '当前身份没有创建或读取此项目的权限，原创建记录仍保留。',
    SESSION_REVOKED: '会话已变化，请重新打开应用。原创建恢复记录仍保留。',
    INVALID_RESPONSE: '返回内容或恢复记录不完整，原创建记录仍保留。',
    REQUEST_MISMATCH: '创建记录与原请求不一致，已停止操作并保留记录。',
    IDEMPOTENCY_CONFLICT: '原创建编号对应不同内容，已停止操作并保留记录。',
    NOT_FOUND: '尚未查询到原创建结果，请保留记录并恢复原操作。',
    DASHBOARD_HIDDEN: '页面已隐藏，请返回后恢复原操作。',
  } as Record<string, string>)[code] || '暂未确认创建结果，请保留记录并恢复原操作。'
}
export function createRequestBytes(request: CreateRequest): number { return new TextEncoder().encode(JSON.stringify(request)).byteLength }
function validPath(path: string): boolean {
  if (!path || [...path].length > 1024 || path !== path.normalize('NFC') || /[\p{Cc}\\<>:"|?*\ud800-\udfff]/u.test(path) || !/\.(md|txt)$/i.test(path)) return false
  return path.split('/').every((segment, i) => !!segment && segment !== '.' && segment !== '..' && !/[. ]$/.test(segment) && !/^(con|prn|aux|nul|com[1-9¹²³]|lpt[1-9¹²³])(?:\.|$)/i.test(segment) && !(i === 0 && segment.toLowerCase() === '.vf'))
}
function parseRequest(value: unknown): CreateRequest {
  const r = object(value)
  if (Object.keys(r).sort().join(',') !== 'category,commit_id,entry_path,entry_text,name') invalid()
  id(r.commit_id)
  if (typeof r.name !== 'string' || !r.name.trim() || !/[^\p{White_Space}]/u.test(r.name) || [...r.name].length > 256 || /[\ud800-\udfff]/u.test(r.name)) throw new CreateError('INVALID_NAME')
  if (typeof r.category !== 'string' || !createCategories.some(c => c.key === r.category)) throw new CreateError('INVALID_CATEGORY')
  if (typeof r.entry_path !== 'string' || !validPath(r.entry_path)) throw new CreateError('INVALID_PATH')
  if (typeof r.entry_text !== 'string' || /[\ud800-\udfff]/u.test(r.entry_text)) throw new CreateError('INVALID_TEXT')
  const request = { commit_id: r.commit_id as string, name: r.name, category: r.category, entry_path: r.entry_path, entry_text: r.entry_text }
  if (createRequestBytes(request) > createBodyLimit) throw new CreateError('BODY_TOO_LARGE')
  return request
}
/** Go CreateRequest field order and JSON escaping; compare the original status hash. */
export async function createRequestHash(request: CreateRequest): Promise<string> {
  const ordered = parseRequest(request)
  const json = JSON.stringify(ordered).replace(/[<>&\u2028\u2029]/g, c => '\\u' + c.charCodeAt(0).toString(16).padStart(4, '0'))
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(json))
  return [...new Uint8Array(digest)].map(b => b.toString(16).padStart(2, '0')).join('')
}
function parseRecord(value: unknown): CreateRecord {
  const r = object(value)
  if (r.schemaVersion !== 1 || typeof r.requestHash !== 'string' || !hashPattern.test(r.requestHash) || Object.keys(r).some(k => !['schemaVersion', 'request', 'requestHash', 'result'].includes(k))) invalid()
  const request = parseRequest(r.request)
  const result = r.result === undefined ? undefined : { project_id: id(object(r.result).project_id), revision_id: id(object(r.result).revision_id) }
  return { schemaVersion: 1, request, requestHash: r.requestHash as string, ...(result ? { result } : {}) }
}
function parseResult(value: unknown, request: CreateRequest): CreateResult {
  const result = object(value), manifest = object(result.manifest), ref = object(result.manifest_ref)
  const project = id(result.project_id), revision = id(result.revision_id)
  id(ref.store_id); id(ref.artifact_id); id(ref.version_id)
  if (manifest.schema_version !== 1 || manifest.kind !== 'project' || manifest.project_id !== project || manifest.name !== request.name || manifest.category !== request.category || manifest.status !== 'draft' || !Array.isArray(manifest.files) || manifest.files.length !== 1) invalid()
  const file = object((manifest.files as unknown[])[0]), content = object(file.content_ref)
  if (file.file_id !== manifest.entry_document_id || file.path !== request.entry_path || file.role !== 'review') invalid()
  id(file.file_id); id(content.store_id); id(content.artifact_id); id(content.version_id)
  return { project_id: project, revision_id: revision }
}
export function createProjectCreator(sdk: AppSDK, storage: RecoveryStorage) {
  const context = sdk.context.get(), initial = identity(context)
  if (context.appId !== 'project') throw new CreateError('SESSION_REVOKED')
  const key = 'project.create.v1:' + JSON.stringify(principal(context))
  let record: CreateRecord | undefined, state: CreateState = 'unknown', disposed = false, hidden = false, working = false
  const requests = new Map<AbortController, boolean>(), stops: Unsubscribe[] = []
  function live() { if (disposed || identity(sdk.context.get()) !== initial) throw new CreateError('SESSION_REVOKED') }
  function readRecord() {
    let raw: string | null
    try { raw = storage.getItem(key) } catch { throw new CreateError('STORAGE_UNAVAILABLE') }
    if (!raw) return undefined
    if (raw.length > createBodyLimit + 4096) return invalid()
    try { return parseRecord(JSON.parse(raw)) } catch { return invalid() }
  }
  function save(next: CreateRecord) {
    live()
    try { const raw = JSON.stringify(next); storage.setItem(key, raw); if (storage.getItem(key) !== raw) throw Error() } catch { throw new CreateError('STORAGE_UNAVAILABLE') }
    record = clone(next)
  }
  record = readRecord()
  function dispose() { if (disposed) return; disposed = true; requests.forEach((_read, c) => c.abort()); requests.clear(); stops.splice(0).forEach(stop => stop()) }
  stops.push(sdk.context.subscribe(c => { if (identity(c) !== initial) dispose() }))
  stops.push(sdk.lifecycle.subscribe(event => { if (event === 'dispose') dispose(); else { hidden = event === 'hidden'; if (hidden) requests.forEach((read, c) => { if (read) c.abort() }) } }))
  async function exact(): Promise<CreateRecord> {
    live(); const saved = readRecord()
    if (!record || !saved || JSON.stringify(record) !== JSON.stringify(saved) || await createRequestHash(record.request) !== record.requestHash) throw new CreateError('REQUEST_MISMATCH')
    live(); if (JSON.stringify(record) !== JSON.stringify(readRecord())) throw new CreateError('REQUEST_MISMATCH')
    return record
  }
  async function request(operation: 'create' | 'commit_status', body: Json) {
    live(); if (hidden) throw new CreateError('DASHBOARD_HIDDEN')
    const controller = new AbortController(); requests.set(controller, operation !== 'create')
    try {
      const response = await sdk.app.request({ path: '/api/v1/project/' + operation, method: 'POST', body }, { signal: controller.signal })
      live(); if (controller.signal.aborted) throw new CreateError('DASHBOARD_HIDDEN')
      if (response.status !== 200) { const error = response.data && typeof response.data === 'object' && !Array.isArray(response.data) ? response.data.code : null; throw new CreateError(typeof error === 'string' ? error : 'DEPENDENCY_UNAVAILABLE') }
      return response.data
    } finally { requests.delete(controller) }
  }
  async function begin(input: CreateInput): Promise<void> {
    live(); if (hidden || working || record || readRecord()) throw new CreateError('PENDING_CREATE')
    working = true
    try {
      const request = parseRequest({ commit_id: commitID(), name: input.name.trim(), category: input.category, entry_path: input.entry_path.trim(), entry_text: input.entry_text })
      const requestHash = await createRequestHash(request)
      live(); if (hidden || readRecord()) throw new CreateError('PENDING_CREATE')
      save({ schemaVersion: 1, request, requestHash }); state = 'unknown'
    } finally { working = false }
  }
  async function readStatus(): Promise<{ state: CreateState; result?: CreateResult }> {
    const original = await exact()
    if (!original.result) throw new CreateError('PENDING_CREATE')
    const value = object(await request('commit_status', { project_id: original.result.project_id, commit_id: original.request.commit_id }))
    if (value.request_sha256 !== original.requestHash) throw new CreateError('REQUEST_MISMATCH')
    if (!['preparing', 'committed', 'conflict', 'failed'].includes(String(value.state))) invalid()
    const result = value.state === 'committed' ? parseResult(value.result, original.request) : undefined
    if (result && (result.project_id !== original.result.project_id || result.revision_id !== original.result.revision_id)) throw new CreateError('REQUEST_MISMATCH')
    state = value.state as CreateState
    return { state, ...(result ? { result } : {}) }
  }
  async function status(): Promise<{ state: CreateState; result?: CreateResult }> {
    if (working) throw new CreateError('PENDING_CREATE')
    working = true
    try { return await readStatus() } finally { working = false }
  }
  async function submit(): Promise<{ state: CreateState; result?: CreateResult }> {
    if (working) throw new CreateError('PENDING_CREATE')
    working = true
    try {
      const original = await exact()
      if (original.result) return await readStatus()
      const result = parseResult(await request('create', original.request as unknown as Json), original.request)
      save({ ...original, result }); state = 'committed'
      return { state, result }
    } finally { working = false }
  }
  async function clearTerminal() {
    if (working) throw new CreateError('PENDING_CREATE')
    working = true
    try {
      const original = await exact()
      if (!['committed', 'conflict', 'failed'].includes(state)) throw new CreateError('PENDING_CREATE')
      try {
        const archive = key + ':terminal:' + original.request.commit_id, raw = JSON.stringify(original), previous = storage.getItem(archive)
        if (previous !== null && previous !== raw) throw Error()
        storage.setItem(archive, raw); if (storage.getItem(archive) !== raw) throw Error()
        storage.removeItem(key); if (storage.getItem(key) !== null) throw Error()
      } catch { throw new CreateError('STORAGE_UNAVAILABLE') }
      record = undefined; state = 'unknown'
    } finally { working = false }
  }
  return { begin, submit, status, clearTerminal, dispose, pending: () => record ? clone(record) : undefined, state: () => state }
}
