import test from 'node:test'
import assert from 'node:assert/strict'
import { createAssetImport, requestHash, type ImportTarget, type ImportRequest, type RecoveryStorage } from '../src/platform/asset-import.ts'
import type { AppSDK, Context, Json, Lifecycle } from '../src/platform/types.ts'

const id = (n: number) => '01900000-0000-7000-8000-00000000000' + n
const target: ImportTarget = { project_id: id(1), revision_id: id(2), name: 'Project', asset_refs: [] }
function memory(): RecoveryStorage {
  const map = new Map<string, string>()
  return { getItem: key => map.get(key) || null, setItem: (key, value) => { map.set(key, value) }, removeItem: key => { map.delete(key) } }
}
function fixture(storage = memory()) {
  let context: Context = { userId: 'user-a', organizationId: 'org-a', stationId: 'station-a', projectId: null, appId: 'project', installationId: 'install-a', contextRevision: 'scope-a' }
  const lifecycle = new Set<(v: Lifecycle) => void>(), contexts = new Set<(v: Context) => void>()
  const requests: { path: string; body: Json; signal?: AbortSignal }[] = []
  let handler: (path: string, body: Json) => Promise<{ status: number; data: Json }> = async () => ({ status: 503, data: { code: 'DEPENDENCY_UNAVAILABLE' } })
  const sdk: AppSDK = {
    version: '1.1.0', context: { get: () => context, subscribe: fn => { contexts.add(fn); return () => contexts.delete(fn) } },
    lifecycle: { subscribe: fn => { lifecycle.add(fn); return () => lifecycle.delete(fn) } }, theme: { get: () => 'light', subscribe: () => () => {} },
    navigation: { get: () => ({ path: '/', revision: 1 }), subscribe: () => () => {}, request: async () => {} }, capabilities: { call: async () => { throw new Error('Other app capability must not be reached') } },
    app: { request: async (input, options) => { requests.push({ path: input.path, body: input.body, signal: options?.signal }); return { ...await handler(input.path, input.body), requestId: 'test' } } },
  }
  return { sdk, storage, requests, handler: (next: typeof handler) => { handler = next }, change: (update: Partial<Context>) => { context = { ...context, ...update }; for (const fn of contexts) fn(context) }, emit: (v: Lifecycle) => { for (const fn of lifecycle) fn(v) } }
}
const result = (request: ImportRequest): Json => ({ project_id: request.project_id, revision_id: id(9), manifest: { project_id: request.project_id, asset_refs: [{ asset_id: request.asset_id, asset_version_id: request.asset_version_id, purpose: request.purpose }] } })

