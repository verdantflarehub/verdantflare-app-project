import test from 'node:test'
import assert from 'node:assert/strict'
import { parseProjectContent, type ProjectOpen } from '../src/platform/project-content.ts'
import { musicReturnRoute, musicSourceEntry, musicVersionRoute } from '../src/platform/music-production.ts'
import { projectSources, projectSourceRoute } from '../src/platform/project-source.ts'
import { businessChanges } from '../src/platform/project-revision-diff.ts'
import { renderer } from './vue-render.ts'
const id = (n: number) => `018ef89a-0000-7000-8000-${String(n).padStart(12, '0')}`
function fixture() {
  const project: ProjectOpen = { project_id: id(1), revision_id: id(2), head_revision_id: id(2), can_write: true, manifest: { name: '专辑', category: 'music', domain_documents: [], files: [{ file_id: id(10), path: 'demo.wav', content_ref: { store_id: id(20), artifact_id: id(21), version_id: id(22) } }] } }
  const v = { key: 'demo1', label: 'Demo 第一版', kind: 'demo', basis_revision_id: id(3), parent_version_key: null as string | null, change_note: '调整第二段歌词', review: { state: 'unreviewed', record_file_id: null }, file_refs: [{ file_id: id(10), content_ref: { ...project.manifest.files[0].content_ref }, purpose: 'output' }], sources: [] as any[] }
  const raw = { schema_version: 1, kind: 'project-content', category: 'music', music: { songs: [{ key: 'song', title: '第一首', lyrics: '第一段\n第二段', production: { versions: [v], selected_version_key: null as string | null } }] } }
  return { project, raw, v, parse: () => parseProjectContent(JSON.stringify(raw), project) }
}
test('music production rejects stale bytes, cycles, foreign files, missing selections and wrong kinds', () => {
  for (const mutate of [
    (f: ReturnType<typeof fixture>) => { f.v.file_refs[0].content_ref.version_id = id(99) },
    (f: ReturnType<typeof fixture>) => { f.v.parent_version_key = 'demo1' },
    (f: ReturnType<typeof fixture>) => { f.v.file_refs[0].file_id = id(99) },
    (f: ReturnType<typeof fixture>) => { f.raw.music.songs[0].production.selected_version_key = 'absent' },
    (f: ReturnType<typeof fixture>) => { f.v.kind = 'video' },
    (f: ReturnType<typeof fixture>) => { f.v.file_refs[0].purpose = 'sample' },
    (f: ReturnType<typeof fixture>) => { f.v.sources.push({ project_id: id(9), revision_id: id(8), item_key: 'song', kind: 'song', version_key: 'one', file_id: null }) },
  ]) { const f = fixture(); mutate(f); assert.throws(f.parse, /音乐制作版本/) }
  const f = fixture(); assert.equal(f.parse().music!.songs[0].production!.versions[0].label, 'Demo 第一版')
})
test('source ownership, fixed version and external return route are validated independently', () => {
  const f = fixture(), source = { project_id: id(1), revision_id: id(2), kind: 'song' as const, item_key: 'song', version_key: 'demo1', file_id: null }
  assert.equal(musicSourceEntry(f.parse().music, source).key, 'song')
  assert.throws(() => musicSourceEntry(f.parse().music, { ...source, version_key: 'missing' }), /不属于/)
  const route = musicVersionRoute({ q: '专辑', sort: 'name' }, source)
  assert.equal(route.query.revision, id(2)); assert.equal(route.query.version, 'demo1')
  const fileTarget = musicVersionRoute({}, { ...source, version_key: null, file_id: id(10) })
  assert.equal(fileTarget.query.tab, 'files'); assert.equal(fileTarget.query.file, id(10)); assert.equal(fileTarget.query.item, undefined)
  for (const bad of ['https://evil.invalid/projects', '//evil.invalid/projects', '/projects?category=video', '/projects?category=music&project=no']) assert.equal(musicReturnRoute(bad), undefined)
  const back = `/projects?category=music&project=${id(1)}&revision=${id(2)}&tab=mvs&item=mv&version=one&fromMusic=old`
  assert.equal(new URL(musicReturnRoute(back)!, 'https://example.invalid').searchParams.has('fromMusic'), false)
})
test('music references and revision comparison use the registered version, not the latest source', () => {
  const f = fixture()
  f.v.sources.push({ project_id: id(5), revision_id: id(6), kind: 'model', item_key: 'voice', version_key: null, file_id: id(7) })
  const parsed = f.parse(), sources = projectSources(parsed)
  assert.equal(sources.length, 1); assert.equal(sources[0].ownerGroup, 'songs'); assert.equal(sources[0].ownerVersion, 'demo1')
  const route = projectSourceRoute(f.project, sources[0], { q: '专辑' })
  assert.equal(route.query.revision, id(6)); assert.equal(route.query.tab, 'files'); assert.equal((route.query as Record<string, unknown>).file, id(7)); assert.equal(route.query.fromCategory, 'music')
  assert.equal(route.query.fromReference, sources[0].key)
  const before = structuredClone(parsed); before.music!.songs[0].production!.versions = []
  assert.equal(businessChanges(before, parsed)[0].key, 'music/songs/song/version/demo1')
  parsed.music!.songs[0].production!.selected_version_key = 'demo1'
  assert.ok(businessChanges(f.parse(), parsed).some(c => c.key === 'music/songs/song/selected'))
})
test('version detail distinguishes browsing, adoption, master and absent fixed versions', async () => {
  const f = fixture(), detail = f.parse().music!.songs[0], render = renderer(), reader = { project: f.project, file: () => f.project.manifest.files[0] }
  const html = await render.render('components/MusicProduction.vue', { reader, detail, group: 'songs' })
  assert.match(html, /Demo 第一版/); assert.match(html, /实际选用：未选定/); assert.match(html, /未添加母带/); assert.match(html, /调整第二段歌词/); assert.match(html, /选用当前版本/)
  const absent = await render.render('components/MusicProduction.vue', { reader, detail, group: 'songs' }, '/projects?version=absent')
  assert.match(absent, /没有此制作版本/); assert.doesNotMatch(absent, /选用当前版本/); assert.doesNotMatch(absent, /调整第二段歌词/)
  delete detail.production
  const legacy = await render.render('components/MusicProduction.vue', { reader, detail, group: 'songs' })
  assert.match(legacy, /尚未登记制作版本/); assert.match(legacy, /未添加母带/); assert.doesNotMatch(legacy, /选用当前版本/)
})

test('viewing a master keeps actual adoption separate and displays only its own review and note', async () => {
  const f = fixture()
  f.raw.music.songs[0].production.versions.push({ ...structuredClone(f.v), key: 'master1', label: '母带第一版', kind: 'master', parent_version_key: 'demo1', change_note: '母带响度调整' })
  f.raw.music.songs[0].production.selected_version_key = 'demo1'
  const detail = f.parse().music!.songs[0], reader = { project: f.project, file: () => f.project.manifest.files[0] }
  const html = await renderer().render('components/MusicProduction.vue', { reader, detail, group: 'songs' }, '/projects?version=master1')
  assert.match(html, /实际选用：Demo 第一版/); assert.match(html, /母带响度调整/); assert.match(html, /未审核/)
  assert.doesNotMatch(html, /调整第二段歌词/); assert.doesNotMatch(html, /未添加母带/)
  assert.equal(detail.production!.selected_version_key, 'demo1')
})
