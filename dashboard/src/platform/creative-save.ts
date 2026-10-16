import type { AppSDK, Json } from './types.ts'
import { commitID } from './asset-import.ts'
import type { CreativeMusicSource, ProjectOpen } from './project-content.ts'

const id = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const path = 'creative/project-content.json'
const scope = (sdk: AppSDK) => {
  const c = sdk.context.get()
  return JSON.stringify([c.userId, c.organizationId, c.stationId, c.appId, c.installationId, c.contextRevision])
}
const object = (value: unknown): Record<string, unknown> => value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : fail()
const fail = (): never => { throw new Error('保存记录或服务返回内容不正确，原提交仍保留。') }

interface CommitRequest {
  project_id: string; expected_revision_id: string; commit_id: string
  changes: Record<string, unknown>
}
interface Pending {
  version: 1; phase: 'write' | 'link'; request: CommitRequest
  document_sha256: string; request_sha256: string
}

function checkPending(value: unknown, projectID: string): Pending {
  const p = object(value), request = object(p.request)
  if (p.version !== 1 || !['write', 'link'].includes(String(p.phase)) || !/^[0-9a-f]{64}$/.test(String(p.document_sha256)) || !/^[0-9a-f]{64}$/.test(String(p.request_sha256)) || request.project_id !== projectID || ![request.expected_revision_id, request.commit_id].every(v => typeof v === 'string' && id.test(v))) fail()
  object(request.changes)
  return p as unknown as Pending
}

function result(value: unknown, projectID: string): ProjectOpen {
  const response = object(value), manifest = object(response.manifest)
  if (response.project_id !== projectID || typeof response.revision_id !== 'string' || !id.test(response.revision_id) || !Array.isArray(manifest.files) || !Array.isArray(manifest.domain_documents)) fail()
  return response as unknown as ProjectOpen
}

export function creativeDraft(direction: string, goal: string, brief: { client: string; audience: string; message: string; action: string }, musicSource: CreativeMusicSource | null, original?: Record<string, unknown>): Record<string, unknown> {
  if (!['promo', 'advertisement', 'brand_film', 'music_video'].includes(direction) || !goal.trim() || goal.length > 256) throw new Error('请填写创作方向和 1 至 256 个字符的传播目标。')
  const root = original ? structuredClone(original) : {
    schema_version: 1, kind: 'project-content', category: 'creative_video',
    creative_video: { direction, brief: { key: 'main', file_id: null, goal }, rights: [], concepts: [], selected_concept_key: null, sequences: [], shots: [], takes: [], edits: [], delivery_variants: [], reviews: [], packages: [] },
  }
  if (root.category !== 'creative_video') fail()
  const work = object(root.creative_video), oldBrief = object(work.brief)
  work.direction = direction
  work.brief = { ...oldBrief, goal: goal.trim(), ...Object.fromEntries(Object.entries(brief).map(([key, value]) => [key, value.trim() || null])) }
  work.music_source = musicSource
  return root
}

