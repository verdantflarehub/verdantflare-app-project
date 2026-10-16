import test from 'node:test'
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { createCreativeSaver } from '../src/platform/creative-save.ts'
import type { AppSDK, Json } from '../src/platform/types.ts'
import type { ProjectOpen } from '../src/platform/project-content.ts'

const id = (n: number) => `01900000-0000-7000-8000-${String(n).padStart(12, '0')}`
const ref = { store_id: id(5), artifact_id: id(6), version_id: id(7) }
const opened: ProjectOpen = { project_id: id(1), revision_id: id(2), manifest: { name: 'Test', category: 'creative_video', files: [{ file_id: id(3), path: 'review.md', role: 'review', content_ref: ref }], domain_documents: [] } }

test('initial creative document recovers a lost first response with the original commit ID', async () => {
  const records = new Map<string, string>(), commits = new Map<string, { request: any; result: any }>(), calls: any[] = []
  const storage = { getItem: (key: string) => records.get(key) ?? null, setItem: (key: string, value: string) => { records.set(key, value) }, removeItem: (key: string) => { records.delete(key) } } as Storage
  let lose = true, loseLink = true, documentText = ''
  const sdk = {
    context: { get: () => ({ userId: 'user', organizationId: 'org', stationId: 'station', appId: 'project', installationId: 'install', contextRevision: '1' }) },
    app: { request: async ({ path, body }: { path: string; body: Json }) => {
      assert.equal(path, '/api/v1/project/commit')
      const request = body as any; calls.push(request)
      let saved = commits.get(request.commit_id)
      if (!saved) {
        const phase = commits.size
        const manifest = structuredClone(phase === 0 ? opened.manifest : commits.values().next().value!.result.manifest)
        if (phase === 0) {
          documentText = request.changes.upsert_files[0].text
          manifest.files.push({ file_id: id(4), path: 'creative/project-content.json', role: 'review', content_ref: ref })
        } else manifest.domain_documents = request.changes.set_domain_documents
        saved = { request, result: { project_id: id(1), revision_id: id(10 + phase), manifest } }
        commits.set(request.commit_id, saved)
      } else assert.deepEqual(request, saved.request)
      if (lose) { lose = false; throw new Error('lost first response') }
      if (commits.size === 2 && loseLink) { loseLink = false; throw new Error('lost link response') }
      return { status: 200, data: saved.result, requestId: 'request' }
    } },
    capabilities: { call: async (name: string, args: any) => {
      assert.equal(name, 'artifact.read')
      assert.equal(args.access.project_revision_id, id(11))
      return { version: { sha256: createHash('sha256').update(documentText).digest('hex') } }
    } },
  } as unknown as AppSDK
  let saver = createCreativeSaver(sdk, id(1), storage)
  await assert.rejects(saver.begin(opened, undefined, 'promo', 'Launch', { client: '', audience: '', message: '', action: '' }, null), /lost first response/)
  assert.ok(saver.pending())
  const [key, raw] = [...records.entries()][0]
  const changed = JSON.parse(raw); changed.request.changes.upsert_files[0].text = '{}'
  records.set(key, JSON.stringify(changed))
  await assert.rejects(saver.resume(), /保存记录或服务返回内容不正确/)
  assert.equal(calls.length, 1)
  records.set(key, raw)
  saver = createCreativeSaver(sdk, id(1), storage)
  await assert.rejects(saver.resume(), /lost link response/)
  assert.equal(saver.pending()?.phase, 'link')
  saver = createCreativeSaver(sdk, id(1), storage)
  const result = await saver.resume()
  assert.equal(result.revision_id, id(11))
  assert.deepEqual(calls[0], calls[1])
  assert.deepEqual(calls[2], calls[3])
  assert.equal(commits.size, 2)
  assert.equal(saver.pending(), null)
  assert.equal(result.manifest.domain_documents[0].file_id, id(4))
})
