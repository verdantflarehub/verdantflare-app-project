import test from 'node:test'
import assert from 'node:assert/strict'
import { ProjectReads } from '../src/platform/project-reads.ts'
const tick = () => new Promise(resolve => setImmediate(resolve))

test('material reads remain bounded and preserve queued results', async () => {
  const queue = new ProjectReads(() => false), releases: (() => void)[] = [], started: number[] = []
  let concurrent = 0, peak = 0
  const all = Array.from({ length: 21 }, (_, n) => queue.run(async () => {
    started.push(n); peak = Math.max(peak, ++concurrent)
    await new Promise<void>(resolve => releases.push(resolve)); concurrent--; return n
  }))
  assert.equal(started.length, 4)
  while (started.length < all.length || concurrent) { releases.splice(0).forEach(resolve => resolve()); await tick() }
  assert.deepEqual(await Promise.all(all), Array.from({ length: 21 }, (_, n) => n))
  assert.deepEqual(started, Array.from({ length: 21 }, (_, n) => n)); assert.equal(peak, 4)
})

test('only transient reads retry, and disposal prevents waiting work from starting', async () => {
  const queue = new ProjectReads((e: any) => e.status === 502)
  let attempts = 0
  assert.equal(await queue.run(async () => { if (++attempts < 3) throw { status: 502 }; return 'fixed bytes' }), 'fixed bytes')
  assert.equal(attempts, 3)
  for (const status of [403, 401, 400]) {
    attempts = 0; await assert.rejects(queue.run(async () => { attempts++; throw { status } })); assert.equal(attempts, 1)
  }
  const pending = new ProjectReads(() => false, 1)
  let release!: () => void, calls = 0
  const first = pending.run(() => new Promise<void>(resolve => { release = resolve }))
  const waiting = pending.run(async () => ++calls)
  const rejected = assert.rejects(waiting, /项目已切换/)
  pending.dispose(); await rejected; release(); await first
  assert.equal(calls, 0); await assert.rejects(pending.run(async () => ++calls), /项目已切换/)
})
