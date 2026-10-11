import type { AppSDK } from './types.ts'
import type { ProjectOpen, ProjectContent, DramaWorldCandidate, WorldRegistration, WorldRegistrationStatus, WorldResult } from './project-content.ts'
import { commitID, type RecoveryStorage } from './asset-import.ts'
import { canonical } from './drama-publication-package.ts'
import { createPublicationAPI, PublicationError, publicationPrincipal } from './publication-client.ts'

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const hash = /^[a-f0-9]{64}$/
const fail = (): never => { throw new PublicationError('INVALID_RESPONSE') }
const object = (v: unknown): Record<string, unknown> => v && typeof v === 'object' && !Array.isArray(v) ? v as Record<string, unknown> : fail()
const id = (v: unknown): string => typeof v === 'string' && uuid.test(v) ? v : fail()
const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T
export interface PublicationPointer { project_id: string; prepared_revision_id: string; candidate_key: string; commit_id: string }
export interface PublicationReceiptPointer extends PublicationPointer { asset_id: string; asset_version_id: string }
export interface PublicationSnapshot { project: ProjectOpen; content: ProjectContent }
export interface PublicationIntent {
  schemaVersion: 1; project_id: string; origin_revision_id: string; candidate_key: string; commit_id: string; prepare_commit_id: string
  prepared_revision_id?: string; request?: WorldRegistration; prepare_hash?: string; prepare_state?: string
  published_asset_id?: string; published_version_id?: string; receipt_hash?: string
  association?: { expected_revision_id: string; association_commit_id: string; request_hash?: string; state?: string }
  completed_revision_id?: string
}
interface CommitStatus { state: 'preparing' | 'committed' | 'conflict' | 'failed'; request_sha256: string; result?: ProjectOpen; current_revision_id?: string; error_code?: string }
export function normalizeRegistration(request: WorldRegistration): WorldRegistration {
  return { ...request, file_ids: [...request.file_ids].sort(), subjects: [...request.subjects].sort(), depends_on: [...(request.depends_on || [])].sort((a, b) => {
    const left = [a.asset_id, a.asset_version_id, a.purpose].join('\u0000'), right = [b.asset_id, b.asset_version_id, b.purpose].join('\u0000'); return left < right ? -1 : left > right ? 1 : 0
  }) }
}
export function publicationPointer(input: unknown): PublicationPointer {
  const p = object(input)
  if (typeof p.candidate_key !== 'string' || !/^[a-zA-Z0-9_-]{1,256}$/.test(p.candidate_key)) fail()
  return { project_id: id(p.project_id), prepared_revision_id: id(p.prepared_revision_id), candidate_key: p.candidate_key as string, commit_id: id(p.commit_id) }
}
export function receiptPointer(input: unknown): PublicationReceiptPointer { const p = object(input); return { ...publicationPointer(p), asset_id: id(p.asset_id), asset_version_id: id(p.asset_version_id) } }
function candidateAt(snapshot: PublicationSnapshot, project: string, key: string, revision?: string): DramaWorldCandidate {
  if (snapshot.project.project_id !== project || revision && snapshot.project.revision_id !== revision || !uuid.test(snapshot.project.revision_id)) fail()
  const candidate = snapshot.content.ai_short_drama?.world_candidates.find(c => c.key === key)
  if (!candidate) throw new PublicationError('REQUEST_MISMATCH')
  return candidate
}
function exactRequest(candidate: DramaWorldCandidate, project: string, commit: string): WorldRegistration {
  const r = candidate.registration_request
  if (candidate.review.state !== 'approved' || !candidate.publication_record_file_id || !r || r.source_project_id !== project || r.commit_id !== commit) throw new PublicationError('REQUEST_MISMATCH')
  return normalizeRegistration(r)
}
function commitStatus(value: unknown, project: string, previousHash?: string): CommitStatus {
  const s = object(value)
  if (!['preparing', 'committed', 'conflict', 'failed'].includes(String(s.state)) || typeof s.request_sha256 !== 'string' || !hash.test(s.request_sha256)) fail()
  if (previousHash && s.request_sha256 !== previousHash) throw new PublicationError('REQUEST_MISMATCH')
  if (s.state === 'committed') { const result = object(s.result); if (result.project_id !== project) fail(); id(result.revision_id) }
  return s as unknown as CommitStatus
}
function validateReceipt(value: unknown, pointer: PublicationReceiptPointer, request: WorldRegistration): Extract<WorldRegistrationStatus, { state: 'committed' }> {
  const s = object(value), result = object(s.result), manifest = object(result.manifest), source = object(manifest.source)
  if (s.state !== 'committed' || typeof s.request_sha256 !== 'string' || !hash.test(s.request_sha256) || result.asset_id !== pointer.asset_id || result.asset_version_id !== pointer.asset_version_id || source.project_id !== request.source_project_id || source.project_revision_id !== request.source_revision_id || source.relation !== request.relation || (source.review_file_id || undefined) !== request.review_file_id || manifest.asset_id !== pointer.asset_id || manifest.name !== request.name || manifest.asset_type !== request.asset_type || canonical(manifest.subjects) !== canonical(request.subjects) || canonical(manifest.depends_on || []) !== canonical(request.depends_on || [])) throw new PublicationError('REQUEST_MISMATCH')
  if (!Array.isArray(manifest.files) || canonical(manifest.files.map(f => id(object(f).file_id)).sort()) !== canonical([...request.file_ids].sort())) throw new PublicationError('REQUEST_MISMATCH')
  const ref = object(result.manifest_ref); id(ref.store_id); id(ref.artifact_id); id(ref.version_id)
  return clone(s as unknown as Extract<WorldRegistrationStatus, { state: 'committed' }>)
}
export function createProjectPublication(sdk: AppSDK, storage: RecoveryStorage, project: string, candidate: string, readSnapshot: (revision?: string) => Promise<PublicationSnapshot>) {
  id(project); if (!/^[a-zA-Z0-9_-]{1,256}$/.test(candidate)) fail()
  const api = createPublicationAPI(sdk, 'project')
  const key = 'project.publication.v1:' + JSON.stringify([...publicationPrincipal(sdk.context.get()), project, candidate])
  let intent: PublicationIntent | undefined, working = false, confirmedConflict: string | undefined
  function readRecord(): PublicationIntent | undefined {
    let raw: string | null
    try { raw = storage.getItem(key) } catch { throw new PublicationError('STORAGE_UNAVAILABLE') }
    if (!raw) return undefined
    if (raw.length > 1024 * 1024) fail()
    try {
      const value = object(JSON.parse(raw))
      if (value.schemaVersion !== 1 || value.project_id !== project || value.candidate_key !== candidate) fail()
      if (Object.keys(value).some(k => !['schemaVersion', 'project_id', 'origin_revision_id', 'candidate_key', 'commit_id', 'prepare_commit_id', 'prepared_revision_id', 'request', 'prepare_hash', 'prepare_state', 'published_asset_id', 'published_version_id', 'receipt_hash', 'association', 'completed_revision_id'].includes(k))) fail()
      for (const key of ['project_id', 'origin_revision_id', 'commit_id', 'prepare_commit_id']) id(value[key])
      for (const key of ['prepared_revision_id', 'published_asset_id', 'published_version_id', 'completed_revision_id']) if (value[key] !== undefined) id(value[key])
      for (const key of ['prepare_hash', 'receipt_hash']) if (value[key] !== undefined && (typeof value[key] !== 'string' || !hash.test(value[key] as string))) fail()
      if (value.prepare_state !== undefined && !['preparing', 'committed', 'conflict', 'failed'].includes(String(value.prepare_state))) fail()
      if ((value.prepare_state !== undefined) !== (value.prepare_hash !== undefined) || (value.published_asset_id !== undefined) !== (value.published_version_id !== undefined)) fail()
      if (value.request !== undefined) {
        const r = object(value.request)
        if (r.commit_id !== value.commit_id || r.source_project_id !== project || !value.prepared_revision_id || !Array.isArray(r.file_ids) || !r.file_ids.length || !Array.isArray(r.subjects) || r.subjects.some(v => typeof v !== 'string') || r.depends_on !== undefined && !Array.isArray(r.depends_on)) fail()
        id(r.source_revision_id); (r.file_ids as unknown[]).forEach(id)
      }
      if (value.association) {
        const a = object(value.association); id(a.expected_revision_id); id(a.association_commit_id)
        if (!value.prepared_revision_id || !value.request || !value.published_asset_id || !value.receipt_hash || Object.keys(a).some(k => !['expected_revision_id', 'association_commit_id', 'request_hash', 'state'].includes(k))) fail()
        if (a.state !== undefined && !['preparing', 'committed', 'conflict', 'failed'].includes(String(a.state))) fail()
        if (a.request_hash !== undefined && (typeof a.request_hash !== 'string' || !hash.test(a.request_hash))) fail()
        if ((a.state !== undefined) !== (a.request_hash !== undefined)) fail()
      }
      return value as unknown as PublicationIntent
    } catch { return fail() }
  }
  function save(next: PublicationIntent) {
    api.live()
    try { const raw = JSON.stringify(next); storage.setItem(key, raw); if (storage.getItem(key) !== raw) throw Error() } catch { throw new PublicationError('STORAGE_UNAVAILABLE') }
    intent = clone(next)
  }
  function exact() { api.live(); const persisted = readRecord(); if (!intent || !persisted || canonical(persisted) !== canonical(intent)) throw new PublicationError('REQUEST_MISMATCH'); return intent }
  try { intent = readRecord() } catch (error) { api.dispose(); throw error }
  async function fixedRequest(record: PublicationIntent) {
    if (!record.prepared_revision_id) throw new PublicationError('NOT_READY')
    const prepared = await readSnapshot(record.prepared_revision_id), c = candidateAt(prepared, project, candidate, record.prepared_revision_id)
    const request = exactRequest(c, project, record.commit_id)
    if (record.request && canonical(normalizeRegistration(record.request)) !== canonical(request)) throw new PublicationError('REQUEST_MISMATCH')
    return request
  }
  async function prepare(originRevision: string): Promise<string> {
    if (working) throw new PublicationError('NOT_READY')
    working = true
    try {
      if (!intent) {
        const snapshot = await readSnapshot(id(originRevision)), c = candidateAt(snapshot, project, candidate, originRevision)
        if (c.review.state !== 'approved' || !c.publication_record_file_id) throw new PublicationError('NOT_READY')
        const next: PublicationIntent = { schemaVersion: 1, project_id: project, origin_revision_id: originRevision, candidate_key: candidate, commit_id: c.registration_request?.commit_id || commitID(), prepare_commit_id: commitID() }
        if (c.registration_request) { next.prepared_revision_id = originRevision; next.request = exactRequest(c, project, next.commit_id) }
        save(next)
      }
      let original = exact()
      if (!original.prepared_revision_id) {
        const status = commitStatus(await api.call('publication_prepare', { project_id: project, origin_revision_id: original.origin_revision_id, candidate_key: candidate, commit_id: original.commit_id, prepare_commit_id: original.prepare_commit_id }), project, original.prepare_hash)
        save({ ...original, prepare_hash: status.request_sha256, prepare_state: status.state, ...(status.state === 'committed' ? { prepared_revision_id: status.result!.revision_id } : {}) })
        if (status.state !== 'committed') throw new PublicationError(status.state === 'conflict' || status.state === 'failed' ? 'TERMINAL_FAILURE' : 'NOT_READY')
        original = exact()
      }
      const request = await fixedRequest(original)
      save({ ...original, request })
      return worldPath()
    } finally { working = false }
  }
  function worldPath(): string {
    const original = exact()
    if (!original.prepared_revision_id) throw new PublicationError('NOT_READY')
    return '/publish?' + new URLSearchParams({ project, prepared: original.prepared_revision_id, candidate, commit: original.commit_id })
  }
  async function restartPreparation(): Promise<string> {
    if (working) throw new PublicationError('NOT_READY')
    working = true
    try {
      const original = exact()
      if (original.prepared_revision_id || original.association) throw new PublicationError('NOT_READY')
      const status = commitStatus(await api.call('publication_prepare', { project_id: project, origin_revision_id: original.origin_revision_id, candidate_key: candidate, commit_id: original.commit_id, prepare_commit_id: original.prepare_commit_id }), project, original.prepare_hash)
      save({ ...original, prepare_hash: status.request_sha256, prepare_state: status.state, ...(status.state === 'committed' ? { prepared_revision_id: status.result!.revision_id } : {}) })
      if (status.state !== 'conflict' && status.state !== 'failed') throw new PublicationError('NOT_READY')
      const current = await readSnapshot()
      candidateAt(current, project, candidate)
      const terminal = exact(), raw = JSON.stringify(terminal), archive = key + ':terminal:' + terminal.commit_id
      try {
        const previous = storage.getItem(archive)
        if (previous !== null && previous !== raw) throw Error()
        storage.setItem(archive, raw)
        if (storage.getItem(archive) !== raw) throw Error()
        storage.removeItem(key)
        if (storage.getItem(key) !== null) throw Error()
      } catch { throw new PublicationError('STORAGE_UNAVAILABLE') }
      intent = undefined; confirmedConflict = undefined
      return current.project.revision_id
    } finally { working = false }
  }
  async function recoverPointer(input: PublicationReceiptPointer) {
    const pointer = receiptPointer(input)
    if (pointer.project_id !== project || pointer.candidate_key !== candidate) throw new PublicationError('REQUEST_MISMATCH')
    if (!intent) {
      const snapshot = await readSnapshot(pointer.prepared_revision_id), c = candidateAt(snapshot, project, candidate, pointer.prepared_revision_id)
      save({ schemaVersion: 1, project_id: project, origin_revision_id: pointer.prepared_revision_id, prepared_revision_id: pointer.prepared_revision_id, candidate_key: candidate, commit_id: pointer.commit_id, prepare_commit_id: commitID(), request: exactRequest(c, project, pointer.commit_id) })
    }
    const original = exact()
    if (original.prepared_revision_id !== pointer.prepared_revision_id || original.commit_id !== pointer.commit_id || original.published_asset_id && original.published_asset_id !== pointer.asset_id || original.published_version_id && original.published_version_id !== pointer.asset_version_id) throw new PublicationError('REQUEST_MISMATCH')
    const request = await fixedRequest(original)
    const status = validateReceipt(await api.call('publication_receipt', pointer), pointer, request)
    if (original.receipt_hash && original.receipt_hash !== status.request_sha256) throw new PublicationError('REQUEST_MISMATCH')
    save({ ...original, request, published_asset_id: pointer.asset_id, published_version_id: pointer.asset_version_id, receipt_hash: status.request_sha256 })
    return status
  }
  function matchPointer(input: PublicationReceiptPointer, original: PublicationIntent) {
    if (input.project_id !== project || input.candidate_key !== candidate || input.prepared_revision_id !== original.prepared_revision_id || input.commit_id !== original.commit_id || input.asset_id !== original.published_asset_id || input.asset_version_id !== original.published_version_id) throw new PublicationError('REQUEST_MISMATCH')
  }
  async function replayAssociation(pointer: PublicationReceiptPointer, original: PublicationIntent): Promise<{ revision_id: string }> {
    matchPointer(pointer, original)
    const status = commitStatus(await api.call('publication_associate', { ...pointer, expected_revision_id: original.association!.expected_revision_id, association_commit_id: original.association!.association_commit_id }), project, original.association!.request_hash)
    confirmedConflict = status.state === 'conflict' ? original.association!.association_commit_id : undefined
    save({ ...original, association: { ...original.association!, state: status.state, request_hash: status.request_sha256 }, ...(status.state === 'committed' ? { completed_revision_id: status.result!.revision_id } : {}) })
    if (status.state !== 'committed') throw new PublicationError(status.state === 'conflict' ? 'REVISION_CONFLICT' : status.state === 'failed' ? 'TERMINAL_FAILURE' : 'NOT_READY')
    return { revision_id: status.result!.revision_id }
  }
  async function associate(input: PublicationReceiptPointer): Promise<{ revision_id: string; result?: WorldResult }> {
    if (working) throw new PublicationError('NOT_READY')
    working = true
    try {
      const pointer = receiptPointer(input)
      // The service can recover the stored association plan without World.
      // Query/replay that exact operation before requesting a new receipt.
      if (intent?.association && confirmedConflict !== intent.association.association_commit_id) return await replayAssociation(pointer, exact())
      const receipt = await recoverPointer(pointer)
      let original = exact()
      if (!original.association || original.association.state === 'conflict') {
        const current = await readSnapshot(), currentCandidate = candidateAt(current, project, candidate)
        if (canonical(exactRequest(currentCandidate, project, original.commit_id)) !== canonical(original.request)) throw new PublicationError('REQUEST_MISMATCH')
        if (currentCandidate.registration_status?.state === 'committed' && canonical(currentCandidate.registration_status) === canonical(receipt)) { save({ ...original, completed_revision_id: current.project.revision_id }); return { revision_id: current.project.revision_id, result: receipt.result } }
        save({ ...original, association: { expected_revision_id: current.project.revision_id, association_commit_id: commitID() } })
        original = exact()
      }
      return { ...await replayAssociation(pointer, original), result: receipt.result }
    } finally { working = false }
  }
  return { prepare, restartPreparation, worldPath, recoverPointer, associate, dispose: api.dispose, pending: () => intent ? clone(intent) : undefined }
}
