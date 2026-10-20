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
  assert.match(html, /<option value="final-png"/)
  assert.match(html, /<option value="final-jpeg"/)
  assert.match(html, /aria-expanded="false"[^>]*><span[^>]*>参考照片/)
  assert.doesNotMatch(html, /aria-label="参考图/)
  assert.ok(html.indexOf('制作成果 · v2') < html.indexOf('制作版本'))
})

test('project landing directly renders its result without an intermediate work card or duplicate navigation', async () => {
  const r = reader(), render = renderer()
  const html = await render.render('components/ImageProject.vue', { reader: r, tab: 'works' })
  assert.match(html, /制作成果 · v2/)
  assert.doesNotMatch(html, /project-card|查看作品与版本|output-choice/)
  assert.match(html, /下载图片/)
  assert.equal((html.match(/<select/g) || []).length, 2)
  const second = structuredClone(r.content.image.works[0]); second.key = 'other'; second.title = '另一件作品'
  r.content.image.works.push(second)
  const multi = await render.render('components/ImageProject.vue', { reader: r, tab: 'works' })
  assert.match(multi, /另一件作品/); assert.equal((multi.match(/<select/g) || []).length, 3)
  const missing = await render.render('components/ImageProject.vue', { reader: r, tab: 'works' }, '/projects?item=missing')
  assert.match(missing, /没有这件作品/); assert.doesNotMatch(missing, /制作成果 · v2/)
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
