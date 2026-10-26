import test from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'
import { effectScope, nextTick, ref } from 'vue'
const require = createRequire(import.meta.url), ts = require('typescript')

test('visibility excludes clipped and hidden elements, waits for reobservation, and cleans up', async () => {
  let mount!: () => void, unmount!: () => void, life!: (event: string) => void, observer: any
  let stopped = false
  const module = { exports: {} as any }
  const dependencies: Record<string, any> = {
    vue: { ...require('vue'), onMounted: (fn: () => void) => { mount = fn }, onBeforeUnmount: (fn: () => void) => { unmount = fn } },
    './host': { useHost: () => ({ appSDK: { lifecycle: { subscribe: (fn: typeof life) => { life = fn; return () => { stopped = true } } } } }) },
  }
  const code = ts.transpileModule(readFileSync(new URL('../src/platform/visible.ts',import.meta.url),'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText
  new Function('require','module','exports',code)((name: string) => dependencies[name],module,module.exports)
  const previous = globalThis.IntersectionObserver
  globalThis.IntersectionObserver = class {
    observed: unknown[] = []; removed: unknown[] = []; disconnected = false
    constructor(readonly callback: (entries: unknown[]) => void, options: any) { observer = this; assert.equal(options.rootMargin,'0px') }
    observe(e: unknown) { this.observed.push(e) }
    unobserve(e: unknown) { this.removed.push(e) }
    disconnect() { this.disconnected = true }
  } as any
  const scope = effectScope(), element = ref({ id: 1 })
  try {
    const visible = scope.run(() => module.exports.useVisible(element))
    assert.equal(visible.value,false); mount()
    const show = (width: number, height: number) => observer.callback([{target:element.value,isIntersecting:true,intersectionRect:{width,height}}])
    show(0,20); assert.equal(visible.value,false)
    show(20,20); assert.equal(visible.value,true)
    life('hidden'); show(20,20); assert.equal(visible.value,false)
    life('active'); assert.equal(visible.value,false)
    show(20,20); assert.equal(visible.value,true)
    element.value = {id:2}; await nextTick(); assert.equal(visible.value,false)
    assert.equal(observer.removed.length,2)
    show(20,20); assert.equal(visible.value,true)
    unmount(); assert.equal(stopped,true); assert.equal(observer.disconnected,true)
  } finally { scope.stop(); globalThis.IntersectionObserver = previous }
})
