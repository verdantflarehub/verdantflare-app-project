import type { AppSDK, Context, Json } from './types.ts'
import { commitID, type RecoveryStorage } from './asset-import.ts'
import type { ProjectOpen } from './project-content.ts'

export interface EditChanges {
  set_metadata?: { name: string }
  upsert_files?: { file_id: string; path: string; role: string; text: string; mime: 'application/json' }[]
  set_domain_documents?: { document_type: string; file_id: string }[]
}
export interface EditRequest { project_id: string; expected_revision_id: string; commit_id: string; changes: EditChanges }
export interface EditRecord { request: EditRequest; requestHash: string }
export type EditState = 'unknown' | 'preparing' | 'committed' | 'conflict' | 'failed'
const identity = (c: Readonly<Context>) => JSON.stringify([c.userId, c.organizationId, c.stationId, c.appId, c.installationId, c.contextRevision])
const validID = (id: unknown) => typeof id === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(id)
const fail = (message: string): never => { throw new Error(message) }
const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T
/** Match Go CommitRequest / Changes field order and escaping for exact recovery. */
export function orderedEdit(request: EditRequest): EditRequest {
  const c = request.changes
  if (!validID(request.project_id) || !validID(request.expected_revision_id) || !validID(request.commit_id) || !c || Object.keys(request).some(k => !['project_id', 'expected_revision_id', 'commit_id', 'changes'].includes(k)) || Object.keys(c).some(k => !['set_metadata', 'upsert_files', 'set_domain_documents'].includes(k))) fail('保存请求不完整。')
  const changes: EditChanges = {}
  if (c.set_metadata) {
    const name = c.set_metadata.name
    if (Object.keys(c.set_metadata).length !== 1 || typeof name !== 'string' || !name.trim() || [...name].length > 256 || /[\ud800-\udfff]/u.test(name)) fail('请填写 1 至 256 个字符的名称。')
    changes.set_metadata = { name }
  }
  if (c.upsert_files) {
    if (c.upsert_files.length !== 1) fail('每次只能保存一份业务文档。')
    changes.upsert_files = c.upsert_files.map(f => {
      if (Object.keys(f).sort().join(',') !== 'file_id,mime,path,role,text' || !validID(f.file_id) || typeof f.path !== 'string' || typeof f.role !== 'string' || typeof f.text !== 'string' || f.mime !== 'application/json') fail('业务文档保存请求不完整。')
      return { file_id: f.file_id, path: f.path, role: f.role, text: f.text, mime: f.mime }
    })
  }
  if (c.set_domain_documents) {
    if (!c.upsert_files || !Array.isArray(c.set_domain_documents)) fail('业务文档关联不完整。')
    changes.set_domain_documents = c.set_domain_documents.map(d => {
      if (Object.keys(d).sort().join(',') !== 'document_type,file_id' || typeof d.document_type !== 'string' || !validID(d.file_id)) fail('业务文档关联不完整。')
      return { document_type: d.document_type, file_id: d.file_id }
    })
  }
  if (!changes.set_metadata && !changes.upsert_files) fail('没有可保存的修改。')
  const result = { project_id: request.project_id, expected_revision_id: request.expected_revision_id, commit_id: request.commit_id, changes }
  if (new TextEncoder().encode(JSON.stringify(result)).length > 1024 * 1024) fail('修改超过单次保存大小，请缩短内容。')
  return result
}
export async function editHash(request: EditRequest): Promise<string> {
  const raw = JSON.stringify(orderedEdit(request)).replace(/[<>&\u2028\u2029]/g, c => '\\u' + c.charCodeAt(0).toString(16).padStart(4, '0'))
  return [...new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(raw)))].map(b => b.toString(16).padStart(2, '0')).join('')
}
export function contentChanges(project: ProjectOpen, text: string): EditChanges {
  const doc = project.manifest.domain_documents.find(d => d.document_type === 'project-content')
  const file = doc ? project.manifest.files.find(f => f.file_id === doc.file_id) : undefined
  if (doc && !file) fail('业务文档不在当前修订中。')
  const fileID = file?.file_id || commitID()
  return { upsert_files: [{ file_id: fileID, path: file?.path || 'project-content.json', role: file?.role || 'domain', text, mime: 'application/json' }], ...(!doc ? { set_domain_documents: [...project.manifest.domain_documents, { document_type: 'project-content', file_id: fileID }] } : {}) }
}
/** Recovery validates the persisted operation, even when reopened from another editor. */
export async function verifyEditReadback(request: EditRequest, project: ProjectOpen, document: (id: string) => Promise<{ text: string }>): Promise<void> {
  if (project.project_id !== request.project_id) fail('保存回读项目不匹配。')
  if (request.changes.set_metadata && project.manifest.name !== request.changes.set_metadata.name) fail('名称保存回读不匹配。')
  const saved = request.changes.upsert_files?.[0]
  if (saved) {
    const doc = project.manifest.domain_documents.find(d => d.document_type === 'project-content')
    if (!doc || (await document(doc.file_id)).text !== saved.text) fail('业务内容保存回读不匹配。')
  }
}
export function createProjectEditor(sdk: AppSDK, storage: RecoveryStorage, projectID: string) {
  const context = sdk.context.get(), initial = identity(context)
  if (context.appId !== 'project' || !validID(projectID)) fail('项目地址无效。')
  const key = 'project.edit.v1:' + JSON.stringify([context.userId, context.organizationId, context.stationId, context.installationId, projectID])
  let disposed = false, hidden = false, working = false, state: EditState = 'unknown'
  const controllers = new Set<AbortController>()
  function live() { if (disposed || identity(sdk.context.get()) !== initial) fail('会话已变化，请重新打开项目。原保存记录已保留。') }
  function read(): EditRecord | undefined {
    try {
      const raw = storage.getItem(key)
      if (!raw) return
      if (raw.length > 1024 * 1024 + 4096) fail('原保存记录过大。')
      const record = JSON.parse(raw) as EditRecord
      if (!record || Object.keys(record).sort().join(',') !== 'request,requestHash' || !/^[0-9a-f]{64}$/.test(record.requestHash) || record.request.project_id !== projectID) fail('原保存记录不完整。')
      orderedEdit(record.request)
      return record
    } catch { return fail('无法读取原保存记录，请保留记录并检查本机存储。') }
  }
  let record = read()
  function dispose() { disposed = true; controllers.forEach(c => c.abort()); controllers.clear() }
  const stopContext = sdk.context.subscribe(c => { if (identity(c) !== initial) dispose() })
  const stopLifecycle = sdk.lifecycle.subscribe(event => { if (event === 'dispose') dispose(); else hidden = event === 'hidden' })
  async function exact() {
    live(); const saved = read()
    if (!record || !saved || JSON.stringify(saved) !== JSON.stringify(record) || await editHash(record.request) !== record.requestHash) fail('保存记录与原请求不一致，已停止操作。')
    live()
    if (JSON.stringify(read()) !== JSON.stringify(record)) fail('原保存记录已被其他页面修改。')
    return record!
  }
  async function request(operation: 'edit' | 'commit_status', body: Json) {
    live(); if (hidden) fail('页面已隐藏，请返回后恢复原保存。')
    const controller = new AbortController(); controllers.add(controller)
    try {
      const response = await sdk.app.request({ path: '/api/v1/project/' + operation, method: 'POST', body }, { signal: controller.signal })
      live()
      if (response.status !== 200) {
        const code = response.data && typeof response.data === 'object' && !Array.isArray(response.data) ? response.data.code : ''
        if (code === 'REVISION_CONFLICT') state = 'conflict'
        else if (code === 'INVALID_ARGUMENT' || code === 'CONTENT_NOT_READY') state = 'failed'
        fail(code === 'PERMISSION_DENIED' ? '没有保存权限，原请求已保留。' : code === 'REVISION_CONFLICT' ? '项目已更新，请保留输入，读取最新修订后重新保存。' : '未确认保存结果，请查询或恢复原保存。')
      }
      return response.data as unknown as Record<string, unknown>
    } finally { controllers.delete(controller) }
  }
  async function begin(revision: string, changes: EditChanges) {
    live(); if (record || read() || working || hidden) fail('请先确认原保存结果。')
    working = true
    try {
      const request = orderedEdit({ project_id: projectID, expected_revision_id: revision, commit_id: commitID(), changes })
      const next = { request, requestHash: await editHash(request) }
      live(); if (hidden || read()) fail('请先确认原保存结果。')
      const raw = JSON.stringify(next)
      storage.setItem(key, raw)
      if (storage.getItem(key) !== raw) fail('无法保存恢复记录，本次未提交。')
      record = next; state = 'unknown'
    } finally { working = false }
  }
  function result(value: unknown): string {
    const v = value as { project_id?: unknown; revision_id?: unknown; manifest?: { project_id?: unknown } }
    if (!v || v.project_id !== projectID || v.manifest?.project_id !== projectID || !validID(v.revision_id)) fail('保存结果不匹配，请保留原请求并查询。')
    return v.revision_id as string
  }
  async function run(status: boolean): Promise<{ state: EditState; revision?: string }> {
    if (working) fail('正在确认保存结果。')
    working = true
    try {
      const original = await exact()
      const response = await request(status ? 'commit_status' : 'edit', (status ? { project_id: projectID, commit_id: original.request.commit_id } : original.request) as unknown as Json)
      if (!status) { const revision = result(response); state = 'committed'; return { state, revision } }
      if (response.request_sha256 !== original.requestHash || !['preparing', 'committed', 'conflict', 'failed'].includes(String(response.state))) fail('保存状态与原请求不匹配。')
      const revision = response.state === 'committed' ? result(response.result) : undefined
      state = response.state as EditState
      return { state, revision }
    } finally { working = false }
  }
  async function clearTerminal() {
    const original = await exact()
    if (working || !['committed', 'conflict', 'failed'].includes(state)) fail('保存结果尚未确认，请恢复原请求。')
    const archive = key + ':' + original.request.commit_id, raw = JSON.stringify(original), old = storage.getItem(archive)
    if (old !== null && old !== raw) fail('原保存归档不一致。')
    storage.setItem(archive, raw)
    if (storage.getItem(archive) !== raw) fail('无法保留原保存记录。')
    storage.removeItem(key)
    if (storage.getItem(key) !== null) fail('无法清理已结束记录。')
    record = undefined; state = 'unknown'
  }
  return { begin, submit: () => run(false), status: () => run(true), clearTerminal, pending: () => record ? clone(record) : undefined, state: () => state, dispose: () => { dispose(); stopContext(); stopLifecycle() } }
}
