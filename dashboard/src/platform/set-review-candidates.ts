import { createReadClient } from './client.ts'
import { parseProjectContent, type ProjectContent, type ProjectOpen } from './project-content.ts'
import { canonicalJSON, ReviewError, reviewID, validLocator, type ReviewLocator } from './set-review.ts'
import type { AppSDK } from './types.ts'

export interface ReviewCandidate { key: string; label: string; title?: string; locator: ReviewLocator }
export interface ReviewCandidateOptions { signal?: AbortSignal; cursor?: string }
export interface ReviewCandidatePage { items: ReviewCandidate[]; nextCursor: string; workTitle?: string }
type ProjectLocator = Extract<ReviewLocator, { kind: 'project' }>

/** Enumerate only mapped versions actually present in the requested immutable revision. */
export function projectReviewCandidates(project: ProjectOpen, content: ProjectContent | null, fixed: ProjectLocator): ReviewCandidate[] {
  if (project.project_id !== fixed.project_id || project.revision_id !== fixed.content_revision_id) throw new ReviewError('INVALID_RESPONSE')
  const items: ReviewCandidate[] = []
  for (const material of content?.ai_short_drama?.globals || []) {
    if (material.kind !== 'location') continue
    if (material.key === fixed.material_key && !fixed.state_key && !fixed.version_key) {
      const locator = { ...fixed }
      items.push({ key: canonicalJSON(locator), title: material.name, label: `${material.name} · 历史附件（版本待关联）`, locator })
    }
    for (const version of material.versions) {
      const state = material.states.find(item => item.key === version.state_key)
      if (!state) throw new ReviewError('INVALID_RESPONSE')
      const locator: ProjectLocator = { kind: 'project', project_id: fixed.project_id, content_revision_id: fixed.content_revision_id, material_key: material.key, state_key: state.key, version_key: version.key }
      if (!validLocator(locator)) throw new ReviewError('INVALID_RESPONSE')
      items.push({ key: canonicalJSON(locator), title: material.name, label: [material.name, state.name, version.label].join(' · '), locator })
    }
  }
  if (new Set(items.map(item => item.key)).size !== items.length) throw new ReviewError('INVALID_RESPONSE')
  return items
}

export async function loadReviewCandidates(sdk: AppSDK, locator: ReviewLocator, options: ReviewCandidateOptions = {}): Promise<ReviewCandidatePage> {
  if (locator.kind !== 'project' || !validLocator(locator) || options.cursor) throw new ReviewError('INVALID_RESPONSE')
  if (options.signal?.aborted) throw new DOMException('候选读取已取消。', 'AbortError')
  const fixed = { ...locator }, initial = canonicalJSON(sdk.context.get()), client = createReadClient(sdk, 'project')
  let invalidated = false
  const cancel = () => { invalidated = true; client.dispose() }
  const stopLife = sdk.lifecycle.subscribe(event => { if (event !== 'active') cancel() })
  const stopContext = sdk.context.subscribe(value => { if (canonicalJSON(value) !== initial) cancel() })
  options.signal?.addEventListener('abort', cancel, { once: true })
  const active = () => { if (invalidated || options.signal?.aborted || canonicalJSON(sdk.context.get()) !== initial) throw new DOMException('候选读取已取消。', 'AbortError') }
  try {
    const project = await client.call<ProjectOpen>('project.open', { project_id: fixed.project_id, revision_id: fixed.content_revision_id })
    active()
    if (project?.project_id !== fixed.project_id || project.revision_id !== fixed.content_revision_id || project.manifest?.category !== 'ai_short_drama' || typeof project.manifest.name !== 'string' || !Array.isArray(project.manifest.files) || !Array.isArray(project.manifest.domain_documents)) throw new ReviewError('INVALID_RESPONSE')
    const files = project.manifest.files
    if (new Set(files.map(file => file.file_id)).size !== files.length || files.some(file => !reviewID(file.file_id) || !file.content_ref || ![file.content_ref.store_id, file.content_ref.artifact_id, file.content_ref.version_id].every(reviewID))) throw new ReviewError('INVALID_RESPONSE')
    const documents = project.manifest.domain_documents.filter(item => item.document_type === 'project-content')
    if (documents.length > 1) throw new ReviewError('INVALID_RESPONSE')
    if (!documents.length) return { items: [], nextCursor: '', workTitle: project.manifest.name }
    const file = files.find(item => item.file_id === documents[0].file_id)
    if (!file) throw new ReviewError('INVALID_RESPONSE')
    const read = await client.call<{ mode: string; text: string; version: { store_id: string; artifact_id: string; version_id: string; size: number; sha256: string } }>('artifact.read', { mode: 'text', content_ref: file.content_ref, access: { project_id: fixed.project_id, project_revision_id: fixed.content_revision_id } })
    active()
    const version = read?.version
    if (read?.mode !== 'text' || typeof read.text !== 'string' || !version || version.store_id !== file.content_ref.store_id || version.artifact_id !== file.content_ref.artifact_id || version.version_id !== file.content_ref.version_id || !Number.isSafeInteger(version.size) || version.size < 0 || version.size > 4 * 1024 * 1024 || !/^[a-f0-9]{64}$/.test(version.sha256)) throw new ReviewError('INVALID_RESPONSE')
    const bytes = new TextEncoder().encode(read.text)
    if (bytes.byteLength !== version.size) throw new ReviewError('INVALID_RESPONSE')
    const digest = [...new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))].map(value => value.toString(16).padStart(2, '0')).join('')
    active()
    if (digest !== version.sha256) throw new ReviewError('INVALID_RESPONSE')
    const content = parseProjectContent(read.text, project)
    return { items: projectReviewCandidates(project, content, fixed), nextCursor: '', workTitle: project.manifest.name }
  } finally {
    options.signal?.removeEventListener('abort', cancel)
    stopLife(); stopContext(); client.dispose()
  }
}
