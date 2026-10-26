import test from 'node:test'
import assert from 'node:assert/strict'
import type { DramaVersion } from '../src/platform/project-content.ts'
import { renderer } from './vue-render.ts'

const id = (n: number) => `01900000-0000-7000-8000-${String(n).padStart(12, '0')}`
const render = renderer()
function fixture(counts: number[]) {
  const versions: DramaVersion[] = counts.map((count, index) => ({
    key: `v${index + 1}`, label: `制作版本 ${index + 1}`, basis_revision_id: id(2),
    parent_version_key: null, change_note: '保留完整版本文件',
    file_refs: Array.from({ length: count }, (_, n) => ({
      file_id: id(1000 * (index + 1) + n), purpose: 'preview',
      content_ref: { store_id: id(3), artifact_id: id(4), version_id: id(5) },
    })),
    generation_record_file_ids: [], review: { state: 'unreviewed', record_file_id: null },
  }))
  const files = versions.flatMap(v => v.file_refs.map(f => ({ ...f, path: `previews/${f.file_id}.png` })))
  const requested: string[] = []
  const reader = {
    project: { manifest: { files } }, content: null,
    file: (fileId: string) => files.find(f => f.file_id === fileId)!,
    thumbnail: async (fileId: string) => { requested.push(fileId); return 'blob:preview' },
  }
  return { reader, versions, requested }
}

test('a large material version mounts and reads only the first 12 previews', async () => {
  const f = fixture([4, 148])
  const selections = [{ purpose: 'preview', version_key: 'v1' }]
  const before = JSON.stringify({ versions: f.versions, selections })
  const html = await render.render('components/drama/DramaVersions.vue', { reader: f.reader, versions: f.versions, selections, materialMode: true })
  assert.deepEqual(f.requested, f.versions[1].file_refs.slice(0, 12).map(f => f.file_id))
  assert.equal((html.match(/class="drama-file tile"/g) || []).length, 12)
  assert.match(html, /已显示 12 \/ 148 个文件/)
  assert.match(html, /加载更多文件/)
  assert.equal(JSON.stringify({ versions: f.versions, selections }), before)
})

test('an explicitly selected older version starts with its own first batch', async () => {
  const f = fixture([25, 148])
  const html = await render.render('components/drama/DramaVersions.vue', { reader: f.reader, versions: f.versions }, '/projects?version=v1')
  assert.deepEqual(f.requested, f.versions[0].file_refs.slice(0, 12).map(f => f.file_id))
  assert.match(html, /已显示 12 \/ 25 个文件/)
})

test('small material versions retain all previews without a load-more control', async () => {
  const f = fixture([4])
  const html = await render.render('components/drama/DramaVersions.vue', { reader: f.reader, versions: f.versions })
  assert.deepEqual(f.requested, f.versions[0].file_refs.map(f => f.file_id))
  assert.equal((html.match(/class="drama-file tile"/g) || []).length, 4)
  assert.doesNotMatch(html, /加载更多文件|file-pagination/)
})
