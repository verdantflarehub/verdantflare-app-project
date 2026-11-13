import test from 'node:test'
import assert from 'node:assert/strict'
import { placeResultCards, resultCardRatio, resultColumnCount } from '../src/platform/result-cards.ts'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'
import { effectScope, nextTick, reactive } from 'vue'
import * as layout from '../src/platform/result-cards.ts'
import { renderer } from './vue-render.ts'

const require = createRequire(import.meta.url)
function setupComponent(name: string, props: object) {
  const { parse, compileScript } = require('vue/compiler-sfc')
  const ts = require('typescript')
  const { descriptor } = parse(readFileSync(new URL(`../src/components/${name}.vue`, import.meta.url), 'utf8'))
  const source = compileScript(descriptor, { id: 'result-interaction' }).content
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const cleanup: (() => void)[] = []
  const module = { exports: {} as any }
  const dependencies: Record<string, unknown> = {
    vue: { ...require('vue'), onBeforeUnmount: (fn: () => void) => cleanup.push(fn) },
    '../platform/result-cards': layout,
    './ProjectMedia.vue': {}, './ProjectPrompt.vue': {},
  }
  new Function('require', 'module', 'exports', code)((key: string) => {
    assert(key in dependencies, key)
    return dependencies[key]
  }, module, module.exports)
  const scope = effectScope()
  const state = scope.run(() => module.exports.default.setup(props, { expose() {} }))
  return { state, dispose() { cleanup.forEach(fn => fn()); scope.stop() } }
}

test('actual image ratios are preserved and extreme dimensions are clamped', () => {
  for (const [width, height, expected] of [[0, 0, 9 / 16], [NaN, 100, 9 / 16], [720, 1280, 9 / 16], [800, 1000, .8], [100, 100, 1], [150, 100, 1.5], [3000, 100, 16 / 9], [100, 3000, 9 / 16]]) {
    assert.equal(resultCardRatio(width, height), expected)
  }
})

test('column count adapts to available width and uses two columns on narrow screens', () => {
  for (const [width, expected] of [[320, 2], [719, 2], [720, 2], [1000, 4], [1500, 6]]) assert.equal(resultColumnCount(width), expected)
})

test('mixed aspect ratios go in the shortest column with stable tie ordering', () => {
  assert.deepEqual(placeResultCards([9 / 16, 16 / 9, 1, 16 / 9, 9 / 16], 2), [[0, 4], [1, 2, 3]])
  assert.deepEqual(placeResultCards(Array(5).fill(9 / 16), 2), [[0, 2, 4], [1, 3]])
  assert.deepEqual(placeResultCards([], 2), [[], []])
})

test('real cards keep columns on image load and preserve source order for gallery navigation', async () => {
  const props = reactive({ reader: {}, detail: { key: 'host', result_file_ids: ['a', 'b', 'c', 'd', 'e'], result_meta: { b: { label: '横图', prompt: 'exact prompt' } } } })
  const view = setupComponent('CharacterResults', props)
  try {
    assert.deepEqual(view.state.columns.value, [[0, 2, 4], [1, 3]])
    view.state.ratios.value = { a: 9 / 16, b: 16 / 9, c: 1, d: 16 / 9, e: 9 / 16 }
    await nextTick()
    assert.deepEqual(view.state.columns.value, [[0, 2, 4], [1, 3]], 'loads must not move already placed cards')
    assert.deepEqual(view.state.gallery.value, ['a', 'b', 'c', 'd', 'e'].map((fileId, i) => ({ fileId, label: `结果 ${i + 1}${i === 1 ? ' · 横图' : ''}` })))
    assert.deepEqual(view.state.cards.value[1].map((item: any) => item.index), [1, 3])
    view.state.width.value = 1000
    await nextTick()
    assert.equal(view.state.columns.value.length, 4)
    assert.deepEqual(view.state.columns.value[1], [1, 4], 'resize places new layout by measured shortest height')
  } finally { view.dispose() }
})

test('cards render collapsed per-image prompts only and never inherit the extension prompt', async () => {
  const html = await renderer().render('components/CharacterResults.vue', {
    reader: {}, detail: { key: 'host', result_file_ids: ['a', 'b'], prompt: 'extension-only', result_meta: { a: { label: '候选', prompt: 'first line\n第二行' } } },
  })
  assert.match(html, /结果 1 · 候选/)
  assert.match(html, /结果 2/)
  assert.equal((html.match(/<details\b/g) || []).length, 1)
  assert.doesNotMatch(html, /<details[^>]*\bopen\b|extension-only/)
  assert.match(html, /first line\n第二行/)
  assert.match(html, /复制提示词/)
})

test('prompt copy sends original whitespace and reports clipboard failure honestly', async () => {
  const previous = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
  const copied: string[] = []
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async (text: string) => { copied.push(text) } } })
  const view = setupComponent('ProjectPrompt', {})
  try {
    await view.state.copy('  first\n第二行\n')
    assert.deepEqual(copied, ['  first\n第二行\n'])
    assert.equal(view.state.message.value, '已复制')
    navigator.clipboard.writeText = async () => { throw new Error('denied') }
    await view.state.copy('text')
    assert.match(view.state.message.value, /复制失败/)
  } finally {
    view.dispose()
    if (previous) Object.defineProperty(navigator, 'clipboard', previous)
    else Reflect.deleteProperty(navigator, 'clipboard')
  }
})
