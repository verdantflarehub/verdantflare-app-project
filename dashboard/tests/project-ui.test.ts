import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parseProjectContent } from '../src/platform/project-content.ts'
import { renderer } from './vue-render.ts'

const id = (n: number) => `01900000-0000-7000-8000-${String(n).padStart(12, '0')}`
export function videoReader(empty = false) {
  return { project: { project_id: id(1), revision_id: id(2), manifest: { name: '短视频页面测试' } }, content: { overview: '人物舞蹈与换装', video: {
    output_file_id: null, duration_seconds: 15, aspect: '9:16',
    storyboards: empty ? [] : [{ key: 'a', title: '第一段', overview: '动作说明', duration_seconds: 5, fields: [], reference_file_ids: [], result_file_ids: [], file_ids: [], prompt: '保持人物一致', image_file_id: null }],
    cast: empty ? [] : [{ key: 'hero', name: '主角甲', role: 'lead', project_id: id(3), revision_id: id(4), portrait_file_id: null }, { key: 'partner', name: '配角乙', role: 'support', project_id: null, revision_id: null, portrait_file_id: null }],
  } } }
}
export function dramaReader() {
  const frontend = resolve(dirname(fileURLToPath(import.meta.url)), '..')
  const design = process.env.VERDANTFLARE_DESIGN_ROOT || resolve(frontend, '../..')
  const raw = JSON.parse(readFileSync(resolve(design, 'docs/design/contracts/drafts/examples/ai-short-drama-globals.json'), 'utf8'))
  const project = { project_id: id(1), revision_id: id(2), head_revision_id: id(3), manifest: { name: '短剧页面测试', category: 'ai_short_drama', files: [10, 11, 12].map(n => ({ file_id: id(n), path: `files/${n}.md`, content_ref: { store_id: id(100), artifact_id: id(n + 20), version_id: id(n + 21) } })), domain_documents: [], asset_refs: [], run_refs: [] } }
  const content = parseProjectContent(JSON.stringify(raw), project)
  return { projectID: project.project_id, project, content, download: async () => ({ version: { mime: 'text/markdown' } }), document: async () => ({ text: '固定资料', mime: 'text/markdown' }) }
}

test('video overview and standalone detail render the approved hierarchy with honest missing media', async () => {
  const render = renderer(), reader = videoReader()
  const overview = await render.render('components/video/ShortVideoProject.vue', { reader, tab: 'storyboards' })
  assert(overview.indexOf('成片预览') < overview.indexOf('short-video-boards'))
  assert.match(overview, /<ol[^>]*short-video-board-grid/)
  assert.match(overview, /item=a/)
  assert.match(overview, /01/); assert.match(overview, /5 秒/); assert.match(overview, /未添加视频/)
  const detail = await render.render('components/video/ShortVideoProject.vue', { reader, tab: 'storyboards' }, '/projects?item=a')
  assert(detail.indexOf('概述') < detail.indexOf('参考图'))
  assert(detail.indexOf('参考图') < detail.indexOf('提示词'))
  assert(detail.indexOf('提示词') < detail.indexOf('结果视频'))
  assert.doesNotMatch(detail, /<details[^>]*\bopen\b|business-tabs/)
  assert.match(detail, /<dt[^>]*>时长<\/dt>/)
  assert.match(await render.render('components/video/ShortVideoProject.vue', { reader, tab: 'storyboards' }, '/projects?item=missing'), /没有这项故事板/)
})

test('video cast separates lead and supporting roles and pins character navigation', async () => {
  const render = renderer(), reader = videoReader()
  const html = await render.render('components/video/ShortVideoProject.vue', { reader, tab: 'cast' }, '/projects?category=video&q=summer')
  assert(html.indexOf('主角甲') < html.indexOf('配角乙'))
  assert.match(html, /本片角色：主角/); assert.match(html, /本片角色：配角/)
  assert(html.includes('item=hero')); assert(html.includes('q=summer'))
  const detail = await render.render('components/video/ShortVideoProject.vue', { reader, tab: 'cast' }, '/projects?category=video&q=summer&item=hero')
  for (const value of [id(3), id(4), `from=${id(1)}`, `fromRevision=${id(2)}`, 'q=summer']) assert(detail.includes(value))
  assert.match(detail, /打开来源角色固定版本/)
  const local = await render.render('components/video/ShortVideoProject.vue', { reader, tab: 'cast' }, '/projects?item=partner')
  assert.match(local, /尚未关联来源角色项目/)
  const empty = await render.render('components/video/ShortVideoProject.vue', { reader: videoReader(true), tab: 'cast' })
  assert.match(empty, /未添加主角/); assert.match(empty, /未添加配角/)
})

