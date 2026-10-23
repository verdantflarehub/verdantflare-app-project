import test from 'node:test'
import assert from 'node:assert/strict'
import { loadReviewCandidates } from '../src/platform/set-review-candidates.ts'
import type { ReviewLocator } from '../src/platform/set-review.ts'
import type { AppSDK, Context, Json, Lifecycle } from '../src/platform/types.ts'

const id = (n: number) => `01900000-0000-7000-8000-${String(n).padStart(12, '0')}`
const fixed: ReviewLocator = { kind: 'project', project_id: id(1), content_revision_id: id(2), material_key: 'courtyard', state_key: 'day', version_key: 'v1' }
const ref = { store_id: id(3), artifact_id: id(4), version_id: id(5) }
function fixture() {
  const lifecycle = new Set<(event: Lifecycle) => void>(), contexts = new Set<(value: Context) => void>(), requests: { path: string; body: Json; signal?: AbortSignal }[] = [], reads: Json[] = []
  let context: Context = { userId: 'reader', organizationId: 'org', stationId: 'station', appId: 'project', installationId: 'install', contextRevision: 'grant-1', projectId: null }
  const version = { key: 'v1', label: '历史图像版', basis_revision_id: id(2), parent_version_key: null, change_note: '固定材料', state_key: 'day', file_refs: [{ file_id: id(7), content_ref: ref, purpose: 'output' }], generation_record_file_ids: [], review: { state: 'unreviewed', record_file_id: null } }
  const body = { schema_version: 1, kind: 'project-content', category: 'ai_short_drama', ai_short_drama: { episodes: [], globals: [
    { key: 'courtyard', kind: 'location', name: '县署', states: [{ key: 'day', name: '日景', definition_refs: [], marks: [] }], versions: [version] },
    { key: 'legacy', kind: 'location', name: '旧附件', states: [], versions: [], file_ids: [id(7)] },
    { key: 'prop', kind: 'prop', name: '道具', states: [], versions: [] },
  ] } }
  const project = { project_id: id(1), revision_id: id(2), head_revision_id: id(99), manifest: { name: '固定作品', category: 'ai_short_drama', files: [{ file_id: id(6), path: 'project-content.json', content_ref: ref }, { file_id: id(7), path: 'render.png', content_ref: ref }], domain_documents: [{ document_type: 'project-content', file_id: id(6) }] } }
  let handle: () => Promise<{ status: number; data: Json; requestId: string }> = async () => ({ status: 200, data: project as unknown as Json, requestId: 'test' })
  let tamper = false
  const sdk: AppSDK = { version: '1.1.0', context: { get: () => context, subscribe: fn => { contexts.add(fn); return () => contexts.delete(fn) } }, lifecycle: { subscribe: fn => { lifecycle.add(fn); return () => lifecycle.delete(fn) } }, theme: { get: () => 'light', subscribe: () => () => {} }, navigation: { get: () => ({ path: '/', revision: 1 }), subscribe: () => () => {}, request: async () => {} }, app: { request: async (input, options) => { requests.push({ ...input, signal: options?.signal }); return handle() } }, capabilities: { call: async (name, args) => {
    assert.equal(name, 'artifact.read'); reads.push(args)
    const text = JSON.stringify(body), bytes = new TextEncoder().encode(text)
    const digest = [...new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))].map(n => n.toString(16).padStart(2, '0')).join('')
    return { mode: 'text', text, version: { ...ref, size: bytes.length, sha256: tamper ? '0'.repeat(64) : digest } }
  } } }
  return { sdk, project, body, requests, reads, lifecycle, contexts, tamper: () => { tamper = true }, handle: (next: typeof handle) => { handle = next }, emit: (event: Lifecycle) => { for (const fn of lifecycle) fn(event) }, change: () => { context = { ...context, contextRevision: 'grant-2' }; for (const fn of contexts) fn(context) } }
}

test('candidate selection reads the requested revision and maps actual location/state/version labels', async () => {
  const f = fixture(), result = await loadReviewCandidates(f.sdk, fixed)
  assert.equal(result.items.length, 1); assert.equal(result.items[0].title, '县署'); assert.equal(result.items[0].label, '县署 · 日景 · 历史图像版')
  assert.deepEqual(result.items[0].locator, fixed); assert.equal(result.nextCursor, '')
  assert.deepEqual(f.requests[0].body, { project_id: id(1), revision_id: id(2) })
  assert.deepEqual((f.reads[0] as Record<string, Json>).access, { project_id: id(1), project_revision_id: id(2) })
  assert.equal(f.contexts.size, 0); assert.equal(f.lifecycle.size, 0)
})

test('a service returning head or foreign revision is rejected without reading its content', async () => {
  const f = fixture(); f.project.revision_id = id(99)
  await assert.rejects(loadReviewCandidates(f.sdk, fixed), /INVALID_RESPONSE/)
  assert.equal(f.reads.length, 0); assert.equal(f.requests.length, 1); assert.equal(f.lifecycle.size, 0)
})

test('unverified document bytes cannot become selectable candidates', async () => {
  const f = fixture(); f.tamper()
  await assert.rejects(loadReviewCandidates(f.sdk, fixed), /INVALID_RESPONSE/)
  assert.equal(f.contexts.size, 0); assert.equal(f.lifecycle.size, 0)
})

test('missing project business document remains a real empty list and does not fabricate versions', async () => {
  const f = fixture(); f.project.manifest.domain_documents = []
  assert.deepEqual(await loadReviewCandidates(f.sdk, fixed), { items: [], nextCursor: '' }); assert.equal(f.reads.length, 0)
})

test('the current legacy location keeps its real title without offering other unmapped locations for comparison', async () => {
  const f = fixture(), legacy: ReviewLocator = { ...fixed, material_key: 'legacy', state_key: '', version_key: '' }
  f.body.ai_short_drama.globals.push({ key: 'other_legacy', kind: 'location', name: '另一旧地点', states: [], versions: [] })
  const result = await loadReviewCandidates(f.sdk, legacy)
  const current = result.items.find(item => item.locator.kind === 'project' && item.locator.material_key === 'legacy')!
  assert.deepEqual(current.locator, legacy)
  assert.equal(current.title, '旧附件'); assert.equal(current.label, '旧附件 · 历史附件（版本待关联）')
  assert.equal(result.items.length, 2)
  assert.equal(result.items.some(item => item.locator.kind === 'project' && item.locator.material_key === 'other_legacy'), false)
})

for (const trigger of ['abort', 'hidden', 'context'] as const) test(`${trigger} cancels a pending read, rejects a late result and releases subscriptions`, async () => {
  const f = fixture(), controller = new AbortController()
  let resolve!: (value: { status: number; data: Json; requestId: string }) => void
  f.handle(() => new Promise(done => { resolve = done }))
  const pending = loadReviewCandidates(f.sdk, fixed, { signal: controller.signal })
  if (trigger === 'abort') controller.abort(); else if (trigger === 'hidden') f.emit('hidden'); else f.change()
  assert.equal(f.requests[0].signal?.aborted, true)
  resolve({ status: 200, data: f.project as unknown as Json, requestId: 'late' })
  await assert.rejects(pending, { name: 'AbortError' })
  assert.equal(f.reads.length, 0); assert.equal(f.contexts.size, 0); assert.equal(f.lifecycle.size, 0)
})
