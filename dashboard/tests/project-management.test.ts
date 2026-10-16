import test from 'node:test'
import assert from 'node:assert/strict'
import { collectProjects, selectProjects, businessKey, projectCategories, projectMenus, restoredListView } from '../src/platform/project-management.ts'
import { createProjectEditor, hasProjectEditRecovery, editHash, orderedEdit, verifyEditReadback, type EditRequest } from '../src/platform/project-edit.ts'
import { readProjectHistory } from '../src/platform/project-revisions.ts'
import type { ProjectOpen } from '../src/platform/project-content.ts'
import { projectContentView, parseProjectContent } from '../src/platform/project-content.ts'
import type { AppSDK, Json, Lifecycle } from '../src/platform/types.ts'

const id = (n: number) => `0199b501-0000-7000-8000-${n.toString(16).padStart(12, '0')}`
test('new projects expose empty typed business views without creating or replacing their documents', () => {
  for (const category of ['character', 'music', 'video', 'ai_short_drama']) {
    const project: ProjectOpen = { project_id: id(1), revision_id: id(2), manifest: { name: '新项目', category, files: [], domain_documents: [] } }
    const original = structuredClone(project)
    const view = projectContentView(project, null)!
    assert.equal(view.category, category)
    assert.equal(view.cover_file_id, null)
    assert.equal(view.overview, null)
    if (category === 'character') assert.deepEqual(view.character?.extensions, [])
    if (category === 'video') assert.deepEqual(view.video?.cast, [])
    if (category === 'music') assert.deepEqual(view.music?.voice.models, [])
    if (category === 'ai_short_drama') assert.deepEqual(view.ai_short_drama?.episodes, [])
    assert.deepEqual(project, original)
    const saved = parseProjectContent(JSON.stringify({ schema_version: 1, kind: 'project-content', category, overview: '实际文档', [category]: category === 'ai_short_drama' ? { episodes: [] } : {} }), project)
    assert.equal(projectContentView(project, saved), saved)
  }
  assert.equal(projectContentView(null, null), null)
})
test('search, count and ordering use later pages; cancellation and cyclic cursors fail closed', async () => {
  const calls: unknown[] = []
  const all = await collectProjects('character', async args => {
    calls.push(args)
    return args.cursor ? { items: [{ project_id: id(2), head_revision_id: id(12), category: 'character', name: '后页匹配', updated_at: '2026-10-15T00:00:00Z' }], next_cursor: '' } : { items: [{ project_id: id(1), head_revision_id: id(11), category: 'character', name: '首项', updated_at: '2026-10-14T00:00:00Z' }], next_cursor: id(1) }
  }, () => true)
  assert.equal(calls.length, 2); assert.equal(all.length, 2)
  assert.equal(selectProjects(all, '匹配', 'updated')[0].project_id, id(2))
  assert.equal(selectProjects(all, '', 'updated')[0].project_id, id(2))
  await assert.rejects(collectProjects('character', async () => ({ items: [], next_cursor: id(1) }), () => true), /分页没有继续/)
  let active = true
  await assert.rejects(collectProjects('music', async () => { active = false; return { items: [] } }, () => active), /查询已取消/)
  await assert.rejects(collectProjects('music', async () => ({ items: all }), () => true), /返回内容不正确/)
})
test('business menus preserve old links while exposing exactly ten drama and four shared menus', () => {
  assert.equal(projectMenus('ai_short_drama').length, 14)
  assert.equal(businessKey('ai_short_drama', 'overview'), 'script')
  assert.equal(businessKey('ai_short_drama', 'materials', 'prop'), 'prop')
  assert.equal(businessKey('ai_short_drama', 'script', 'continuity'), 'continuity')
  assert.equal(businessKey('ai_short_drama', '', ''), 'script')
  assert(projectCategories.some(category => category.key === 'creative_video'))
  assert.deepEqual(projectMenus('creative_video').slice(0, 6).map(menu => menu.key), ['overview', 'brief', 'story', 'shots', 'edit', 'delivery'])
})
test('host category returns restore list context while clearing search within a list stays clear', () => {
  const memory = new Map([['character', { q: '角色', sort: 'name', shown: '36' }]])
  assert.deepEqual(restoredListView({ category: 'character' }, { category: 'character', project: id(1) }, memory), memory.get('character'))
  assert.deepEqual(restoredListView({ category: 'character' }, { category: 'music' }, memory), memory.get('character'))
  assert.equal(restoredListView({ category: 'character' }, { category: 'character', q: '角色' }, memory), undefined)
  assert.equal(restoredListView({ category: 'character', q: '新搜索' }, { category: 'music' }, memory), undefined)
})
function fixture() {
  const records = new Map<string, string>(), calls: { path: string; body: Json }[] = []
  let lose = true, lifecycle: ((event: Lifecycle) => void) | undefined
  const context = { userId: id(1), organizationId: id(2), stationId: id(3), projectId: null, appId: 'project', installationId: id(4), contextRevision: '1' }
  const sdk = {
    context: { get: () => context, subscribe: () => () => {} },
    lifecycle: { subscribe: (fn: typeof lifecycle) => { lifecycle = fn; return () => {} } },
    app: { request: async (input: { path: string; body: Json }) => {
      calls.push(input)
      const saved = JSON.parse([...records.values()][0]) as { request: EditRequest; requestHash: string }
      const result = { project_id: id(10), revision_id: id(12), manifest: { project_id: id(10) } }
      if (input.path.endsWith('/edit')) {
        assert.deepEqual(input.body, saved.request)
        if (lose) { lose = false; throw Error('Lost response') }
        return { status: 200, data: result }
      }
      return { status: 200, data: { state: 'committed', request_sha256: await editHash(saved.request), result } }
    } },
  } as unknown as AppSDK
  const storage = { getItem: (key: string) => records.get(key) ?? null, setItem: (key: string, value: string) => { records.set(key, value) }, removeItem: (key: string) => { records.delete(key) } }
  return { sdk, storage, records, calls, hidden: () => lifecycle?.('hidden') }
}
test('editing journals the exact request before sending and recovers a lost response after reopening', async () => {
  const f = fixture(), editor = createProjectEditor(f.sdk, f.storage, id(10))
  await editor.begin(id(11), { set_metadata: { name: '项目<&>' } })
  const original = editor.pending()
  assert.equal(hasProjectEditRecovery(f.sdk.context.get(), f.storage, id(10)), true)
  assert.equal(hasProjectEditRecovery(f.sdk.context.get(), f.storage, id(11)), false)
  assert.equal(hasProjectEditRecovery({ ...f.sdk.context.get(), userId: id(99) }, f.storage, id(10)), false)
  await assert.rejects(editor.submit(), /Lost response/)
  await assert.rejects(editor.begin(id(11), { set_metadata: { name: 'changed' } }), /原保存/)
  editor.dispose()
  const reopened = createProjectEditor(f.sdk, f.storage, id(10))
  assert.deepEqual(reopened.pending(), original)
  const status = await reopened.status()
  assert.equal(status.revision, id(12)); assert.equal(status.state, 'committed')
  await reopened.clearTerminal()
  assert.equal(reopened.pending(), undefined)
  assert.equal(hasProjectEditRecovery(f.sdk.context.get(), f.storage, id(10)), false)
  assert.equal(f.records.size, 1, 'terminal request is archived')
  reopened.dispose()
})
test('editing rejects changed journals, hidden submissions and out-of-scope changes', async () => {
  const f = fixture(), editor = createProjectEditor(f.sdk, f.storage, id(10))
  await editor.begin(id(11), { set_metadata: { name: 'original' } })
  const key = [...f.records.keys()][0], changed = JSON.parse(f.records.get(key)!)
  changed.request.changes.set_metadata.name = 'tampered'; f.records.set(key, JSON.stringify(changed))
  await assert.rejects(editor.submit(), /原请求不一致/)
  assert.equal(f.calls.length, 0); editor.dispose()
  assert.throws(() => orderedEdit({ project_id: id(10), expected_revision_id: id(11), commit_id: id(13), changes: { set_asset_refs: [] } } as unknown as EditRequest), /保存请求不完整/)
  const h = fixture(), hidden = createProjectEditor(h.sdk, h.storage, id(10))
  await hidden.begin(id(11), { set_metadata: { name: 'original' } }); h.hidden()
  await assert.rejects(hidden.submit(), /页面已隐藏/); assert.equal(h.calls.length, 0); hidden.dispose()
})