export function createCreativeSaver(sdk: AppSDK, projectID: string, storage: Storage) {
  if (!id.test(projectID) || sdk.context.get().appId !== 'project') fail()
  const initialScope = scope(sdk), key = `creative.save.v1:${initialScope}:${projectID}`
  let working = false
  const live = () => { if (scope(sdk) !== initialScope) throw new Error('会话已变化，原提交记录已保留。') }
  const load = (): Pending | null => {
    let raw: string | null
    try { raw = storage.getItem(key) } catch { throw new Error('无法读取保存恢复记录。') }
    if (!raw) return null
    if (raw.length > 4 * 1024 * 1024) fail()
    try { return checkPending(JSON.parse(raw), projectID) } catch { return fail() }
  }
  const store = (pending: Pending) => {
    live()
    const raw = JSON.stringify(pending)
    try { storage.setItem(key, raw); if (storage.getItem(key) !== raw) throw Error() }
    catch { throw new Error('无法写入保存恢复记录，尚未提交。') }
  }
  const clear = () => {
    try { storage.removeItem(key); if (storage.getItem(key) !== null) throw Error() }
    catch { throw new Error('提交已完成，但恢复记录未能清除；请刷新后核对修订。') }
  }
  async function call(request: CommitRequest): Promise<ProjectOpen> {
    live()
    const response = await sdk.app.request({ path: '/api/v1/project/commit', method: 'POST', body: request as unknown as Json })
    live()
    if (response.status !== 200) throw new Error(response.status === 409 ? '项目已有新修订；原提交记录已保留，请核对冲突。' : response.status === 403 ? '没有项目写入权限；原提交记录已保留。' : '提交结果未确认；请恢复原提交。')
    return result(response.data, projectID)
  }
  async function digest(text: string): Promise<string> {
    const hash = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text))
    return [...new Uint8Array(hash)].map(v => v.toString(16).padStart(2, '0')).join('')
  }
  async function run(): Promise<ProjectOpen> {
    if (working) throw new Error('已有保存正在进行。')
    working = true
    try {
      let pending = load()
      if (!pending) throw new Error('没有待恢复的提交。')
      if (await digest(JSON.stringify(pending.request)) !== pending.request_sha256) fail()
      let committed = await call(pending.request)
      if (pending.phase === 'write' && !committed.manifest.domain_documents.some(doc => doc.document_type === 'project-content')) {
        const file = committed.manifest.files.find(item => item.path === path)
        if (!file || !id.test(file.file_id)) throw new Error('创意视频业务文件未写入项目修订。')
        const documents = [...committed.manifest.domain_documents, { document_type: 'project-content', file_id: file.file_id }]
        const request: CommitRequest = { project_id: projectID, expected_revision_id: committed.revision_id, commit_id: commitID(), changes: { set_domain_documents: documents } }
        pending = { version: 1, phase: 'link', document_sha256: pending.document_sha256, request_sha256: await digest(JSON.stringify(request)), request }
        store(pending)
        committed = await call(pending.request)
      }
      const document = committed.manifest.domain_documents.find(doc => doc.document_type === 'project-content')
      const file = committed.manifest.files.find(item => item.file_id === document?.file_id)
      if (!file) throw new Error('创意视频业务文档未绑定到项目修订。')
      const read = object(await sdk.capabilities.call('artifact.read', {
        mode: 'metadata', content_ref: file.content_ref,
        access: { project_id: projectID, project_revision_id: committed.revision_id },
      } as unknown as Json))
      if (object(read.version).sha256 !== pending.document_sha256) fail()
      clear()
      return committed
    } finally { working = false }
  }
  async function beginDocument(opened: ProjectOpen, document: Record<string, unknown>) {
    live()
    if (working || load()) throw new Error('已有待确认的提交，请先恢复原提交。')
    if (opened.project_id !== projectID || !id.test(opened.revision_id) || opened.manifest.category !== 'creative_video') fail()
    if (document.schema_version !== 1 || document.kind !== 'project-content' || document.category !== 'creative_video') fail()
    const content = JSON.stringify(document)
    const existing = opened.manifest.domain_documents.find(doc => doc.document_type === 'project-content')
    const file = existing && opened.manifest.files.find(item => item.file_id === existing.file_id)
    if (existing && !file) fail()
    const request: CommitRequest = {
      project_id: projectID, expected_revision_id: opened.revision_id, commit_id: commitID(),
      changes: { upsert_files: [{ ...(file ? { file_id: file.file_id } : {}), path: file?.path || path, role: file?.role || 'review', text: content, mime: 'application/json' }] },
    }
    store({ version: 1, phase: 'write', request, document_sha256: await digest(content), request_sha256: await digest(JSON.stringify(request)) })
    return run()
  }
  async function begin(opened: ProjectOpen, original: Record<string, unknown> | undefined, direction: string, goal: string, brief: { client: string; audience: string; message: string; action: string }, musicSource: CreativeMusicSource | null) {
    return beginDocument(opened, creativeDraft(direction, goal, brief, musicSource, original))
  }
  return { begin, beginDocument, resume: run, pending: load }
}
