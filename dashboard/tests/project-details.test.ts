import test from 'node:test'
import assert from 'node:assert/strict'
import { parseProjectContent, type ProjectOpen } from '../src/platform/project-content.ts'
import { coverFile, coverMetadata, coverPosition, defaultCoverPosition, dragCover } from '../src/platform/cover-position.ts'
import { fixedMusicSource, projectSources, projectSourceRoute, sourceReturnCategory } from '../src/platform/project-source.ts'
import { businessChanges, revisionChanges } from '../src/platform/project-revision-diff.ts'
import { renderer } from './vue-render.ts'

const id = (n: number) => `01900000-0000-7000-8000-${String(n).padStart(12, '0')}`
const file = (n: number, path = `${n}.png`) => ({ file_id: id(n), path, content_ref: { store_id: id(100), artifact_id: id(n + 100), version_id: id(n + 200) } })
const project = (revision = 2): ProjectOpen => ({ project_id: id(1), revision_id: id(revision), manifest: { name: '测试项目', category: 'video', files: [file(10)], domain_documents: [], asset_refs: [], run_refs: [] } })
function video() {
  const owner = project()
  const raw = { schema_version: 1, kind: 'project-content', category: 'video', video: { cast: [
    { key: 'hero:甲', name: '主角甲', role: 'lead', portrait_file_id: id(10), project_id: id(3), revision_id: id(4) },
    { key: 'local', name: '本地角色', role: 'support' },
  ] } }
  return { owner, raw, content: parseProjectContent(JSON.stringify(raw), owner) }
}

test('direct sources contain only explicit fixed relations and preserve list filters on source navigation', () => {
  const { owner, content } = video(), sources = projectSources(content)
  assert.equal(sources.length, 1)
  assert.equal(sources[0].key, 'project:cast:hero:甲')
  assert.equal(sources[0].revision_id, id(4))
  assert.deepEqual(projectSources(null), [])
  const target = projectSourceRoute(owner, sources[0], { q: '夏天', sort: 'name', project: id(1), reference: 'old', item: 'other', fixed: id(99), candidate: 'pending', scene: 's', version: 'v' })
  assert.equal(target.path, '/projects')
  assert.equal(target.query.category, 'character')
  assert.equal(target.query.project, id(3)); assert.equal(target.query.revision, id(4))
  assert.equal(target.query.fromRevision, owner.revision_id)
  assert.equal(target.query.fromReference, sources[0].key)
  assert.equal(target.query.fromCategory, 'video')
  for (const key of ['reference', 'item', 'fixed', 'candidate', 'scene', 'version']) assert.equal(target.query[key as keyof typeof target.query], undefined)
  assert.equal((target.query as Record<string, unknown>).q, '夏天')
  assert.equal((target.query as Record<string, unknown>).sort, 'name')
})

test('common source list uses the current fixed content and does not substitute an absent source', async () => {
  const { owner, content } = video(), render = renderer(), reader = { project: owner, content }
  const html = await render.render('components/ProjectShared.vue', { reader, tab: 'references' })
  assert.match(html, /直接项目来源/); assert.match(html, /主角甲/)
  assert.doesNotMatch(html, /本地角色/)
  const href = html.match(/href="([^"]+)"/)![1]
  assert.equal(new URL(href, 'https://example.invalid').searchParams.get('reference'), 'project:cast:hero:甲')
  const absent = await render.render('components/ProjectShared.vue', { reader, tab: 'references' }, '/projects?reference=project:cast:missing')
  assert.match(absent, /引用不在当前修订/); assert.doesNotMatch(absent, /打开来源详情/)
})