test('recovered edits verify original changes independently of the reopened dialog', async () => {
  const request = { project_id: id(10), expected_revision_id: id(11), commit_id: id(13), changes: { set_metadata: { name: '原保存名称' } } }
  const project = { project_id: id(10), manifest: { name: '原保存名称', domain_documents: [{ document_type: 'project-content', file_id: id(20) }] } } as ProjectOpen
  const unused = async () => { throw Error('rename must not read documents') }
  await verifyEditReadback(request, project, unused)
  await assert.rejects(verifyEditReadback(request, { ...project, manifest: { ...project.manifest, name: '旧名称' } }, unused), /名称保存回读/)
  const contentRequest: EditRequest = { ...request, changes: { upsert_files: [{ file_id: id(20), path: 'project-content.json', role: 'domain', text: '{"cover_file_id":null}', mime: 'application/json' }] } }
  await verifyEditReadback(contentRequest, project, async file => { assert.equal(file, id(20)); return { text: contentRequest.changes.upsert_files![0].text } })
  await assert.rejects(verifyEditReadback(contentRequest, project, async () => ({ text: 'stale' })), /业务内容保存回读/)
})

test('revision history follows real parents, rejects foreign or malformed identities, and cancels late responses', async () => {
  const revision = (n: number, parent: string) => ({ project_id: id(10), revision_id: id(n), parent_revision_id: parent, manifest: { project_id: id(10), name: '项目', files: [] } }) as unknown as ProjectOpen
  const calls: string[] = [], seen: ProjectOpen[] = []
  await readProjectHistory(id(10), id(12), async key => { calls.push(key); return key === id(12) ? revision(12, id(11)) : revision(11, '') }, () => true, r => seen.push(r))
  assert.deepEqual(calls, [id(12), id(11)]); assert.equal(seen.length, 2)
  calls.length = 0
  await assert.rejects(readProjectHistory(id(10), id(12), async key => { calls.push(key); return revision(12, 'invalid') }, () => true, () => {}), /父修订/)
  assert.deepEqual(calls, [id(12)], 'invalid parents must never be requested')
  await assert.rejects(readProjectHistory(id(10), id(12), async () => revision(12, id(12)), () => true, () => {}), /修订关系/)
  await assert.rejects(readProjectHistory(id(10), id(12), async () => ({ ...revision(12, ''), project_id: id(99) }), () => true, () => {}), /不匹配/)
  let active = true, appended = false
  await readProjectHistory(id(10), id(12), async () => { active = false; return revision(12, '') }, () => active, () => { appended = true })
  assert.equal(appended, false)
})
