import test from 'node:test'
import assert from 'node:assert/strict'
import { lifecycleKey, readLifecycle, runLifecycle, type LifecycleRecord } from '../src/platform/project-lifecycle.ts'
import type { AppSDK, Context, Json } from '../src/platform/types.ts'
const id = (n: number) => `0199b501-0000-7000-8000-${n.toString(16).padStart(12, '0')}`
const record: LifecycleRecord = { action: 'delete', name: '验证项目', request: { project_id: id(1), change_id: id(2), expected_revision_id: id(3), expected_lifecycle_version: 0 } }
function fixture(statuses: number[]) {
  let context: Context = { userId: id(10), organizationId: id(11), stationId: id(12), appId: 'project', installationId: id(13), contextRevision: '1', projectId: null }
  const calls: { path: string; body?: Json }[] = []
  const sdk = { context: { get: () => context }, app: { request: async (input: { path: string; body?: Json }) => { calls.push(input); return { status: statuses.shift()!, data: { project_id: id(1), change_id: id(2), state: 'completed', deleted: true, lifecycle_version: 1, head_revision_id: id(3) } } } } } as unknown as AppSDK
  return { sdk, calls, change: () => { context = { ...context, organizationId: id(20) } } }
}
test('recovery checks stable status before retrying exactly the saved request', async () => {
  const f = fixture([404, 200]); await runLifecycle(f.sdk, record, true)
  assert.deepEqual(f.calls.map(c => c.path), ['/api/v1/project/lifecycle_status', '/api/v1/project/delete'])
  assert.deepEqual(f.calls[1].body, record.request)
  const completed = fixture([200]); await runLifecycle(completed.sdk, record, true); assert.equal(completed.calls.length, 1)
})
test('ambiguous failures do not replay mutation; conflicts and busy are definitive', async () => {
  for (const status of [503, 401]) { const f = fixture([status]); await assert.rejects(runLifecycle(f.sdk, record, true), (e: { definitive?: boolean }) => e.definitive === false); assert.equal(f.calls.length, 1) }
  const f = fixture([409]); await assert.rejects(runLifecycle(f.sdk, record, false), (e: { definitive?: boolean }) => e.definitive === true)
})
test('journal separates identities and rejects malformed or extra request fields', () => {
  const f = fixture([]), c = f.sdk.context.get(), key = lifecycleKey(c); f.change(); assert.notEqual(key, lifecycleKey(f.sdk.context.get()))
  assert.deepEqual(readLifecycle({ getItem: () => JSON.stringify(record) }, key), record)
  assert.throws(() => readLifecycle({ getItem: () => JSON.stringify({ ...record, request: { ...record.request, organization_id: id(9) } }) }, key))
})