test('music source pins the chosen song and revision and returns to the owning drama reference', async () => {
  const music = project(4); music.manifest.category = 'music'; music.head_revision_id = id(99)
  const content = parseProjectContent(JSON.stringify({ schema_version: 1, kind: 'project-content', category: 'music', music: { songs: [{ key: 'song_one', title: '开场曲' }] } }), music)
  const fixed = fixedMusicSource(music, content, 'song_one')
  assert.equal(fixed.revision_id, id(4), 'must not pin a different head')
  assert.throws(() => fixedMusicSource(music, content, 'absent'), /没有这首歌曲/)
  assert.throws(() => fixedMusicSource(project(), content, 'song_one'), /没有这首歌曲/)
  const drama = project(5); drama.manifest.category = 'ai_short_drama'
  const raw = { schema_version: 1, kind: 'project-content', category: 'ai_short_drama', ai_short_drama: { episodes: [], globals: [{ key: 'score', kind: 'music', name: '开场配乐', music_source: fixed }] } }
  const parsed = parseProjectContent(JSON.stringify(raw), drama), source = projectSources(parsed)[0]
  assert.equal(source.key, 'project:music:score')
  const target = projectSourceRoute(drama, source, { q: '片名', sort: 'name', material: 'score' })
  assert.equal(target.query.category, 'music'); assert.equal(target.query.tab, 'songs'); assert.equal(target.query.item, 'song_one')
  assert.equal(target.query.fromRevision, drama.revision_id); assert.equal(target.query.fromCategory, 'ai_short_drama')
  assert.equal(target.query.fromReference, source.key)
  assert.equal(sourceReturnCategory(target.query.fromCategory), 'ai_short_drama')
  assert.equal(sourceReturnCategory('unknown'), 'video')
  const list = await renderer().render('components/ProjectShared.vue', { reader: { project: drama, content: parsed }, tab: 'references' })
  assert.match(list, /音乐项目/); assert.match(list, /开场配乐/)
  const before = structuredClone(parsed); before.ai_short_drama!.globals[0].music_source = null
  assert.equal(businessChanges(before, parsed)[0].key, 'global/score/music-source')
  for (const bad of [{ project_id: id(1), song_key: 'song' }, { ...fixed, project_id: 'invalid' }, { ...fixed, song_key: '' }, { ...fixed, head: id(99) }]) {
    assert.throws(() => parseProjectContent(JSON.stringify({ ...raw, ai_short_drama: { globals: [{ ...raw.ai_short_drama.globals[0], music_source: bad }] } }), drama))
  }
  assert.throws(() => parseProjectContent(JSON.stringify({ ...raw, ai_short_drama: { globals: [{ ...raw.ai_short_drama.globals[0], kind: 'prop' }] } }), drama))
})

test('revision changes compare the real parent and retain added, changed, renamed and removed file identities', () => {
  const before = project(2), after = project(3)
  before.manifest.files = [file(10, 'old.png'), file(11), file(12), file(13)]
  after.parent_revision_id = before.revision_id
  after.manifest.files = [file(10, 'new.png'), { ...file(11), content_ref: { ...file(11).content_ref, version_id: id(999) } }, file(13), file(14)]
  before.manifest.asset_refs = [{ asset_id: id(50), asset_version_id: id(51), purpose: '角色' }]
  after.manifest.asset_refs = [{ asset_id: id(50), asset_version_id: id(52), purpose: '角色' }]
  before.manifest.selections = [{ purpose: 'preview', file_ids: [id(10)] }]
  after.manifest.selections = [{ purpose: 'preview', file_ids: [id(14)] }]
  before.manifest.entry_document_id = id(10); after.manifest.entry_document_id = id(14)
  const diff = revisionChanges(before, after)
  assert.deepEqual(diff.files.map(c => c.key), [id(10), id(11), id(12), id(14)])
  assert.equal(diff.files[0].before?.path, 'old.png'); assert.equal(diff.files[0].after?.path, 'new.png')
  assert.equal(diff.files[2].after, undefined); assert.equal(diff.files[3].before, undefined)
  assert.equal(diff.assets.length, 1); assert.equal(diff.assets[0].after?.asset_version_id, id(52))
  assert.equal(diff.selections.length, 1); assert.equal(diff.entry?.after, id(14))
  assert.throws(() => revisionChanges(null, after), /父修订/)
  assert.throws(() => revisionChanges({ ...before, project_id: id(99) }, after), /同一项目/)
  assert.throws(() => revisionChanges({ ...before, revision_id: id(98) }, after), /真实父修订/)
  assert.equal(revisionChanges(null, before).files.length, 4)
})

test('revision file links pin both versions and missing parent never reports added files', async () => {
  const before = project(2), after = project(3), render = renderer()
  after.parent_revision_id = before.revision_id; after.manifest.files[0].path = 'renamed.png'
  const html = await render.render('components/ProjectRevisionDetail.vue', { revision: after, parent: before, historyBusy: false }, `/projects?project=${id(1)}&fixed=${id(3)}&tab=revisions&q=夏天`)
  assert.match(html, /查看变更前/); assert.match(html, /查看变更后/)
  const links = [...html.matchAll(/href="([^"]+)"/g)].map(m => m[1]).filter(h => h.includes('tab=files'))
  assert(links.some(h => h.includes(`revision=${id(2)}`)))
  assert(links.some(h => h.includes(`revision=${id(3)}`)))
  assert(links.every(h => !h.includes('fixed=')))
  const missing = await render.render('components/ProjectRevisionDetail.vue', { revision: after, historyBusy: false })
  assert.match(missing, /父修订暂不可读/); assert.doesNotMatch(missing, /新增|文件变化/)
})

