import test from 'node:test'
import assert from 'node:assert/strict'
import { createProjectPublication, normalizeRegistration, type PublicationReceiptPointer, type PublicationSnapshot } from '../src/platform/project-publication.ts'
import { PublicationError } from '../src/platform/publication-client.ts'
import { canonicalRoute } from '../src/platform/route-links.ts'
import type { AppSDK, Context, Json, Lifecycle } from '../src/platform/types.ts'
const id = (n: number) => `01900000-0000-7000-8000-${String(n).padStart(12, '0')}`
const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T
function fixture() {
  const candidate = { key: 'hall', name: 'Hall', source: { kind: 'global', key: 'hall' }, description: null, publication_record_file_id: id(10), review: { state: 'approved', record_file_id: id(11) }, registration_request: null, registration_status: null }
  const initial = { project: { project_id: id(1), revision_id: id(2), manifest: { name: 'Project', category: 'ai_short_drama', files: [], domain_documents: [] } }, content: { schema_version: 1, kind: 'project-content', category: 'ai_short_drama', overview: 'x'.repeat(2 * 1024 * 1024), cover_file_id: null, ai_short_drama: { script_file_id: null, aspect: null, globals: [], usages: [], deliverables: [], world_candidates: [candidate], continuity: [], episodes: [] } } } as unknown as PublicationSnapshot
  const snapshots = new Map([[id(2), clone(initial)]]), records = new Map<string, string>(), calls: { operation: string; body: any }[] = []
  let head = id(2), serial = 30, losePrepare = false, loseAssociation = false, conflict = false, prepared = 0, associated = 0, worldOffline = false, prepareConflict = false, prepareResponseState = 'conflict'
  const commits = new Map<string, { body: any; status: any }>()
  let request: any, receipt: any
  const lifecycle = new Set<(event: Lifecycle) => void>(), contexts = new Set<(value: Context) => void>()
  const context: Context = { userId: 'user', organizationId: 'org', stationId: 'station', appId: 'project', installationId: 'installed', contextRevision: 'grant1', projectId: null }
  const storage = { getItem: (key: string) => records.get(key) || null, setItem: (key: string, value: string) => { records.set(key, value) }, removeItem: (key: string) => { records.delete(key) } }
  const read = async (revision?: string) => clone(snapshots.get(revision || head)!)
  const sdk: AppSDK = {
    version: '1.1.0', context: { get: () => context, subscribe: fn => { contexts.add(fn); return () => contexts.delete(fn) } }, lifecycle: { subscribe: fn => { lifecycle.add(fn); return () => lifecycle.delete(fn) } }, theme: { get: () => 'light', subscribe: () => () => {} },
    navigation: { get: () => ({ path: '/', revision: 1 }), subscribe: () => () => {}, request: async () => {} }, capabilities: { call: async () => { throw Error('Unexpected capability') } },
    app: { request: async input => {
      const operation = input.path.split('/').at(-1)!, body = clone(input.body) as any; calls.push({ operation, body }); assert.ok(JSON.stringify(body).length < 2048, 'only narrow IDs cross the SDK')
      let data: any
      if (operation === 'publication_receipt') { if (worldOffline) throw Error('World offline'); assert.equal(body.commit_id, request.commit_id); assert.equal(body.asset_id, id(7)); assert.equal(body.asset_version_id, id(8)); data = receipt }
      else if (operation === 'publication_prepare' || operation === 'publication_associate') {
        assert.ok(records.size >= 1, 'persist intent before writes')
        const key = operation === 'publication_prepare' ? body.prepare_commit_id : body.association_commit_id
        if (commits.has(key)) { const saved = commits.get(key)!; assert.deepEqual(saved.body, body, 'retry drift'); data = saved.status }
        else if (operation === 'publication_prepare' && prepareConflict) { prepareConflict = false; data = { state: prepareResponseState, request_sha256: 'b'.repeat(64), current_revision_id: head }; commits.set(key, { body, status: data }) }
        else if (operation === 'publication_prepare') {
          prepared++; request = { commit_id: body.commit_id, source_project_id: id(1), source_revision_id: id(2), file_ids: [id(10)], name: 'Hall', asset_type: 'location', subjects: ['Hall'], relation: 'produced_in', review_file_id: id(11) }
          const next = clone(initial); head = id(3); next.project.revision_id = head; next.content.ai_short_drama!.world_candidates[0].registration_request = clone(request); snapshots.set(head, next)
          const ref = { store_id: id(20), artifact_id: id(21), version_id: id(22) }
          receipt = { state: 'committed', request_sha256: 'a'.repeat(64), result: { asset_id: id(7), asset_version_id: id(8), manifest_ref: ref, manifest: { schema_version: 1, kind: 'asset-version', asset_id: id(7), name: 'Hall', asset_type: 'location', subjects: ['Hall'], depends_on: [], source: { project_id: id(1), project_revision_id: id(2), relation: 'produced_in', review_file_id: id(11) }, files: [{ file_id: id(10), path: 'scene.json', role: 'result', content_ref: ref }] } } }
          data = { state: 'committed', request_sha256: 'b'.repeat(64), result: next.project }; commits.set(key, { body, status: data })
          if (losePrepare) { losePrepare = false; throw Error('lost prepare') }
        } else {
          if (conflict) { conflict = false; const next = clone(snapshots.get(head)!); head = id(++serial); next.project.revision_id = head; next.content.overview = 'Other editor'; snapshots.set(head, next); data = { state: 'conflict', request_sha256: 'c'.repeat(64), current_revision_id: head } }
          else { assert.equal(body.expected_revision_id, head); associated++; const next = clone(snapshots.get(head)!); head = id(++serial); next.project.revision_id = head; next.content.ai_short_drama!.world_candidates[0].registration_status = clone(receipt); snapshots.set(head, next); data = { state: 'committed', request_sha256: 'd'.repeat(64), result: next.project } }
          commits.set(key, { body, status: data }); if (loseAssociation) { loseAssociation = false; throw Error('lost association') }
        }
      } else throw Error('Unexpected route ' + operation)
      return { status: 200, data: clone(data) as Json, requestId: 'request' }
    } },
  }
  return { sdk, storage, records, calls, read, snapshots, head: () => head, counts: () => ({ prepared, associated }), losePrepare: () => { losePrepare = true }, loseAssociation: () => { loseAssociation = true }, conflict: () => { conflict = true },
    pointer: (): PublicationReceiptPointer => ({ project_id: id(1), prepared_revision_id: id(3), candidate_key: 'hall', commit_id: request.commit_id, asset_id: id(7), asset_version_id: id(8) }),
    changeCandidate: () => { snapshots.get(head)!.content.ai_short_drama!.world_candidates[0].registration_request!.commit_id = id(99) },
    badReceipt: () => { receipt.result.manifest.source.project_revision_id = id(99) },
    worldOffline: () => { worldOffline = true },
    prepareConflict: () => { prepareConflict = true },
    prepareState: (state: string) => { prepareConflict = true; prepareResponseState = state },
    currentPrepared: () => {
      head = id(9); const current = clone(initial); current.project.revision_id = head
      current.content.ai_short_drama!.world_candidates[0].registration_request = { commit_id: id(90), source_project_id: id(1), source_revision_id: id(2), file_ids: [id(10)], name: 'Hall', asset_type: 'location', subjects: ['Hall'], relation: 'produced_in', review_file_id: id(11) }
      snapshots.set(head, current)
    },
  }
}
test('large project preparation only sends fixed IDs and preserves original source revision', async () => {
  const f = fixture(), c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read)
  const path = await c.prepare(id(2)); assert.match(path, /^\/publish\?/); assert.equal(c.pending()?.prepared_revision_id, id(3)); assert.equal(c.pending()?.request?.source_revision_id, id(2)); assert.equal(f.calls.length, 1)
  assert.equal(canonicalRoute('project', '/publication?project=' + id(1)), '/publication?project=' + id(1))
  await c.associate(f.pointer()); assert.deepEqual(f.counts(), { prepared: 1, associated: 1 }); assert.ok(f.calls.every(c => !c.operation.startsWith('world'))); c.dispose()
})
test('unknown prepare outcome retries same IDs after reconstruction without deriving from head', async () => {
  const f = fixture(); let c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read); f.losePrepare()
  await assert.rejects(c.prepare(id(2)), /lost prepare/); const original = c.pending(); c.dispose()
  c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read); await c.prepare(id(3))
  assert.equal(c.pending()?.commit_id, original?.commit_id); assert.deepEqual(f.calls[0].body, f.calls[1].body); assert.equal(f.counts().prepared, 1); c.dispose()
})
test('lost association response replays the original narrow request and keeps successful World separate', async () => {
  const f = fixture(); let c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read); await c.prepare(id(2)); f.loseAssociation()
  await assert.rejects(c.associate(f.pointer()), /lost association/); c.dispose()
  c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read); f.worldOffline(); await c.associate(f.pointer())
  const writes = f.calls.filter(c => c.operation === 'publication_associate'); assert.equal(writes.length, 2); assert.deepEqual(writes[0].body, writes[1].body); assert.equal(f.counts().associated, 1); c.dispose()
})
test('known association conflict alone advances its CAS ID and preserves unrelated editor content', async () => {
  const f = fixture(), c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read); await c.prepare(id(2)); f.conflict()
  await assert.rejects(c.associate(f.pointer()), e => e instanceof PublicationError && e.code === 'REVISION_CONFLICT')
  const previous = c.pending(); await c.associate(f.pointer()); assert.notEqual(c.pending()?.association?.association_commit_id, previous?.association?.association_commit_id)
  assert.equal(c.pending()?.commit_id, previous?.commit_id); assert.equal((await f.read()).content.overview, 'Other editor'); c.dispose()
})
test('changed candidate or forged return identifiers cannot write a project association', async () => {
  const f = fixture(), c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read); await c.prepare(id(2))
  await assert.rejects(c.associate({ ...f.pointer(), commit_id: id(99) }), e => e instanceof PublicationError && e.code === 'REQUEST_MISMATCH')
  f.badReceipt(); await assert.rejects(c.associate(f.pointer()), e => e instanceof PublicationError && e.code === 'REQUEST_MISMATCH'); assert.equal(f.counts().associated, 0); c.dispose()
})
test('storage failure prevents any preparation write', async () => {
  const f = fixture(), c = createProjectPublication(f.sdk, { ...f.storage, setItem: () => { throw Error('quota') } }, id(1), 'hall', f.read)
  await assert.rejects(c.prepare(id(2)), e => e instanceof PublicationError && e.code === 'STORAGE_UNAVAILABLE'); assert.equal(f.calls.length, 0); c.dispose()
})
test('a persisted conflict is reconfirmed before a reconstructed client creates a new CAS ID', async () => {
  const f = fixture(); let c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read); await c.prepare(id(2)); f.conflict()
  await assert.rejects(c.associate(f.pointer())); const original = c.pending()!.association!.association_commit_id; c.dispose()
  c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read)
  await assert.rejects(c.associate(f.pointer()), e => e instanceof PublicationError && e.code === 'REVISION_CONFLICT')
  assert.equal(c.pending()!.association!.association_commit_id, original)
  await c.associate(f.pointer()); assert.notEqual(c.pending()!.association!.association_commit_id, original); c.dispose()
})
test('damaged recovery state or hashes cannot enter the association flow', async () => {
  const f = fixture(), c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read); await c.prepare(id(2)); f.loseAssociation(); await assert.rejects(c.associate(f.pointer())); c.dispose()
  const [key, raw] = [...f.records][0]
  for (const change of [(r: any) => { r.association.state = 'invented' }, (r: any) => { r.receipt_hash = 'invalid' }, (r: any) => { delete r.published_version_id }]) {
    const record = JSON.parse(raw); change(record); f.records.set(key, JSON.stringify(record))
    assert.throws(() => createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read), e => e instanceof PublicationError && e.code === 'INVALID_RESPONSE')
  }
})
test('explicit restart reconfirms preparation failure and archives it before a new intent', async () => {
  const f = fixture(); let c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read); f.prepareConflict()
  await assert.rejects(c.prepare(id(2)), e => e instanceof PublicationError && e.code === 'TERMINAL_FAILURE')
  const old = c.pending(); c.dispose(); c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read)
  assert.equal(await c.restartPreparation(), id(2)); assert.equal(c.pending(), undefined)
  assert.deepEqual(f.calls[0].body, f.calls[1].body); assert.equal([...f.records.keys()].filter(key => key.endsWith(':terminal:' + old!.commit_id)).length, 1)
  await c.prepare(id(2)); assert.notEqual(c.pending()?.commit_id, old?.commit_id); assert.equal(f.counts().prepared, 1); c.dispose()
})
test('a lost successful preparation cannot be reset into a new World operation', async () => {
  const f = fixture(), c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read); f.losePrepare()
  await assert.rejects(c.prepare(id(2))); const old = c.pending()
  await assert.rejects(c.restartPreparation(), e => e instanceof PublicationError && e.code === 'NOT_READY')
  assert.equal(c.pending()?.commit_id, old?.commit_id); assert.equal(c.pending()?.prepared_revision_id, id(3)); assert.equal(f.records.size, 1); c.dispose()
})
test('restart reuses an already prepared current head including its original World ID and R0', async () => {
  const f = fixture(), c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read); f.prepareConflict()
  await assert.rejects(c.prepare(id(2))); f.currentPrepared()
  const revision = await c.restartPreparation(); assert.equal(revision, id(9)); const before = f.calls.length
  await c.prepare(revision); assert.equal(f.calls.length, before, 'existing request needs no new preparation write')
  assert.equal(c.pending()?.commit_id, id(90)); assert.equal(c.pending()?.prepared_revision_id, id(9)); assert.equal(c.pending()?.request?.source_revision_id, id(2)); c.dispose()
})
test('confirmed failed preparation may restart while preparing state retains the original intent', async () => {
  for (const state of ['failed', 'preparing']) {
    const f = fixture(), c = createProjectPublication(f.sdk, f.storage, id(1), 'hall', f.read); f.prepareState(state)
    await assert.rejects(c.prepare(id(2))); const original = c.pending()
    if (state === 'failed') { await c.restartPreparation(); assert.equal(c.pending(), undefined) }
    else { await assert.rejects(c.restartPreparation(), e => e instanceof PublicationError && e.code === 'NOT_READY'); assert.equal(c.pending()?.commit_id, original?.commit_id); assert.equal(f.records.size, 1) }
    c.dispose()
  }
})
test('absent depends_on and different field ordering compare as one registration', () => {
  const request = { commit_id: id(4), source_project_id: id(1), source_revision_id: id(2), file_ids: [id(11), id(10)], name: 'Hall', asset_type: 'location', subjects: ['B', 'A'], relation: 'produced_in' as const }
  assert.deepEqual(normalizeRegistration(request), normalizeRegistration({ ...request, file_ids: [id(10), id(11)], subjects: ['A', 'B'], depends_on: [] }))
})
