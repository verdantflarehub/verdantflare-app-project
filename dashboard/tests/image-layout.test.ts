import test from 'node:test'
import assert from 'node:assert/strict'
import { renderer } from './vue-render.ts'

function reader() {
  const version = (key: string, files: string[]) => ({ key, label: key, parent_version_key: null, change_note: '制作说明', reference_file_ids: ['source'], prompt_file_ids: [], file_refs: files.map(file_id => ({ file_id, purpose: 'output' })) })
  return { content: { image: { works: [{ key: 'poster', title: '作品', display_version_key: 'v2', versions: [version('v1', ['old']), version('v2', ['final-png', 'final-jpeg'])] }] } },
    file: (id: string) => ({ path: id }), media: async (id: string) => 'memory:' + id }
}

test('image detail starts with one final result, keeps all output choices, and defers reference media', async () => {
  const render = renderer(), html = await render.render('components/ImageProject.vue', { reader: reader(), tab: 'works' }, '/projects?item=poster')
  assert.equal((html.match(/class="project-media image/g) || []).length, 1)
  assert.match(html, /制作成果 · v2/)
  assert.match(html, /aria-pressed="true"[^>]*>final-png/)
  assert.match(html, /aria-pressed="false"[^>]*>final-jpeg/)
  assert.match(html, /aria-expanded="false"[^>]*>展开参考与对照/)
  assert.doesNotMatch(html, /aria-label="参考图/)
  assert.ok(html.indexOf('制作成果 · v2') < html.indexOf('制作版本'))
})

test('explicit historical version changes result and comparison does not duplicate output formats', async () => {
  const render = renderer(), r = reader()
  const old = await render.render('components/ImageProject.vue', { reader: r, tab: 'works' }, '/projects?item=poster&version=v1')
  assert.match(old, /制作成果 · v1/); assert.doesNotMatch(old, /final-jpeg/)
  const compared = await render.render('components/ImageProject.vue', { reader: r, tab: 'works' }, '/projects?item=poster&compare=v1')
  assert.equal((compared.match(/class="project-media image/g) || []).length, 3)
  const missing = await render.render('components/ImageProject.vue', { reader: r, tab: 'works' }, '/projects?item=poster&version=absent')
  assert.match(missing, /没有这个制作版本/); assert.doesNotMatch(missing, /制作成果 · v2/)
})