test('business comparison detects exact cast source revisions without rewriting content', () => {
  const { content } = video(), next = structuredClone(content), original = JSON.stringify(content)
  next.video!.cast[0].revision_id = id(8)
  const diff = businessChanges(content, next)
  assert.equal(diff.length, 1); assert.equal(diff[0].key, 'cast/hero:甲')
  assert.equal((diff[0].after!.value as Record<string, unknown>).revision_id, id(8))
  assert.equal(JSON.stringify(content), original)
  assert.deepEqual(businessChanges(content, structuredClone(content)), [])
})

test('business comparison retains per-material, shot and deliverable fixed selections', () => {
  const owner = project(); owner.manifest.category = 'ai_short_drama'
  const before = parseProjectContent(JSON.stringify({ schema_version: 1, kind: 'project-content', category: 'ai_short_drama', ai_short_drama: {
    globals: [{ key: 'score', kind: 'music', name: '开场配乐' }],
    episodes: [{ key: 'ep', title: '第一集', scenes: [{ key: 's', title: '开场', shots: [{ key: 'sh', title: '开场镜头' }] }] }],
    deliverables: [{ key: 'rough', title: '粗剪', file_ids: [] }],
  } }), owner)
  const after = structuredClone(before), drama = after.ai_short_drama!
  drama.globals[0].selected_versions = [{ purpose: 'audio', version_key: 'v1', state_key: null }]
  drama.episodes[0].scenes[0].shots[0].selected_versions = [{ purpose: 'video', version_key: 'v2' }]
  drama.deliverables[0].selected_version_key = 'v3'
  const added = businessChanges(before, after)
  assert.deepEqual(added.map(v => v.key), ['global/score/selected', 'shot/ep/s/sh', 'delivery/rough'])
  assert(added.every(v => !v.before && v.after))
  assert(businessChanges(after, before).every(v => v.before && !v.after))
})

test('cover parsing and save metadata retain legacy fallback, fixed file and crop position', () => {
  const owner = project(); owner.manifest.category = 'character'
  const raw = { schema_version: 1, kind: 'project-content', category: 'character', character: { portrait_file_id: id(10) } }
  const legacy = parseProjectContent(JSON.stringify(raw), owner)
  assert.equal(coverFile(legacy), id(10)); assert.deepEqual(legacy.cover_position, { x: 50, y: 0 })
  const selected = { ...raw, ...coverMetadata(id(10), { x: 0, y: 87.5 }) }
  const saved = parseProjectContent(JSON.stringify(selected), owner)
  assert.deepEqual(saved.cover_position, { x: 0, y: 87.5 }); assert.equal(coverFile(saved), id(10))
  const restored = parseProjectContent(JSON.stringify({ ...raw, ...coverMetadata(null, { x: 50, y: 0 }) }), owner)
  assert.equal(coverFile(restored), null)
  assert.equal(restored.character?.portrait_file_id, id(10), 'restoring placeholder must retain original portrait')
  for (const bad of [{ x: -1, y: 0 }, { x: 50, y: 101 }, { x: '50', y: 0 }, { x: 0 }, { x: 0, y: 0, extra: 1 }, [], 'top']) {
    assert.throws(() => parseProjectContent(JSON.stringify({ ...raw, cover_position: bad }), owner))
  }
  assert.throws(() => parseProjectContent(JSON.stringify({ ...selected, cover_placeholder: true }), owner))
  assert.throws(() => parseProjectContent(JSON.stringify({ ...raw, cover_placeholder: 'false' }), owner))
  assert.throws(() => coverPosition({ x: NaN, y: 0 }))
})

test('cover drag follows real object-fit overflow, clamps and preserves axes without overflow', () => {
  assert.deepEqual(dragCover({ x: 50, y: 20 }, -100, 80, 320, 200, 1000, 200), { x: 64.71, y: 20 })
  assert.deepEqual(dragCover({ x: 50, y: 50 }, 100, -100, 320, 200, 400, 1000), { x: 50, y: 66.67 })
  assert.deepEqual(dragCover({ x: 50, y: 0 }, -10000, 0, 320, 200, 1000, 200), { x: 100, y: 0 })
  assert.deepEqual(dragCover({ x: 50, y: 0 }, 10000, 0, 320, 200, 1000, 200), { x: 0, y: 0 })
  const start = defaultCoverPosition()
  assert.deepEqual(dragCover(start, 40, 40, 320, 200, 640, 400), start)
  assert.deepEqual(dragCover(start, 40, 40, 0, 200, 640, 400), start)
  assert.deepEqual(dragCover(start, NaN, 40, 320, 200, 640, 400), start)
})