test('drama missing deep links stay recoverable and material detail returns to its own category', async () => {
  const render = renderer(), reader = dramaReader()
  const missing = await render.render('components/drama/ShortDramaProject.vue', { reader, tab: 'materials' }, '/projects?material=missing&state=missing')
  assert.match(missing, /当前修订没有这项资料/)
  const prop = reader.content.ai_short_drama!.globals.find(g => g.kind === 'prop')!
  const detail = await render.render('components/drama/ShortDramaProject.vue', { reader, tab: 'materials' }, `/projects?material=${prop.key}&kind=character&find=ring&status=ready`)
  assert.match(detail, /返回道具列表/)
  assert.match(detail, /kind=prop/); assert.match(detail, /find=ring/); assert.match(detail, /status=ready/)
  assert.match(detail, /入库到数字世界/)
})

test('drama character identity shows linked looks and preserves source-version ownership', async () => {
  const render = renderer(), reader = dramaReader(), drama = reader.content.ai_short_drama!
  const character = drama.globals.find(g => g.kind === 'character')!, look = drama.globals.find(g => g.kind === 'wardrobe')!
  look.input_global_keys = [character.key]
  const before = JSON.stringify(reader.content)
  const html = await render.render('components/drama/ShortDramaProject.vue', { reader, tab: 'materials' }, `/projects?material=${character.key}`)
  assert.match(html, /剧情造型与状态/); assert(html.includes(look.name)); assert(html.includes(`returnMaterial=${character.key}`))
  const list = await render.render('components/drama/ShortDramaProject.vue', { reader, tab: 'materials' }, '/projects?kind=character')
  assert.match(list, /此历史版本尚未登记制作素材/)
  assert.equal(JSON.stringify(reader.content), before)
})

test('authored Markdown tables render as readable labels and values, with escaped markup', async () => {
  const html = await renderer().render('components/drama/DramaProse.vue', { text: '| 场景 | 灯光 |\n| --- | --- |\n| 船舱 | <script>unsafe</script> |\n\n| 单独内容 | 必须保留 |' })
  assert.match(html, /场景：/); assert.match(html, /灯光：/); assert.match(html, /单独内容/)
  assert.match(html, /&lt;script&gt;/); assert.doesNotMatch(html, /<script>|---/)
})

test('material version comparison excludes a different output purpose without changing selection', async () => {
  const reader = dramaReader()
  const version = (key: string, label: string, purpose: string) => ({ key, label, state_key: 'night', purpose: 'image', segment_key: null, basis_revision_id: id(2), parent_version_key: null, change_note: '版本说明', file_refs: [{ file_id: id(10), purpose, content_ref: reader.project.manifest.files[0].content_ref }], generation_record_file_ids: [], review: { state: 'unreviewed', record_file_id: null } })
  const versions = [version('v1', '图片第一稿', 'image'), version('engine', '工程稿', 'project'), version('v2', '图片第二稿', 'image')]
  const selections = [{ purpose: 'image', version_key: 'v1', state_key: 'night' }]
  const html = await renderer().render('components/drama/DramaVersions.vue', { reader, versions, selections, materialMode: true }, '/projects?version=v2&compare=engine')
  const comparison = html.match(/<select[^>]*>[\s\S]*?<\/select>/g)![1]
  assert.match(comparison, /图片第一稿/); assert.doesNotMatch(comparison, /工程稿/)
  assert.match(html, /剧情状态/); assert.match(html, /未选用候选/)
  assert.equal(selections[0].version_key, 'v1')
})

