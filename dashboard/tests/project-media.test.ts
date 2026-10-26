import test from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { effectScope, nextTick, reactive } from 'vue'
import { renderer } from './vue-render.ts'

const require = createRequire(import.meta.url)
const { parse, compileScript } = require('vue/compiler-sfc')
const ts = require('typescript')
const filename = fileURLToPath(new URL('../src/components/ProjectMedia.vue', import.meta.url))
const { descriptor } = parse(readFileSync(filename, 'utf8'), { filename })
const source = compileScript(descriptor, { id: 'project-media-interaction' }).content
const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText

// Exercise the real SFC handlers and watchers without a browser or media service.
function videoComponent(media: (fileId: string, kind: string) => Promise<string>) {
  const cleanup: Array<() => void> = []
  const module = { exports: {} as any }
  const dependencies = {
    vue: { ...require('vue'), onBeforeUnmount: (fn: () => void) => cleanup.push(fn), onServerPrefetch() {} },
    '../platform/host': { useHost: () => ({ appSDK: {} }) },
    '../platform/project-reader': { readerError: (error: Error) => error.message },
    '../platform/media-preview': { closePreview() {} },
    '../platform/set-viewer-entry': { findSetPackageForFile: () => null },
  }
  new Function('require', 'module', 'exports', code)((name: string) => {
    assert(name in dependencies, `unexpected component dependency: ${name}`)
    return dependencies[name as keyof typeof dependencies]
  }, module, module.exports)
  const props = reactive({ reader: { media }, fileId: 'first-video', kind: 'video', label: '机位预演', thumbnail: true })
  const scope = effectScope()
  const state = scope.run(() => module.exports.default.setup(props, { expose() {} }))
  const dialog = { opens: 0, closes: 0, pauses: 0, showModal() { this.opens++ }, close() { this.closes++ }, querySelector() { return { pause: () => dialog.pauses++ } } }
  state.videoViewer.value = dialog
  const previousHTMLElement = globalThis.HTMLElement
  globalThis.HTMLElement = class {} as typeof HTMLElement
  return { props, state, dialog, dispose() { cleanup.forEach(fn => fn()); scope.stop(); globalThis.HTMLElement = previousHTMLElement } }
}

test('video tiles render an on-demand entry without reading media or creating a player', async () => {
  let reads = 0
  const reader = { media: async () => { reads++; return 'blob:video' }, thumbnail: async () => { throw new Error('video must not request image thumbnails') } }
  const html = await renderer().render('components/ProjectMedia.vue', { reader, fileId: 'video', kind: 'video', label: '机位预演', thumbnail: true })
  assert.equal(reads, 0)
  assert.match(html, /播放机位预演/)
  assert.doesNotMatch(html, /<video\b|autoplay|blob:video/)
})

test('opening a video loads once, closing stops playback, and reopening uses the cached source', async () => {
  const reads: string[] = []
  const view = videoComponent(async (file, kind) => { reads.push(`${file}:${kind}`); return 'blob:video' })
  try {
    assert.deepEqual(reads, [])
    assert.equal(view.state.videoOpen.value, false)
    await view.state.openVideo()
    assert.deepEqual(reads, ['first-video:video'])
    assert.equal(view.dialog.opens, 1)
    assert.equal(view.state.videoOpen.value, true)
    view.state.closeVideo()
    assert.equal(view.dialog.pauses, 1)
    assert.equal(view.dialog.closes, 1)
    assert.equal(view.state.videoOpen.value, false)
    await view.state.openVideo()
    assert.deepEqual(reads, ['first-video:video'])
    assert.equal(view.dialog.opens, 2)
    assert.equal(view.state.videoOpen.value, true)
  } finally { view.dispose() }
})

test('switching a video while its read is pending prevents a late dialog and resets playback', async () => {
  let finish!: (url: string) => void
  const view = videoComponent(() => new Promise(resolve => { finish = resolve }))
  try {
    const pending = view.state.openVideo()
    assert.equal(view.state.busy.value, true)
    view.props.fileId = 'second-video'
    await nextTick()
    finish('blob:old-video')
    await pending
    assert.equal(view.dialog.opens, 0)
    assert.equal(view.state.source.value, '')
    assert.equal(view.state.videoOpen.value, false)
  } finally { view.dispose() }
})

test('failed video reads leave the tile closed and allow a user retry', async () => {
  let reads = 0
  const view = videoComponent(async () => { if (++reads === 1) throw new Error('读取失败'); return 'blob:video' })
  try {
    await view.state.openVideo()
    assert.equal(view.state.error.value, '读取失败')
    assert.equal(view.dialog.opens, 0)
    assert.equal(view.state.videoOpen.value, false)
    await view.state.openVideo()
    assert.equal(reads, 2)
    assert.equal(view.dialog.opens, 1)
    assert.equal(view.state.videoOpen.value, true)
  } finally { view.dispose() }
})
