import test from 'node:test'
import assert from 'node:assert/strict'
import { createBodyLimit, createCategories, CreateError, createProjectCreator, createRequestBytes, createRequestHash, type CreateInput, type CreateRequest } from '../src/platform/project-create.ts'
import { canonicalRoute, legacyStudioTarget } from '../src/platform/route-links.ts'
import type { AppSDK, Context, Json, Lifecycle } from '../src/platform/types.ts'

const id = (n: number) => `01900000-0000-7000-8000-${String(n).padStart(12, '0')}`
const input: CreateInput = { name: '新建项目', category: 'music', entry_path: 'review.md', entry_text: '# 初稿\n' }
const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T
const code = (expected: string) => (error: unknown) => error instanceof CreateError && error.code === expected
function fixture() {
  const records = new Map<string, string>(), creates = new Map<string, { request: CreateRequest; result: any; hash: string }>()
  const calls: { path: string; body: any; signal?: AbortSignal }[] = []
  const lifecycle = new Set<(event: Lifecycle) => void>(), contexts = new Set<(value: Readonly<Context>) => void>()
  const context: Context = { userId: 'user', organizationId: 'org', stationId: 'station', appId: 'project', installationId: 'install', contextRevision: 'grant-1', projectId: null }
  let lose = false, fail: { status: number; code: string } | undefined, holding: Promise<void> | undefined, release: (() => void) | undefined
  let responseEdit: (value: any) => any = value => value, statusEdit: (value: any) => any = value => value
  const storage = { getItem: (key: string) => records.get(key) ?? null, setItem: (key: string, value: string) => { records.set(key, value) }, removeItem: (key: string) => { records.delete(key) } }
  const sdk: AppSDK = {
    version: '1.1.0', context: { get: () => context, subscribe: fn => { contexts.add(fn); return () => contexts.delete(fn) } }, lifecycle: { subscribe: fn => { lifecycle.add(fn); return () => lifecycle.delete(fn) } }, theme: { get: () => 'light', subscribe: () => () => {} },
    navigation: { get: () => ({ path: '/create', revision: 1 }), subscribe: () => () => {}, request: async () => {} }, capabilities: { call: async () => { throw Error('Unexpected capability') } },
    app: { request: async (request, options) => {
      const body = clone(request.body) as any; calls.push({ path: request.path, body, signal: options?.signal })
      if (holding) await holding
      if (fail) return { status: fail.status, data: { code: fail.code }, requestId: 'request' }
      let data: any
      if (request.path === '/api/v1/project/create') {
        const journal = [...records.values()].map(raw => JSON.parse(raw)).find(record => record.request?.commit_id === body.commit_id)
        assert.deepEqual(journal?.request, body, 'persist exact original body before sending')
        const existing = creates.get(body.commit_id)
        if (existing) { assert.deepEqual(existing.request, body); data = existing.result }
        else {
          const ref = { store_id: id(4), artifact_id: id(5), version_id: id(6) }
          data = { project_id: id(1), revision_id: id(2), manifest_ref: ref, manifest: { schema_version: 1, kind: 'project', project_id: id(1), name: body.name, category: body.category, status: 'draft', entry_document_id: id(3), files: [{ file_id: id(3), path: body.entry_path, role: 'review', content_ref: ref }], selections: [], asset_refs: [], run_refs: [], domain_documents: [] }, invalidated_selections: [] }
          creates.set(body.commit_id, { request: body, result: clone(data), hash: await createRequestHash(body) })
        }
        if (lose) { lose = false; throw Error('Lost response') }
        data = responseEdit(clone(data))
      } else {
        assert.equal(request.path, '/api/v1/project/commit_status')
        const original = creates.get(body.commit_id)!; assert.equal(body.project_id, original.result.project_id)
        data = statusEdit({ state: 'committed', request_sha256: original.hash, result: clone(original.result) })
      }
      return { status: 200, data: clone(data) as Json, requestId: 'request' }
    } },
  }
  return { sdk, storage, records, calls, creates, context,
    lose: () => { lose = true }, fail: (status: number, code: string) => { fail = { status, code } },
    hold: () => { holding = new Promise<void>(resolve => { release = resolve }) }, release: () => { release?.(); holding = undefined },
    event: (value: Lifecycle) => lifecycle.forEach(fn => fn(value)), change: (change: Partial<Context>) => { Object.assign(context, change); contexts.forEach(fn => fn(context)) },
    responseEdit: (fn: typeof responseEdit) => { responseEdit = fn }, statusEdit: (fn: typeof statusEdit) => { statusEdit = fn },
  }
}
const untilCalled = async (f: ReturnType<typeof fixture>) => { while (!f.calls.length) await new Promise(resolve => setTimeout(resolve, 0)) }