test('all drama business pages render and scene, lighting and package deep links reject missing identities', async () => {
  const render = renderer(), reader = dramaReader(), drama = reader.content.ai_short_drama!
  for (const tab of ['script', 'character', 'wardrobe', 'prop', 'location', 'continuity', 'sound', 'music', 'production', 'delivery']) {
    const html = await render.render('components/drama/ShortDramaProject.vue', { reader, tab }, `/projects?tab=${tab}`)
    assert.match(html, /workspace-body/, tab)
    assert.doesNotMatch(html, /<nav[^>]*category-rail/, 'business must use the common project menus')
  }
  const location = drama.globals.find(g => g.kind === 'location')!
  assert(location)
  location.states = [{ key: 'night', name: '夜晚', definition_refs: [], marks: [] }]
  const state = location.states[0]
  const lighting = `/projects?tab=location&material=${location.key}&lighting=${state.key}`
  const valid = await render.render('components/drama/ShortDramaProject.vue', { reader, tab: 'location' }, lighting)
  assert(valid.includes(state.name)); assert.match(valid, /环境与灯光/)
  const invalid = await render.render('components/drama/ShortDramaProject.vue', { reader, tab: 'location' }, `/projects?material=${location.key}&lighting=missing&state=${state.key}`)
  assert.match(invalid, /没有此场景状态/); assert.doesNotMatch(invalid, /场景状态依据/)
  const version = await render.render('components/drama/ShortDramaProject.vue', { reader, tab: 'location' }, lighting + '&version=missing')
  assert.match(version, /没有指定的制作版本/)
  const deliverable = drama.deliverables[0]
  assert(deliverable)
  const delivery = await render.render('components/drama/ShortDramaProject.vue', { reader, tab: 'delivery' }, `/projects?deliverable=${deliverable.key}&package=${deliverable.key}`)
  assert.match(delivery, /实际交付记录/)
  const wrongPackage = await render.render('components/drama/ShortDramaProject.vue', { reader, tab: 'delivery' }, `/projects?deliverable=${deliverable.key}&package=missing`)
  assert.match(wrongPackage, /没有此成果或交付包/); assert.doesNotMatch(wrongPackage, /实际交付记录/)
  const missingScene = await render.render('components/drama/ShortDramaProject.vue', { reader, tab: 'script' }, '/projects?scene=missing')
  assert.match(missingScene, /没有此剧本场次/)
})

test('common lists and object details use the fixed manifest and reject absent references', async () => {
  const render = renderer(), reader = dramaReader()
  reader.project.manifest.run_refs = [{ service_id: 'video', run_id: 'native-run' }] as never
  reader.project.manifest.asset_refs = [{ asset_id: id(50), asset_version_id: id(51), purpose: '角色身份' }] as never
  const files = await render.render('components/ProjectShared.vue', { reader, tab: 'files' }, '/projects?find=10&fileType=document')
  assert.match(files, /files\/10.md/); assert.doesNotMatch(files, /files\/11.md/)
  const missingFile = await render.render('components/ProjectShared.vue', { reader, tab: 'files' }, '/projects?file=missing')
  assert.match(missingFile, /文件不在当前项目修订/)
  const runs = await render.render('components/ProjectShared.vue', { reader, tab: 'runs' })
  assert.match(runs, /native-run/)
  const run = await render.render('components/ProjectShared.vue', { reader, tab: 'runs' }, '/projects?run=video:native-run')
  assert.match(run, /原生服务查询/)
  const reference = await render.render('components/ProjectShared.vue', { reader, tab: 'references' }, `/projects?reference=${id(50)}:${id(51)}:${encodeURIComponent('角色身份')}`)
  assert.match(reference, /角色身份/); assert(reference.includes(id(51)))
  const missingReference = await render.render('components/ProjectShared.vue', { reader, tab: 'references' }, '/projects?reference=missing')
  assert.match(missingReference, /资产引用不在当前修订/)
})

test('shot and deliverable versions have single-version details without version switching or comparison', async () => {
  const render = renderer(), reader = dramaReader(), drama = reader.content.ai_short_drama!
  const episode = drama.episodes[0], scene = episode.scenes[0], shot = scene.shots[0], delivery = drama.deliverables[0]
  const version = (key: string, label: string) => ({ key, label, basis_revision_id: id(2), parent_version_key: null, change_note: label + '修改', file_refs: [], generation_record_file_ids: [], review: { state: 'unreviewed' as const, record_file_id: null } })
  shot.versions = [version('v1', '整镜第一版'), version('v2', '整镜第二版')]
  delivery.versions = [version('v1', '成果第一版'), version('v2', '成果第二版')]
  const shotPath = `/projects?tab=production&episode=${episode.key}&scene=${scene.key}&shot=${shot.key}`
  const html = await render.render('components/drama/ShortDramaProject.vue', { reader, tab: 'production' }, shotPath + '&version=v2&compare=v1')
  assert.match(html, /镜头制作版本/); assert.match(html, /整镜第二版/)
  assert.doesNotMatch(html, /整镜第一版|<select|完整版本与修改历史/)
  const missing = await render.render('components/drama/ShortDramaProject.vue', { reader, tab: 'production' }, shotPath + '&version=missing')
  assert.match(missing, /没有此镜头制作版本/)
  const result = await render.render('components/drama/ShortDramaProject.vue', { reader, tab: 'delivery' }, `/projects?tab=delivery&deliverable=${delivery.key}&version=v2`)
  assert.match(result, /成果版本/); assert.match(result, /成果第二版/)
  assert.doesNotMatch(result, /成果第一版|<select|完整版本与修改历史/)
})
