import test from 'node:test'
import assert from 'node:assert/strict'
import { canonicalRoute, legacyStudioTarget, nativeRunTarget, projectTarget, worldTarget, useAssetTarget } from '../src/platform/route-links.ts'
import { dramaBreadcrumb, dramaBrowserTarget, parseDramaPath, rememberDramaListContext } from '../src/platform/drama-route.ts'
const a = '01900000-0000-7000-8000-000000000001', b = '01900000-0000-7000-8000-000000000002'
test('legacy bookmarks preserve the fixed revision, filters and app route', () => {
  const path = `/projects?project=${a}&revision=${b}&category=video&q=summer&tab=cast&item=lead`
  assert.deepEqual(legacyStudioTarget(path), { appId: 'project', path })
  assert.deepEqual(legacyStudioTarget(`/projects?mode=world&asset=${a}&version=${b}&asset_type=music-work`), { appId: 'world', path: `/?asset=${a}&version=${b}&asset_type=music-work` })
  assert.deepEqual(legacyStudioTarget('/projects?view=files'), { appId: 'project', path: '/projects' })
  assert.deepEqual(legacyStudioTarget('/projects?view=files&project=' + a), { appId: 'project', path: '/workspace?project=' + a })
  assert.deepEqual(legacyStudioTarget('/projects?mode=world&view=files&asset=' + a), { appId: 'world', path: '/?asset=' + a })
  assert.equal(canonicalRoute('project', '/use-asset?asset=' + a + '&version=' + b), '/use-asset?asset=' + a + '&version=' + b)
  assert.equal(canonicalRoute('project', '/?category=music'), '/projects?category=music')
  for (const category of ['image', 'character', 'music', 'video', 'creative_video']) assert.equal(canonicalRoute('project', '/' + category), '/projects?category=' + category)
  assert.equal(canonicalRoute('project', '/ai_short_drama'), '/ai_short_drama')
  assert.equal(canonicalRoute('project', `/projects?category=ai_short_drama&project=${a}&revision=${b}&tab=location&material=bath&state=day&find=room`), `/ai_short_drama/${a}/materials/location/bath/day?revision=${b}&q=room`)
  assert.equal(canonicalRoute('project', `/ai_short_drama?project=${a}&revision=${b}&tab=location&material=bath&state=day&returnMaterial=x&compare=v1`), `/ai_short_drama/${a}/materials/location/bath/day?revision=${b}`)
})
test('drama breadcrumbs project 7 levels, include historical version/package/continuity and recall list filters', () => {
  const listView = parseDramaPath(`/ai_short_drama/${a}/materials/location?revision=${b}&q=bath&status=ready`)!
  rememberDramaListContext(listView)
  const detailView = parseDramaPath(`/ai_short_drama/${a}/materials/location/bath/day?revision=${b}&version=v2`)!
  const crumbs = dramaBreadcrumb(detailView, { work: '密室样片', material: '废弃浴室', state: '日景', version: 'V2' })
  assert.deepEqual(crumbs.map(c => c.label), ['项目管理', '短剧制作', '密室样片', '素材', '场景', '废弃浴室', '日景', 'V2'])
  assert.equal(crumbs[4].to.query.q, 'bath')
  assert.equal(crumbs[4].to.query.status, 'ready')
  const pkgView = parseDramaPath(`/ai_short_drama/${a}/delivery/trailer/package?revision=${b}`)!
  assert.deepEqual(dramaBreadcrumb(pkgView, { work: '密室样片', deliverable: '样片预告' }).map(c => c.label), ['项目管理', '短剧制作', '密室样片', '成果与交付', '样片预告', '交付包'])
  const contView = parseDramaPath(`/ai_short_drama/${a}/script/continuity/sh01?revision=${b}`)!
  assert.deepEqual(dramaBreadcrumb(contView, { work: '密室样片', shot: '镜头一' }).map(c => c.label), ['项目管理', '短剧制作', '密室样片', '剧本', '连续性', '镜头一'])
})
test('short-drama project tabs keep the work instead of falling back to character', () => {
  const here = `/ai_short_drama/${a}?revision=${b}`
  const cleared = Object.fromEntries(['item', 'episode', 'scene', 'shot', 'material', 'state', 'version', 'compare', 'deliverable', 'kind', 'find', 'fileType', 'status', 'view', 'purpose', 'segment', 'continuity', 'returnMaterial', 'file', 'fixed', 'run', 'reference', 'candidate', 'lighting', 'package'].map(key => [key, undefined]))
  const location = dramaBrowserTarget(here, { ...cleared, tab: 'location' })
  assert.deepEqual(location, { path: `/ai_short_drama/${a}/materials/location`, query: { revision: b } })
  assert.equal(location?.path.includes('/projects'), false)
  assert.deepEqual(dramaBrowserTarget(here, { ...cleared, tab: 'tasks' }), { path: `/ai_short_drama/${a}/production/shots`, query: { revision: b } })
  assert.deepEqual(dramaBrowserTarget(here, { ...cleared, tab: 'production' }), { path: `/ai_short_drama/${a}/production/shots`, query: { revision: b } })
  assert.deepEqual(dramaBrowserTarget(here, { ...cleared, tab: 'script' }), { path: `/ai_short_drama/${a}/script/body`, query: { revision: b } })
  assert.deepEqual(dramaBrowserTarget(`/ai_short_drama/${a}/materials/location?revision=${b}`, { q: '浴室', shown: undefined }), { path: `/ai_short_drama/${a}/materials/location`, query: { revision: b, q: '浴室' } })
  assert.deepEqual(dramaBrowserTarget(here, { project: undefined, category: 'ai_short_drama', tab: undefined, revision: undefined }), { path: '/ai_short_drama', query: {} })
  assert.equal(dramaBrowserTarget('/projects?category=character&project=' + a, { tab: 'setting' }), undefined)
  assert.deepEqual(dramaBrowserTarget('/ai_short_drama', { project: a, revision: b, tab: undefined }), { path: `/ai_short_drama/${a}`, query: { revision: b } })
})
test('cross application targets contain only app identity and fixed business version', () => {
  assert.deepEqual(projectTarget(a, b), { appId: 'project', path: `/projects?project=${a}&revision=${b}` })
  assert.deepEqual(worldTarget(a, b), { appId: 'world', path: `/?asset=${a}&version=${b}` })
  assert.deepEqual(useAssetTarget(a, b), { appId: 'project', path: `/use-asset?asset=${a}&version=${b}` })
  assert.throws(() => useAssetTarget(a, 'head'))
  assert.throws(() => projectTarget(a, 'head'))
  assert.throws(() => worldTarget(a, 'latest'))
  for (const path of ['//outside.example/projects', '/projects?project=x&project=y', '/projects#other', '/projects/../world']) assert.throws(() => canonicalRoute('project', path))
})
test('native runs keep original IDs and only expose verified media task routes', () => {
  assert.deepEqual(nativeRunTarget('video', 'video_task_original'), { appId: 'video-mcp', path: '/dashboard/tasks/video_task_original' })
  assert.deepEqual(nativeRunTarget('image', 'imgtask-original'), { appId: 'image-mcp', path: '/dashboard/tasks/imgtask-original' })
  for (const [service, id] of [['music', 'unknown'], ['video', '../new'], ['image', 'id?submit=true'], ['video', '']]) assert.equal(nativeRunTarget(service, id), null)
})
