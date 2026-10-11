import type { AppSDK, Context, Json, Unsubscribe } from './types.ts'

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const hashPattern = /^[0-9a-f]{64}$/
const invalid = (): never => { throw new ImportError('INVALID_RESPONSE') }
const object = (v: unknown): Record<string, unknown> => v && typeof v === 'object' && !Array.isArray(v) ? v as Record<string, unknown> : invalid()
const id = (v: unknown): string => typeof v === 'string' && uuid.test(v) ? v : invalid()
const text = (v: unknown): string => typeof v === 'string' ? v : invalid()
const purposeOK = (v: unknown): v is string => typeof v === 'string' && v.trim().length > 0 && [...v].length <= 256 && !/[\u0000-\u001f\u007f\ud800-\udfff]/u.test(v)
const principal = (c: Readonly<Context>) => [c.userId, c.organizationId, c.stationId, c.appId, c.installationId]
const identity = (c: Readonly<Context>) => JSON.stringify([...principal(c), c.contextRevision])
export interface AssetRef { asset_id: string; asset_version_id: string; purpose: string }
export interface ImportProject { project_id: string; head_revision_id: string; name: string; category: string }
export interface ImportTarget { project_id: string; revision_id: string; name: string; asset_refs: AssetRef[] }
export interface ImportRequest { project_id: string; expected_revision_id: string; commit_id: string; asset_id: string; asset_version_id: string; purpose: string }
export interface ImportRecord { schemaVersion: 1; request: ImportRequest; requestHash: string }
export interface ImportResult { project_id: string; revision_id: string }
export type ImportState = 'unknown' | 'preparing' | 'committed' | 'conflict' | 'failed'
export interface RecoveryStorage { getItem(key: string): string | null; setItem(key: string, value: string): void; removeItem(key: string): void }
export class ImportError extends Error { constructor(readonly code: string) { super(code) } }
export function importError(error: unknown): string {
  const e = error && typeof error === 'object' ? error as { code?: unknown; message?: unknown } : {}
  const code = typeof e.code === 'string' ? e.code : e.message
  const messages: Record<string, string> = {
    PERMISSION_DENIED: '当前身份没有目标项目写入权限，或不能使用所选资产版本。',
    REVISION_CONFLICT: '目标项目已更新。请查询原提交结果，确认冲突后重新读取目标修订。',
    PENDING_IMPORT: '此资产已有未确认的提交，请先查询或重试原提交。',
    NOT_FOUND: '尚未查到原提交，请保留记录并重试原请求。',
    STORAGE_UNAVAILABLE: '无法保存提交恢复记录，本次未发起新的保存。请检查桌面或浏览器存储。',
    INVALID_RESPONSE: '返回内容或恢复记录不完整，已停止操作并保留原提交。',
    REQUEST_MISMATCH: '提交记录与服务端请求指纹不一致，已停止操作。',
    IDEMPOTENCY_CONFLICT: '原提交编号对应不同请求，已停止操作并保留记录。',
    INVALID_PURPOSE: '请填写 1 至 256 个字符的用途。',
    SESSION_REVOKED: '会话已变化，请重新打开应用；原提交恢复记录仍保留。',
  }
  return typeof code === 'string' && messages[code] || '暂未确认保存结果，原提交记录仍保留。请查询结果或重试原提交。'
}
function parseRefs(value: unknown): AssetRef[] {
  if (!Array.isArray(value) || value.length > 10000) return invalid()
  return value.map(raw => { const v = object(raw); return { asset_id: id(v.asset_id), asset_version_id: id(v.asset_version_id), purpose: text(v.purpose) } })
}
function parseRecord(value: unknown, asset: string, version: string): ImportRecord {
  const v = object(value), r = object(v.request)
  if (Object.keys(v).sort().join(',') !== 'request,requestHash,schemaVersion' || v.schemaVersion !== 1 || !hashPattern.test(text(v.requestHash)) || Object.keys(r).sort().join(',') !== 'asset_id,asset_version_id,commit_id,expected_revision_id,project_id,purpose' || r.asset_id !== asset || r.asset_version_id !== version || !purposeOK(r.purpose)) invalid()
  return { schemaVersion: 1, requestHash: v.requestHash as string, request: { project_id: id(r.project_id), expected_revision_id: id(r.expected_revision_id), commit_id: id(r.commit_id), asset_id: asset, asset_version_id: version, purpose: r.purpose as string } }
}
function parseResult(value: unknown, request: ImportRequest): ImportResult {
  const v = object(value), manifest = object(v.manifest)
  if (v.project_id !== request.project_id || manifest.project_id !== request.project_id || !parseRefs(manifest.asset_refs).some(ref => ref.asset_id === request.asset_id && ref.asset_version_id === request.asset_version_id && ref.purpose === request.purpose)) invalid()
  return { project_id: request.project_id, revision_id: id(v.revision_id) }
}
export function commitID(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  let milliseconds = Date.now()
  for (let i = 5; i >= 0; i--) { bytes[i] = milliseconds % 256; milliseconds = Math.floor(milliseconds / 256) }
  bytes[6] = (bytes[6] & 15) | 112; bytes[8] = (bytes[8] & 63) | 128
  const hex = [...bytes].map(b => b.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}
/** Same fixed CommitRequest shape used by Project.UseAsset, including Go JSON escapes. */
export async function requestHash(request: ImportRequest, previous: AssetRef[]): Promise<string> {
  const refs = previous.filter(ref => !(ref.asset_id === request.asset_id && ref.purpose === request.purpose)).map(ref => ({ asset_id: ref.asset_id, asset_version_id: ref.asset_version_id, purpose: ref.purpose }))
  refs.push({ asset_id: request.asset_id, asset_version_id: request.asset_version_id, purpose: request.purpose })
  const payload = { project_id: request.project_id, expected_revision_id: request.expected_revision_id, commit_id: request.commit_id, changes: { set_asset_refs: refs } }
  const json = JSON.stringify(payload).replace(/[<>&\u2028\u2029]/g, c => '\\u' + c.charCodeAt(0).toString(16).padStart(4, '0'))
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(json))
  return [...new Uint8Array(digest)].map(b => b.toString(16).padStart(2, '0')).join('')
}
export function createAssetImport(sdk: AppSDK, storage: RecoveryStorage, asset: string, version: string) {
  id(asset); id(version)
  const context = sdk.context.get(), initial = identity(context)
  if (context.appId !== 'project') throw new ImportError('SESSION_REVOKED')
  const key = 'project.asset-import.v1:' + JSON.stringify([...principal(context), asset, version])
  let disposed = false, hidden = false, writing = false, record: ImportRecord | undefined, state: ImportState = 'unknown'
  const pending = new Map<AbortController, boolean>(), subscriptions: Unsubscribe[] = []
  function live() { if (disposed || identity(sdk.context.get()) !== initial) throw new ImportError('SESSION_REVOKED') }
  function readRecord() {
    let raw: string | null
    try { raw = storage.getItem(key) } catch { throw new ImportError('STORAGE_UNAVAILABLE') }
    if (!raw) return undefined
    if (raw.length > 8192) invalid()
    try { return parseRecord(JSON.parse(raw), asset, version) } catch { return invalid() }
  }
  function saveRecord(next: ImportRecord) {
    try { storage.setItem(key, JSON.stringify(next)); if (storage.getItem(key) !== JSON.stringify(next)) throw new Error() } catch { throw new ImportError('STORAGE_UNAVAILABLE') }
  }
  record = readRecord()
  function dispose() { if (disposed) return; disposed = true; pending.forEach((_read, controller) => controller.abort()); pending.clear(); subscriptions.splice(0).forEach(stop => stop()) }
  subscriptions.push(sdk.context.subscribe(c => { if (identity(c) !== initial) dispose() }))
  subscriptions.push(sdk.lifecycle.subscribe(event => { if (event === 'dispose') dispose(); else { hidden = event === 'hidden'; if (hidden) pending.forEach((read, controller) => { if (read) controller.abort() }) } }))
  async function request(operation: 'list' | 'open' | 'use_asset' | 'commit_status', body: Json) {
    live(); if (hidden) throw new ImportError('DASHBOARD_HIDDEN')
    const controller = new AbortController(); pending.set(controller, operation !== 'use_asset')
    try {
      const response = await sdk.app.request({ method: 'POST', path: '/api/v1/project/' + operation, body }, { signal: controller.signal })
      live(); if (controller.signal.aborted) throw new ImportError('REQUEST_CANCELLED')
      if (response.status !== 200) {
        const error = response.data && typeof response.data === 'object' && !Array.isArray(response.data) ? response.data.code : undefined
        throw new ImportError(typeof error === 'string' ? error : 'DEPENDENCY_UNAVAILABLE')
      }
      return object(response.data)
    } finally { pending.delete(controller) }
  }
  async function list(cursor = ''): Promise<{ items: ImportProject[]; next_cursor: string }> {
    if (cursor) id(cursor)
    const v = await request('list', { require_write: true, limit: 50, ...(cursor ? { cursor } : {}) })
    if (!Array.isArray(v.items) || v.items.length > 100) return invalid()
    const items = v.items.map(raw => { const p = object(raw); return { project_id: id(p.project_id), head_revision_id: id(p.head_revision_id), name: text(p.name), category: text(p.category) } })
    const next = v.next_cursor === '' ? '' : id(v.next_cursor)
    if (new Set(items.map(p => p.project_id)).size !== items.length || (cursor && next === cursor)) invalid()
    return { items, next_cursor: next }
  }
  async function open(project: string): Promise<ImportTarget> {
    const v = await request('open', { project_id: id(project), require_write: true }), manifest = object(v.manifest)
    if (v.project_id !== project || manifest.project_id !== project || v.revision_id !== v.head_revision_id) invalid()
    return { project_id: project, revision_id: id(v.revision_id), name: text(manifest.name), asset_refs: parseRefs(manifest.asset_refs) }
  }
  async function begin(target: ImportTarget, purpose: string): Promise<ImportRecord> {
    live(); if (hidden || writing || record || readRecord()) throw new ImportError('PENDING_IMPORT')
    if (!purposeOK(purpose)) throw new ImportError('INVALID_PURPOSE')
    writing = true
    try {
      const req: ImportRequest = { project_id: id(target.project_id), expected_revision_id: id(target.revision_id), commit_id: commitID(), asset_id: asset, asset_version_id: version, purpose }
      const next: ImportRecord = { schemaVersion: 1, request: req, requestHash: await requestHash(req, parseRefs(target.asset_refs)) }
      live(); if (hidden || readRecord()) throw new ImportError('PENDING_IMPORT')
      saveRecord(next); record = next; state = 'unknown'
      return JSON.parse(JSON.stringify(next)) as ImportRecord
    } finally { writing = false }
  }
  function exactRecord() {
    live()
    const persisted = readRecord()
    if (!record || !persisted || persisted.requestHash !== record.requestHash || JSON.stringify(persisted.request) !== JSON.stringify(record.request)) throw new ImportError('REQUEST_MISMATCH')
    return record
  }
  async function submit(): Promise<ImportResult> {
    if (writing) throw new ImportError('PENDING_IMPORT')
    const original = exactRecord(); writing = true
    try {
      const value = await request('use_asset', original.request as unknown as Json)
      const result = parseResult(value, original.request); state = 'committed'
      return result
    } finally { writing = false }
  }
  async function status(): Promise<{ state: ImportState; result?: ImportResult }> {
    const original = exactRecord()
    const value = await request('commit_status', { project_id: original.request.project_id, commit_id: original.request.commit_id })
    if (value.request_sha256 !== original.requestHash) throw new ImportError('REQUEST_MISMATCH')
    if (!['preparing', 'committed', 'conflict', 'failed'].includes(text(value.state))) invalid()
    const result = value.state === 'committed' ? parseResult(value.result, original.request) : undefined
    if (value.state !== 'committed' && value.result != null) invalid()
    state = value.state as ImportState
    return { state, ...(result ? { result } : {}) }
  }
  function clearTerminal() {
    exactRecord()
    if (!['conflict', 'failed', 'committed'].includes(state) || writing) throw new ImportError('PENDING_IMPORT')
    try { storage.removeItem(key) } catch { throw new ImportError('STORAGE_UNAVAILABLE') }
    record = undefined; state = 'unknown'
  }
  return { list, open, begin, submit, status, clearTerminal, dispose, pending: () => record ? JSON.parse(JSON.stringify(record)) as ImportRecord : undefined }
}