test('Create digest matches the Go contract including HTML, Chinese and line separators', async () => {
  const request = { commit_id: '0199b501-0000-7000-8000-000000000004', name: '新建 <&>', category: 'ai_short_drama', entry_path: 'docs/review.md', entry_text: '# 初稿\n内容 <tag>&\u2028next\u2029' }
  assert.equal(await createRequestHash(request), '788489772901d511a56df66cbc4c46f8034ce17212d027640604ee43f4952303')
})
test('all six original categories persist exact input before creating and only terminal state opens a new intent', async () => {
  assert.deepEqual(createCategories.map(c => c.key), ['character', 'music', 'video', 'ai_short_drama', 'creative_video', 'image', 'music-mv'])
  for (const category of createCategories) {
    const f = fixture(), c = createProjectCreator(f.sdk, f.storage)
    await c.begin({ ...input, category: category.key, name: ' 项目 ', entry_path: ' docs/review.txt ', entry_text: '  保留原文\n' })
    assert.equal(f.calls.length, 0); const original = c.pending()!
    assert.equal(original.request.name, '项目'); assert.equal(original.request.entry_path, 'docs/review.txt'); assert.equal(original.request.entry_text, '  保留原文\n')
    await assert.rejects(c.clearTerminal(), code('PENDING_CREATE')); await assert.rejects(c.begin(input), code('PENDING_CREATE'))
    assert.deepEqual(await c.submit(), { state: 'committed', result: { project_id: id(1), revision_id: id(2) } })
    await c.clearTerminal(); assert.equal(c.pending(), undefined); assert.equal(f.records.size, 1)
    assert.ok([...f.records.keys()][0].endsWith(':terminal:' + original.request.commit_id))
    await c.begin(input); assert.notEqual(c.pending()!.request.commit_id, original.request.commit_id); c.dispose()
  }
})
test('lost creation response reconstructs and replays identical ID and full body without duplicating the project', async () => {
  const f = fixture(); let c = createProjectCreator(f.sdk, f.storage); await c.begin(input); f.lose()
  await assert.rejects(c.submit(), /Lost response/); const original = c.pending(); c.dispose()
  f.change({ contextRevision: 'grant-2' }); c = createProjectCreator(f.sdk, f.storage)
  assert.deepEqual(c.pending(), original); assert.equal((await c.submit()).state, 'committed')
  assert.deepEqual(f.calls[0].body, f.calls[1].body); assert.equal(f.creates.size, 1); c.dispose()
})
test('storage quota and failed read-back prevent sending; failure after success keeps recoverable original intent', async () => {
  for (const fault of ['throw', 'silent']) {
    const f = fixture(), c = createProjectCreator(f.sdk, { ...f.storage, setItem: () => { if (fault === 'throw') throw Error('quota') } })
    await assert.rejects(c.begin(input), code('STORAGE_UNAVAILABLE')); assert.equal(f.calls.length, 0); c.dispose()
  }
  const f = fixture(); let writes = 0
  let c = createProjectCreator(f.sdk, { ...f.storage, setItem: (key, raw) => { if (++writes === 2) throw Error('quota'); f.storage.setItem(key, raw) } })
  await c.begin(input); const original = c.pending(); await assert.rejects(c.submit(), code('STORAGE_UNAVAILABLE'))
  assert.equal(c.state(), 'unknown'); assert.deepEqual(c.pending(), original); c.dispose()
  c = createProjectCreator(f.sdk, f.storage); await c.submit(); assert.deepEqual(f.calls[0].body, f.calls[1].body); assert.equal(f.creates.size, 1); c.dispose()
})
test('complete escaped UTF-8 body permits the limit and rejects one byte over before persistence', async () => {
  const text = '中😀"\\\n'.repeat(30), base = { commit_id: id(1), ...input, entry_text: text }
  const exactText = text + 'x'.repeat(createBodyLimit - createRequestBytes(base))
  assert.equal(createRequestBytes({ ...base, entry_text: exactText }), createBodyLimit)
  const f = fixture(), c = createProjectCreator(f.sdk, f.storage); await c.begin({ ...input, entry_text: exactText }); assert.equal(createRequestBytes(c.pending()!.request), createBodyLimit); c.dispose()
  const over = fixture(), client = createProjectCreator(over.sdk, over.storage)
  await assert.rejects(client.begin({ ...input, entry_text: exactText + 'x' }), code('BODY_TOO_LARGE')); assert.equal(over.records.size, 0); assert.equal(over.calls.length, 0); client.dispose()
})
test('invalid names, Unicode, categories and portable paths never persist an unrecoverable request', async () => {
  const invalid: [Partial<CreateInput>, string][] = [
    [{ name: ' \u0085\u2003 ' }, 'INVALID_NAME'], [{ name: '😀'.repeat(257) }, 'INVALID_NAME'], [{ name: '\ud800' }, 'INVALID_NAME'], [{ category: 'custom' }, 'INVALID_CATEGORY'], [{ entry_text: '\udfff' }, 'INVALID_TEXT'],
    ...['../review.md', '/review.md', 'a\\review.md', '.VF/review.md', 'CON.md', 'com¹.txt', 'lpt².txt', 'a./review.md', 'a /review.md', 'e\u0301.md', 'review.json', 'a//review.md', 'x\u0085.md', 'x\ud800.md'].map(entry_path => [{ entry_path }, 'INVALID_PATH'] as [Partial<CreateInput>, string]),
  ]
  for (const [change, error] of invalid) {
    const f = fixture(), c = createProjectCreator(f.sdk, f.storage); await assert.rejects(c.begin({ ...input, ...change }), code(error)); assert.equal(f.records.size, 0); c.dispose()
  }
  const f = fixture(), c = createProjectCreator(f.sdk, f.storage); await c.begin({ ...input, name: '😀'.repeat(256), entry_path: '文档/😀.md' }); assert.ok(c.pending()); c.dispose()
})
test('HTTP failures never authorize discarding the original request or obtaining another ID', async () => {
  for (const [status, error] of [[400, 'INVALID_ARGUMENT'], [403, 'PERMISSION_DENIED'], [404, 'NOT_FOUND'], [409, 'IDEMPOTENCY_CONFLICT'], [409, 'CONTENT_NOT_READY'], [503, 'DEPENDENCY_UNAVAILABLE']] as const) {
    const f = fixture(), c = createProjectCreator(f.sdk, f.storage); await c.begin(input); const original = c.pending(); f.fail(status, error)
    await assert.rejects(c.submit(), code(error)); assert.equal(c.state(), 'unknown'); assert.deepEqual(c.pending(), original)
    await assert.rejects(c.clearTerminal(), code('PENDING_CREATE')); await assert.rejects(c.begin(input), code('PENDING_CREATE')); assert.deepEqual(c.pending(), original); c.dispose()
  }
})
test('known result recovery reads the original commit status and verifies its hash before terminal clearance', async () => {
  const f = fixture(); let c = createProjectCreator(f.sdk, f.storage); await c.begin(input); await c.submit(); c.dispose()
  c = createProjectCreator(f.sdk, f.storage); assert.equal(c.state(), 'unknown'); await assert.rejects(c.clearTerminal(), code('PENDING_CREATE'))
  f.statusEdit(value => ({ ...value, request_sha256: '0'.repeat(64) })); await assert.rejects(c.submit(), code('REQUEST_MISMATCH')); await assert.rejects(c.clearTerminal(), code('PENDING_CREATE'))
  f.statusEdit(value => value); await c.submit(); assert.equal(c.state(), 'committed'); assert.equal(f.calls.filter(call => call.path.endsWith('/create')).length, 1)
  await c.clearTerminal(); c.dispose()
})
test('a verified failed or conflicted status is terminal; preparing retains the original request', async () => {
  for (const state of ['failed', 'conflict', 'preparing']) {
    const f = fixture(); let c = createProjectCreator(f.sdk, f.storage); await c.begin(input); await c.submit(); c.dispose(); c = createProjectCreator(f.sdk, f.storage)
    f.statusEdit(value => ({ ...value, state, result: undefined })); assert.equal((await c.status()).state, state)
    if (state === 'preparing') await assert.rejects(c.clearTerminal(), code('PENDING_CREATE'))
    else { await c.clearTerminal(); assert.equal(c.pending(), undefined) }
    c.dispose()
  }
})
test('corrupt hashes and malformed or mismatching results do not reach terminal state', async () => {
  const f = fixture(); let c = createProjectCreator(f.sdk, f.storage); await c.begin(input); c.dispose()
  const [key, raw] = [...f.records][0], record = JSON.parse(raw); record.requestHash = '0'.repeat(64); f.records.set(key, JSON.stringify(record))
  c = createProjectCreator(f.sdk, f.storage); await assert.rejects(c.submit(), code('REQUEST_MISMATCH')); assert.equal(f.calls.length, 0); c.dispose()
  f.records.set(key, raw); c = createProjectCreator(f.sdk, f.storage); f.responseEdit(value => { value.manifest.name = 'Other'; return value })
  await assert.rejects(c.submit(), code('INVALID_RESPONSE')); assert.equal(c.state(), 'unknown'); await assert.rejects(c.clearTerminal(), code('PENDING_CREATE')); c.dispose()
})
test('journal recovery is isolated by subject, organization, Station and installation', async () => {
  for (const field of ['userId', 'organizationId', 'stationId', 'installationId'] as const) {
    const f = fixture(), original = createProjectCreator(f.sdk, f.storage); await original.begin(input); original.dispose()
    const previous = f.context[field]; f.change({ [field]: 'other' }); const other = createProjectCreator(f.sdk, f.storage); assert.equal(other.pending(), undefined); other.dispose()
    f.change({ [field]: previous }); const recovered = createProjectCreator(f.sdk, f.storage); assert.ok(recovered.pending()); recovered.dispose()
  }
})
test('hiding an accepted create lets it finish while scope revocation aborts and rejects late results', async () => {
  for (const revoke of [false, true]) {
    const f = fixture(), c = createProjectCreator(f.sdk, f.storage); await c.begin(input); f.hold(); const running = c.submit(); await untilCalled(f)
    if (revoke) f.change({ contextRevision: 'grant-2' }); else f.event('hidden')
    assert.equal(f.calls[0].signal?.aborted, revoke); f.release()
    if (revoke) { await assert.rejects(running, code('SESSION_REVOKED')); assert.equal(c.pending()?.result, undefined) }
    else { await running; assert.equal(c.state(), 'committed'); assert.ok(c.pending()?.result) }
    c.dispose()
  }
})
test('hidden status reads abort and duplicate writes, status and clearance remain serialized', async () => {
  const f = fixture(), c = createProjectCreator(f.sdk, f.storage); await c.begin(input); f.hold(); const writing = c.submit(); await untilCalled(f)
  await assert.rejects(c.submit(), code('PENDING_CREATE')); await assert.rejects(c.status(), code('PENDING_CREATE')); await assert.rejects(c.clearTerminal(), code('PENDING_CREATE'))
  f.release(); await writing; f.calls.length = 0; f.hold(); const reading = c.status(); await untilCalled(f)
  await assert.rejects(c.clearTerminal(), code('PENDING_CREATE')); await assert.rejects(c.submit(), code('PENDING_CREATE'))
  f.event('hidden'); assert.equal(f.calls[0].signal?.aborted, true); f.release(); await assert.rejects(reading, code('DASHBOARD_HIDDEN'))
  f.event('active'); const clearing = c.clearTerminal(); await assert.rejects(c.status(), code('PENDING_CREATE')); await clearing; assert.equal(c.pending(), undefined); c.dispose()
})
test('revocation or journal replacement during the digest await cannot send the old request', async () => {
  for (const change of ['scope', 'storage']) {
    const f = fixture(), c = createProjectCreator(f.sdk, f.storage); await c.begin(input)
    const nativeDigest = crypto.subtle.digest, digest = nativeDigest.bind(crypto.subtle)
    let ready!: () => void, release!: () => void
    const started = new Promise<void>(resolve => { ready = resolve }), gate = new Promise<void>(resolve => { release = resolve })
    crypto.subtle.digest = async (algorithm, data) => { const result = await digest(algorithm, data); ready(); await gate; return result }
    try {
      const submitting = c.submit(); await started
      if (change === 'scope') f.change({ contextRevision: 'grant-2' })
      else { const [key, raw] = [...f.records][0], record = JSON.parse(raw); record.request.name = 'Replaced'; f.records.set(key, JSON.stringify(record)) }
      release(); await assert.rejects(submitting, code(change === 'scope' ? 'SESSION_REVOKED' : 'REQUEST_MISMATCH')); assert.equal(f.calls.length, 0)
    } finally { release(); crypto.subtle.digest = nativeDigest; c.dispose() }
  }
})
test('the create route is preserved and historical files landing reaches the project list', () => {
  assert.equal(canonicalRoute('project', '/create'), '/create')
  assert.deepEqual(legacyStudioTarget('/projects?view=files'), { appId: 'project', path: '/projects' })
})