test('the request fingerprint matches the Go use_asset contract and preserves reference replacement semantics', async () => {
  const request = { project_id: id(1), expected_revision_id: id(2), commit_id: id(3), asset_id: id(4), asset_version_id: id(5), purpose: '配乐<&\u2028' }
  const prior = [{ asset_id: id(6), asset_version_id: id(7), purpose: '旁白' }, { asset_id: id(4), asset_version_id: id(8), purpose: request.purpose }]
  assert.equal(await requestHash(request, prior), '2f959ca57040652db2ecb009cd1dbd3a0655ff98af07702c45c0f4e34114cebe')
})
test('target discovery requests write permission and pins the actually opened head', async () => {
  const f = fixture(), client = createAssetImport(f.sdk, f.storage, id(4), id(5))
  f.handler(async path => ({ status: 200, data: path.endsWith('/list') ? { items: [{ project_id: id(1), head_revision_id: id(2), name: 'Project', category: 'video' }], next_cursor: '' } : { project_id: id(1), revision_id: id(3), head_revision_id: id(3), manifest: { project_id: id(1), name: 'Project', asset_refs: [] } } }))
  await client.list(); const opened = await client.open(id(1))
  assert.equal(opened.revision_id, id(3))
  assert.deepEqual(f.requests.map(r => (r.body as Record<string, Json>).require_write), [true, true])
  f.handler(async () => ({ status: 403, data: { code: 'PERMISSION_DENIED' } }))
  await assert.rejects(client.open(id(1)), { code: 'PERMISSION_DENIED' })
  client.dispose()
})
test('lost response is recovered with the identical durable request across the same user login epoch', async () => {
  const f = fixture(), client = createAssetImport(f.sdk, f.storage, id(4), id(5))
  const journal = await client.begin(target, '背景音乐')
  assert.equal(f.requests.length, 0)
  assert.equal(journal.request.commit_id[14], '7')
  await assert.rejects(client.submit(), { code: 'DEPENDENCY_UNAVAILABLE' })
  await assert.rejects(client.begin(target, 'another'), { code: 'PENDING_IMPORT' })
  const first = f.requests[0].body
  client.dispose(); f.change({ contextRevision: 'new-login' })
  const recovered = createAssetImport(f.sdk, f.storage, id(4), id(5))
  assert.deepEqual(recovered.pending(), journal)
  f.handler(async () => ({ status: 200, data: result(journal.request) }))
  assert.equal((await recovered.submit()).revision_id, id(9))
  assert.deepEqual(f.requests[1].body, first)
  recovered.clearTerminal(); assert.equal(recovered.pending(), undefined); recovered.dispose()
})
test('different actors, installations and asset versions cannot consume a stored pending import', async () => {
  const f = fixture(), client = createAssetImport(f.sdk, f.storage, id(4), id(5))
  await client.begin(target, 'reference'); client.dispose()
  for (const [asset, version] of [[id(6), id(5)], [id(4), id(6)]]) { const other = createAssetImport(f.sdk, f.storage, asset, version); assert.equal(other.pending(), undefined); other.dispose() }
  for (const update of [{ userId: 'other-user' }, { userId: 'user-a', installationId: 'other-install' }, { installationId: 'install-a', organizationId: 'other-org' }]) {
    f.change(update); const other = createAssetImport(f.sdk, f.storage, id(4), id(5)); assert.equal(other.pending(), undefined); other.dispose()
  }
})
test('a terminal status must match the exact request hash before a new confirmation is allowed', async () => {
  const f = fixture(), client = createAssetImport(f.sdk, f.storage, id(4), id(5))
  const journal = await client.begin(target, 'reference')
  f.handler(async () => ({ status: 200, data: { state: 'conflict', request_sha256: 'f'.repeat(64) } }))
  await assert.rejects(client.status(), { code: 'REQUEST_MISMATCH' })
  assert.throws(() => client.clearTerminal(), { code: 'PENDING_IMPORT' })
  f.handler(async () => ({ status: 404, data: { code: 'NOT_FOUND' } }))
  await assert.rejects(client.status(), { code: 'NOT_FOUND' })
  assert.deepEqual(client.pending(), journal)
  f.handler(async () => ({ status: 200, data: { state: 'conflict', request_sha256: journal.requestHash } }))
  assert.equal((await client.status()).state, 'conflict'); client.clearTerminal()
  const next = await client.begin({ ...target, revision_id: id(8) }, 'reference')
  assert.notEqual(next.request.commit_id, journal.request.commit_id)
  assert.equal(next.request.expected_revision_id, id(8)); client.dispose()
})
test('storage failure prevents dispatch, malformed results preserve the pending journal', async () => {
  const f = fixture(), broken = { ...f.storage, setItem: () => { throw new Error('storage full') } }
  const blocked = createAssetImport(f.sdk, broken, id(4), id(5))
  await assert.rejects(blocked.begin(target, 'reference'), { code: 'STORAGE_UNAVAILABLE' })
  assert.equal(f.requests.length, 0); blocked.dispose()
  const client = createAssetImport(f.sdk, f.storage, id(4), id(5)); const journal = await client.begin(target, 'reference')
  f.handler(async () => ({ status: 200, data: { project_id: id(1), revision_id: id(9), manifest: { project_id: id(1), asset_refs: [] } } }))
  await assert.rejects(client.submit(), { code: 'INVALID_RESPONSE' })
  assert.deepEqual(client.pending(), journal); client.dispose()
})
test('hide preserves an accepted write; account replacement aborts it and rejects its late response', async () => {
  const f = fixture(), client = createAssetImport(f.sdk, f.storage, id(4), id(5)), journal = await client.begin(target, 'reference')
  let resolve!: (value: { status: number; data: Json }) => void
  f.handler(() => new Promise(done => { resolve = done }))
  const request = client.submit(); f.emit('hidden')
  assert.equal(f.requests[0].signal?.aborted, false)
  f.change({ userId: 'other' }); assert.equal(f.requests[0].signal?.aborted, true)
  resolve({ status: 200, data: result(journal.request) })
  await assert.rejects(request, { code: 'SESSION_REVOKED' })
  f.change({ userId: 'user-a', contextRevision: 'new-login' })
  const recovery = createAssetImport(f.sdk, f.storage, id(4), id(5)); assert.deepEqual(recovery.pending(), journal); recovery.dispose()
})
