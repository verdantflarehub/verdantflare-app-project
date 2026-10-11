import test from 'node:test'
import assert from 'node:assert/strict'
import { createWorkspaceClient, snapshot, conflictPreview, workspaceError, type WorkspaceRequest } from '../src/platform/workspace-client.ts'
import type { AppSDK, Context, Json, Lifecycle } from '../src/platform/types.ts'
const project = '01900000-0000-7000-8000-000000000001', file = '01900000-0000-7000-8000-000000000002'
const handle = 'fd853794-1600-4a93-8332-fbf95faf9a5e'
function fixture() {
  let context: Context = { userId: 'user-a', organizationId: 'org', stationId: 'station', projectId: null, appId: 'project', installationId: 'install', contextRevision: 'scope' }
  const lifecycle = new Set<(v: Lifecycle) => void>(), contexts = new Set<(v: Context) => void>()
  const requests: { name: string; input: Json; signal?: AbortSignal }[] = []
  let handler: () => Promise<Json> = async () => ({})
  const sdk: AppSDK = {
    version: '1.1.0', context: { get: () => context, subscribe: fn => { contexts.add(fn); return () => contexts.delete(fn) } },
    lifecycle: { subscribe: fn => { lifecycle.add(fn); return () => lifecycle.delete(fn) } }, theme: { get: () => 'light', subscribe: () => () => {} },
    navigation: { get: () => ({ path: '/', revision: 1 }), subscribe: () => () => {}, request: async () => {} },
    capabilities: { call: async (name, input, options) => { requests.push({ name, input, signal: options?.signal }); return handler() } },
    app: { request: async () => { throw new Error('Direct API must not be reached') } },
  }
  return { sdk, requests, lifecycle, contexts, handler: (next: typeof handler) => { handler = next }, emit: (v: Lifecycle) => { for (const fn of lifecycle) fn(v) }, change: () => { context = { ...context, userId: 'user-b' }; for (const fn of contexts) fn(context) } }
}
test('native operations use only the declared capability and reject paths and identity fields', async () => {
  const f = fixture(), client = createWorkspaceClient(f.sdk)
  await client.call({ operation: 'openDirectory', project_id: project })
  assert.equal(f.requests[0].name, 'workspace.local')
  for (const request of [
    { operation: 'openDirectory', project_id: project, directory: 'C:/private' },
    { operation: 'importFiles', workspace_handle: handle, role: 'reference', source_path: 'C:/private' },
    { operation: 'saveFiles', workspace_handle: handle, file_ids: [file], installation_id: 'other' },
    { operation: 'saveTexts', workspace_handle: handle, file_ids: [file, file] },
    { operation: 'fetch', workspace_handle: handle, file_id: file, max_bytes: 2 ** 41 },
    { operation: 'execute', workspace_handle: handle },
  ]) await assert.rejects(client.call(request as WorkspaceRequest))
  assert.equal(f.requests.length, 1)
  client.dispose()
})
test('hiding cancels reads but allows an accepted write to finish and prevents new work', async () => {
  const f = fixture(), client = createWorkspaceClient(f.sdk)
  const resolves: ((value: Json) => void)[] = []
  f.handler(() => new Promise(done => resolves.push(done)))
  const write = client.call({ operation: 'saveFiles', workspace_handle: handle, file_ids: [file] })
  const read = client.call({ operation: 'status', workspace_handle: handle })
  f.emit('hidden')
  assert.equal(f.requests[0].signal?.aborted, false)
  assert.equal(f.requests[1].signal?.aborted, true)
  await assert.rejects(client.call({ operation: 'resume', workspace_handle: handle }), { name: 'AbortError' })
  resolves[0]({ revision_id: file }); resolves[1]({})
  assert.deepEqual(await write, { revision_id: file })
  await assert.rejects(read, { name: 'AbortError' })
  client.dispose()
})
test('scope replacement cancels accepted writes and rejects late results without retaining the old handle', async () => {
  const f = fixture(), client = createWorkspaceClient(f.sdk)
  let resolve!: (value: Json) => void
  f.handler(() => new Promise(done => { resolve = done }))
  client.remember(project, handle)
  const pending = client.call({ operation: 'resume', workspace_handle: handle })
  f.change()
  assert.equal(f.requests[0].signal?.aborted, true)
  resolve({ project_id: project })
  await assert.rejects(pending, { name: 'AbortError' })
  assert.equal(client.retained(project), undefined)
  assert.equal(f.lifecycle.size + f.contexts.size, 0)
})
test('pending errors keep recovery handles and never trigger another commit automatically', async () => {
  const f = fixture(), client = createWorkspaceClient(f.sdk)
  const failure = Object.assign(new Error('PENDING_COMMIT'), { code: 'PENDING_COMMIT' })
  f.handler(async () => { throw failure })
  client.remember(project, handle)
  await assert.rejects(client.call({ operation: 'saveTexts', workspace_handle: handle, file_ids: [file] }), failure)
  assert.equal(client.retained(project), handle)
  assert.equal(f.requests.length, 1)
  assert.match(workspaceError(failure), /恢复原提交/)
  assert.doesNotMatch(workspaceError(new Error('C:/private/path')), /private/)
  client.dispose(); client.remember(project)
})
test('workspace snapshots require the exact project, revision and manifest file mapping', () => {
  const value = { workspace_handle: handle, state: { schema_version: 1, kind: 'local-workspace', project_id: project, base_revision_id: file }, manifest: { project_id: project, name: 'Project', files: [{ file_id: file, path: 'review.md', role: 'review' }] }, files: [{ file_id: file, path: 'review.md', state: 'modified' }] }
  assert.equal(snapshot(value, project).state.base_revision_id, file)
  assert.throws(() => snapshot(value, file))
  assert.throws(() => snapshot({ ...value, files: [{ ...value.files[0], path: 'other.md' }] }, project))
  assert.throws(() => snapshot({ ...value, files: [value.files[0], value.files[0]] }, project))
  assert.throws(() => snapshot({ ...value, manifest: { ...value.manifest, files: [{ ...value.manifest.files[0], path: '../private' }] } }, project))
})
test('conflict preview preserves exact commit and request identity and rejects duplicate choices', async () => {
  const preview = { project_id: project, commit_id: file, request_sha256: 'a'.repeat(64), base_revision_id: file, head_revision_id: project, files: [{ index: 0, pending: { path: 'review.md', role: 'review', text: 'local' }, remote_changed: true, can_keep_pending: true }] }
  assert.equal(conflictPreview(preview, project).request_sha256, preview.request_sha256)
  assert.throws(() => conflictPreview({ ...preview, files: [...preview.files, ...preview.files] }, project))
  const f = fixture(), client = createWorkspaceClient(f.sdk)
  await assert.rejects(client.call({ operation: 'resolveConflict', workspace_handle: handle, resolution: { commit_id: file, request_sha256: preview.request_sha256, head_revision_id: project, choices: [{ index: 0, choice: 'head' }, { index: 0, choice: 'pending' }] } }))
  assert.equal(f.requests.length, 0)
  client.dispose()
})
