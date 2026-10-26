import test from 'node:test'
import assert from 'node:assert/strict'
import { ProjectReader } from '../src/platform/project-reader.ts'
import { connectHost, disconnectHost } from '../src/platform/host.ts'

const uuid = (n: number) => `00000000-0000-7000-8000-${String(n).padStart(12, '0')}`
const tick = () => new Promise(resolve => setImmediate(resolve))

test('thumbnail bursts share four read slots with documents, deduplicate, and cancel queued work', async () => {
  const previousWindow = Object.getOwnPropertyDescriptor(globalThis, 'window')
  const pending: Array<() => void> = []
  let active = 0, peak = 0, thumbnails = 0, documents = 0
  const enter = () => { active++; peak = Math.max(peak, active) }
  const leave = () => { active-- }
  const context = { appId: 'project' }
  const sdk = {
    version: '1.1.0',
    context: { get: () => context, subscribe: () => () => {} },
    theme: { subscribe: () => () => {} },
    navigation: { subscribe: () => () => {} },
    lifecycle: { subscribe: () => () => {} },
    app: { request: async () => {
      enter(); thumbnails++
      await new Promise<void>(resolve => pending.push(resolve)); leave()
      return { status: 200, data: { mime: 'image/webp', data: 'AA==' } }
    } },
    capabilities: { call: async () => {
      enter(); documents++
      await new Promise<void>(resolve => pending.push(resolve)); leave()
      // Invalid fixed-version metadata must fail without loading any media.
      return { mode: 'text', text: '', version: null }
    } },
  }
  Object.defineProperty(globalThis, 'window', { configurable: true, value: { $wujie: { props: {
    appSDK: sdk, dashboardHost: { ready() {}, resize() {} },
  } } } })
  const reader = new ProjectReader(uuid(1), uuid(2))
  reader.project = {
    project_id: uuid(1), revision_id: uuid(2), manifest: {
      name: 'Large material', category: 'ai_short_drama', domain_documents: [],
      files: Array.from({ length: 54 }, (_, i) => ({
        file_id: uuid(i + 3), path: `views/${i}.png`,
        content_ref: { store_id: uuid(100), artifact_id: uuid(i + 200), version_id: uuid(i + 300) },
      })),
    },
  }
  try {
    connectHost('project')
    const first = reader.thumbnail(uuid(3), 640)
    const duplicate = reader.thumbnail(uuid(3), 640)
    const burst = Array.from({ length: 52 }, (_, i) => reader.thumbnail(uuid(i + 4), 640))
    const document = reader.document(uuid(56)).catch(error => error)
    await tick()
    assert.equal(thumbnails, 4)
    assert.equal(documents, 0)
    // Release the first batch, then cancel while the rest are still queued.
    pending.splice(0).forEach(resolve => resolve())
    const [url, sameURL] = await Promise.all([first, duplicate])
    assert.equal(url, sameURL)
    await tick()
    assert.equal(thumbnails, 8)
    const completion = Promise.allSettled([...burst, document])
    reader.dispose()
    pending.splice(0).forEach(resolve => resolve())
    await completion
    assert.equal(thumbnails, 8, 'disposed readers must not start queued thumbnails')
    assert.equal(documents, 0, 'queued documents share the same limit and cancellation')
    assert.equal(peak, 4)
    assert.equal(active, 0)
  } finally {
    reader.dispose(); disconnectHost()
    if (previousWindow) Object.defineProperty(globalThis, 'window', previousWindow)
    else Reflect.deleteProperty(globalThis, 'window')
  }
})

test('a queued operation with its own retry does not receive another retry layer', async () => {
  const { ProjectReads } = await import('../src/platform/project-reads.ts')
  const reads = new ProjectReads(() => true)
  let attempts = 0
  await assert.rejects(reads.run(async () => { attempts++; throw new Error('thumbnail failed after its retry') }, 0))
  assert.equal(attempts, 1)
  reads.dispose()
})
