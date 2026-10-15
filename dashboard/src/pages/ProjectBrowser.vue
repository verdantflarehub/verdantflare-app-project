<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, triggerRef, watch } from 'vue'
import { onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { callTool, MCPRequestError } from '../platform/mcp'
import { isProjectID, ProjectReader, readerError, projectContentView, type Detail, type ProjectItem } from '../platform/project-reader'
import ProjectMedia from '../components/ProjectMedia.vue'
import ProjectPrompt from '../components/ProjectPrompt.vue'
import ProjectCover from '../components/ProjectCover.vue'
import { useHost } from '../platform/host'
import ShortDramaProject from '../components/drama/ShortDramaProject.vue'
import ShortVideoProject from '../components/video/ShortVideoProject.vue'
import { projectCategories, isCategory, projectMenus, businessKey, commonMenus, collectProjects, selectProjects, restoredListView } from '../platform/project-management'
import ProjectShared from '../components/ProjectShared.vue'
import CreateProjectPage from './CreateProjectPage.vue'
import ProjectEditDialog from '../components/ProjectEditDialog.vue'
import { hasProjectEditRecovery } from '../platform/project-edit'

const route = useRoute(), router = useRouter()
const { appSDK, dashboardHost } = useHost()
const query = (key: string) => typeof route.query[key] === 'string' ? route.query[key] as string : ''
const projectID = computed(() => query('project')), revisionID = computed(() => query('revision'))
const filter = computed(() => isCategory(query('category')) ? query('category') : 'character'), search = computed(() => query('q'))
const sort = computed(() => query('sort') === 'name' ? 'name' : 'updated')
const categories = projectCategories
const categoryName = (key: string) => categories.find(c => c.key === key)?.label || '项目'
const listCategory = computed(() => categories.find(c => c.key === filter.value)!)
const visibleCount = computed(() => Math.max(12, Math.min(100000, Number(query('shown')) || 12)))
const creating = ref(false), createDialog = ref<HTMLDialogElement>()
watch(creating, async open => { await nextTick(); if (open) createDialog.value?.showModal(); else createDialog.value?.close() })
const editingReader = shallowRef<ProjectReader>(), editingMode = ref<'rename' | 'cover' | 'extension'>('rename'), editLoading = ref(false), editRecovery = ref(false)
function pendingEdit(id: string) { try { return hasProjectEditRecovery(appSDK.context.get(), window.localStorage, id) } catch { return true } }
let editGeneration = 0
async function editProject(item: ProjectItem, mode: 'rename' | 'cover' | 'extension') {
  const token = ++editGeneration; editingReader.value?.dispose(); editingReader.value = undefined; editLoading.value = true; listError.value = ''
  const next = new ProjectReader(item.project_id, item.head_revision_id)
  try { await next.load(); if (!alive || token !== editGeneration) { next.dispose(); return }; if (!next.project?.can_write && !pendingEdit(item.project_id)) throw new Error('当前身份没有编辑权限。'); editingMode.value = mode; editingReader.value = next }
  catch (cause) { next.dispose(); if (alive && token === editGeneration) listError.value = readerError(cause) }
  finally { if (alive && token === editGeneration) editLoading.value = false }
}
function closeEdit() { editGeneration++; editingReader.value?.dispose(); editingReader.value = undefined; editRecovery.value = !!projectID.value && pendingEdit(projectID.value) }
function savedEdit(revision: string) { closeEdit(); listCache.clear(); if (projectID.value) void router.replace(link({ revision })); else void loadList() }
function editCurrent(mode: 'rename' | 'cover' | 'extension') { if (project.value) void editProject({ project_id: project.value.project_id, head_revision_id: project.value.revision_id, name: project.value.manifest.name, category: project.value.manifest.category }, mode) }
const listCache = new Map<string, ProjectItem[]>()
const reader = shallowRef<ProjectReader | null>(null), loading = ref(false), error = ref('')
const items = ref<ProjectItem[]>([]), listLoading = ref(false), listError = ref(''), listReady = ref(false)
let request = 0, listRequest = 0, alive = true, signedOut = false
const project = computed(() => reader.value?.project), content = computed(() => projectContentView(project.value || null, reader.value?.content || null))
const historical = computed(() => !!project.value?.head_revision_id && project.value.revision_id !== project.value.head_revision_id)
const character = computed(() => content.value?.character), music = computed(() => content.value?.music), video = computed(() => content.value?.video), drama = computed(() => content.value?.ai_short_drama)
const tabs = computed(() => projectMenus(project.value?.manifest.category || ''))
const activeTab = computed(() => tabs.value.find(t => t.key === businessKey(project.value?.manifest.category || '', query('tab'), query('kind'))) || tabs.value[0])
const objectDetail = computed(() => ['item', 'material', 'shot', 'scene', 'continuity', 'deliverable', 'file', 'fixed', 'run', 'reference', 'candidate', 'lighting', 'package'].some(key => !!query(key)))
const shared = computed(() => commonMenus.some(t => t.key === activeTab.value?.key))
const dramaMaterial = computed(() => drama.value?.globals.find(g => g.key === query('material')))
const dramaKindName = computed(() => ({ script: '剧本正文', character: '人物卡片', wardrobe: '服装造型', prop: '道具', location: '场景', sound: '声音', music: '配乐' }[dramaMaterial.value?.kind || 'character']))
const dramaEpisode = computed(() => query('episode') ? drama.value?.episodes.find(ep => ep.key === query('episode')) : drama.value?.episodes[0])
const dramaScene = computed(() => dramaEpisode.value?.scenes.find(scene => scene.key === query('scene')))
const dramaShot = computed(() => dramaScene.value?.shots.find(shot => shot.key === query('shot')))
const entries = computed<Detail[]>(() => {
  switch (activeTab.value?.key) {
    case 'voice': return music.value?.voice.models || []
    case 'extensions': return character.value?.extensions || []
    case 'songs': return music.value?.songs || []
    case 'mvs': return music.value?.mvs || []
    case 'storyboards': return video.value?.storyboards || []
    default: return []
  }
})
const visibleEntries = computed(() => entries.value.filter(e => (e.title + ' ' + (e.overview || '')).toLocaleLowerCase().includes(query('find').trim().toLocaleLowerCase())))
const detail = computed(() => entries.value.find(d => d.key === query('item')))
const objectTitle = computed(() => query('file') ? project.value?.manifest.files.find(f => f.file_id === query('file'))?.path || '文件详情'
  : query('fixed') ? '固定修订详情' : query('run') ? '原生运行详情' : query('reference') ? '固定资产引用' : query('candidate') ? '入库候选'
  : query('lighting') ? '环境与灯光' : query('package') ? '交付包'
  : query('material') ? dramaMaterial.value?.name || '资料详情'
  : query('shot') ? dramaShot.value?.versions.find(v => v.key === query('version'))?.label || dramaShot.value?.title || '镜头详情'
  : query('scene') ? dramaScene.value?.title || '场次详情'
  : query('deliverable') ? drama.value?.deliverables.find(d => d.key === query('deliverable'))?.title || '成果详情'
  : query('continuity') ? '连续性详情'
  : query('item') && activeTab.value?.key === 'cast' ? video.value?.cast.find(c => c.key === query('item'))?.name || '片中角色详情'
  : detail.value?.title)
const previousPage = computed(() => {
  void route.fullPath
  const back = router.options.history.state.back
  if (typeof back !== 'string' || !back.startsWith('/projects?')) return null
  const previous = router.resolve(back)
  return previous.query.project === projectID.value ? previous : null
})

const filtered = computed(() => selectProjects(items.value, search.value, sort.value))
const shown = computed(() => filtered.value.slice(0, visibleCount.value))
const displayTime = (time?: string) => time && Number.isFinite(Date.parse(time)) ? new Date(time).toLocaleString('zh-CN') : '更新时间未知'
const resultKind = computed(() => activeTab.value?.key === 'extensions' ? 'image' : ['songs', 'voice'].includes(activeTab.value?.key || '') ? 'audio' : 'video')
const fromProject = computed(() => isProjectID(query('from')) && isProjectID(query('fromRevision')) ? { id: query('from'), revision: query('fromRevision'), name: query('fromName') || '创意视频' } : null)
const value = (text?: string | null) => text?.trim() ? text : '未填写'
const duration = (seconds?: number | null) => seconds == null ? '时长未填写' : `${seconds} 秒`
function link(change: Record<string, string | undefined>) { return { path: '/projects', query: { ...route.query, publish: undefined, publication: undefined, ...change } } }
const listLink = computed(() => link({ category: project.value?.manifest.category || filter.value, file: undefined, fixed: undefined, run: undefined, reference: undefined, candidate: undefined, lighting: undefined, package: undefined, project: undefined, revision: undefined, tab: undefined, item: undefined, episode: undefined, scene: undefined, shot: undefined, from: undefined, fromRevision: undefined, fromName: undefined, material: undefined, state: undefined, version: undefined, compare: undefined, deliverable: undefined, kind: undefined, find: undefined, fileType: undefined, status: undefined, view: undefined, purpose: undefined, segment: undefined, continuity: undefined, returnMaterial: undefined }))
function openProject(item: ProjectItem) { return link({ file: undefined, fixed: undefined, run: undefined, reference: undefined, candidate: undefined, lighting: undefined, package: undefined, project: item.project_id, revision: item.head_revision_id, tab: undefined, item: undefined, episode: undefined, scene: undefined, shot: undefined, material: undefined, state: undefined, version: undefined, compare: undefined, deliverable: undefined, kind: undefined, find: undefined, fileType: undefined, status: undefined, view: undefined, purpose: undefined, segment: undefined, continuity: undefined, returnMaterial: undefined }) }
const positions = new Map<string, number>()
const listViews = new Map<string, { q?: string; sort?: string; shown?: string }>()
const businessViews = new Map<string, Record<string, string>>()
const businessViewKeys = ['item', 'episode', 'scene', 'shot', 'material', 'state', 'version', 'compare', 'deliverable', 'kind', 'find', 'fileType', 'status', 'view', 'purpose', 'segment', 'continuity', 'returnMaterial', 'file', 'fixed', 'run', 'reference', 'candidate', 'lighting', 'package']
function businessViewKey(query: Record<string, unknown>, tab: string) { return JSON.stringify([query.project, query.revision, tab]) }
function menuLink(tab: string) {
  return link({ ...Object.fromEntries(businessViewKeys.map(key => [key, undefined])), tab, ...(businessViews.get(businessViewKey(route.query, tab)) || {}) })
}
const scope = appSDK.context.get()
const scrollStorageKey = `project.scroll:${JSON.stringify([scope.userId, scope.organizationId, scope.stationId, scope.installationId, scope.contextRevision])}`
const viewsKey = scrollStorageKey + ':views'
const businessViewsKey = scrollStorageKey + ':business'
try { const stored: unknown = JSON.parse(sessionStorage.getItem(businessViewsKey) || '[]'); if (Array.isArray(stored)) for (const entry of stored.slice(-80)) { if (!Array.isArray(entry) || typeof entry[0] !== 'string' || entry[0].length > 512 || !entry[1] || typeof entry[1] !== 'object') continue; const view = Object.fromEntries(Object.entries(entry[1]).filter(([key, value]) => businessViewKeys.includes(key) && typeof value === 'string' && value.length <= 4096)) as Record<string, string>; businessViews.set(entry[0], view) } } catch { /* Keep in memory. */ }
try { const stored = JSON.parse(sessionStorage.getItem(viewsKey) || '{}'); for (const [category, view] of Object.entries(stored)) { if (isCategory(category) && view && typeof view === 'object') { const v = view as Record<string, unknown>; listViews.set(category, { q: typeof v.q === 'string' ? v.q.slice(0, 256) : undefined, sort: v.sort === 'name' ? 'name' : undefined, shown: typeof v.shown === 'string' && /^\d{1,6}$/.test(v.shown) ? v.shown : undefined }) } } } catch { /* Storage is optional. */ }
try {
  const stored: unknown = JSON.parse(sessionStorage.getItem(scrollStorageKey) || '[]')
  if (Array.isArray(stored)) for (const entry of stored.slice(-60)) {
    if (Array.isArray(entry) && typeof entry[0] === 'string' && entry[0].length <= 4096 && typeof entry[1] === 'number' && Number.isFinite(entry[1]) && entry[1] >= 0) positions.set(entry[0], entry[1])
  }
} catch { /* Navigation works without browser storage. */ }
onBeforeRouteUpdate((to, from) => {
  if (!from.query.project && isCategory(String(from.query.category))) { listViews.set(String(from.query.category), { q: typeof from.query.q === 'string' ? from.query.q : undefined, sort: typeof from.query.sort === 'string' ? from.query.sort : undefined, shown: typeof from.query.shown === 'string' ? from.query.shown : undefined }); try { sessionStorage.setItem(viewsKey, JSON.stringify(Object.fromEntries(listViews))); sessionStorage.setItem(viewsKey + ':last', String(from.query.category)) } catch { /* Keep in memory. */ } }
  if (signedOut) return
  if (typeof from.query.project === 'string' && typeof from.query.revision === 'string') {
    const key = businessViewKey(from.query, businessKey(String(from.query.category), String(from.query.tab || ''), String(from.query.kind || '')))
    businessViews.delete(key); businessViews.set(key, Object.fromEntries(businessViewKeys.flatMap(key => typeof from.query[key] === 'string' ? [[key, from.query[key]]] : [])) as Record<string, string>)
    while (businessViews.size > 80) businessViews.delete(businessViews.keys().next().value!)
    try { sessionStorage.setItem(businessViewsKey, JSON.stringify([...businessViews])) } catch { /* Keep in memory. */ }
  }
  positions.delete(from.fullPath); positions.set(from.fullPath, dashboardHost.getScroll())
  while (positions.size > 60) positions.delete(positions.keys().next().value!)
  try { sessionStorage.setItem(scrollStorageKey, JSON.stringify([...positions])) } catch { /* Keep in-memory positions. */ }
  const view = restoredListView(to.query, from.query, listViews)
  if (view) return { path: to.path, query: { ...to.query, ...view } }
})
async function restoreScroll() { await nextTick(); if (alive) dashboardHost.restoreScroll(positions.get(route.fullPath) || 0) }
async function loadList() {
  if (signedOut) return
  const current = ++listRequest, category = filter.value
  if (!isCategory(category)) return
  listLoading.value = true; listError.value = ''
  const cached = listCache.get(category)
  items.value = cached || []; listReady.value = !!cached
  try {
    const all = await collectProjects(category, args => callTool('project.list', args), () => alive && !signedOut && current === listRequest)
    if (!alive || current !== listRequest) return
    items.value = all; listCache.set(category, all); listReady.value = true
  } catch (cause) { if (alive && current === listRequest) { if (cause instanceof MCPRequestError && [401, 403].includes(cause.status)) { items.value = []; listCache.delete(category); listReady.value = false }; listError.value = readerError(cause) } }
  finally { if (alive && current === listRequest) { listLoading.value = false; void restoreScroll() } }
}
async function loadProject() {
  const current = ++request
  reader.value?.dispose(); reader.value = null; error.value = ''; loading.value = false; editRecovery.value = false
  if (signedOut) return
  if (!projectID.value) { if (!listReady.value && !listLoading.value) void loadList(); return }
  loading.value = true
  const next = new ProjectReader(projectID.value, revisionID.value || undefined)
  reader.value = next
  try {
    await next.load()
    if (!alive || current !== request) return
    triggerRef(reader)
    editRecovery.value = pendingEdit(projectID.value)
    // Pin an unversioned deep link once; subsequent refreshes keep this revision.
    if (!revisionID.value || query('category') !== next.project!.manifest.category) await router.replace(link({ revision: next.project!.revision_id, category: next.project!.manifest.category }))
  } catch (cause) { if (alive && current === request) { next.dispose(); reader.value = null; error.value = readerError(cause) } }
  finally { if (alive && current === request) { loading.value = false; void restoreScroll() } }
}
function logout() {
  signedOut = true; request++; listRequest++; reader.value?.dispose(); reader.value = null
  closeEdit(); items.value = []; listReady.value = false; loading.value = false; listLoading.value = false
  positions.clear(); listCache.clear(); listViews.clear(); businessViews.clear()
  try { sessionStorage.removeItem(scrollStorageKey); sessionStorage.removeItem(viewsKey); sessionStorage.removeItem(viewsKey + ':last'); sessionStorage.removeItem(businessViewsKey) } catch { /* Storage can be disabled. */ }
  error.value = '当前会话已退出，请登录后刷新。'; listError.value = error.value
}
if (!projectID.value && !isCategory(query('category'))) { let last = 'character'; try { const stored = sessionStorage.getItem(viewsKey + ':last') || ''; if (isCategory(stored)) last = stored } catch { /* Use first category. */ } void router.replace(link({ category: last, ...(listViews.get(last) || {}) })) }
function retry() { signedOut = false; if (projectID.value) void loadProject(); else void loadList() }
watch([projectID, revisionID], () => void loadProject(), { immediate: true })
watch(filter, () => { listReady.value = false; if (!projectID.value) void loadList() })
watch(() => route.fullPath, () => { if (!loading.value && !listLoading.value) void restoreScroll() })
const stopContext = appSDK.context.subscribe(next => { if (JSON.stringify([next.userId, next.organizationId, next.stationId, next.installationId, next.contextRevision]) !== JSON.stringify([scope.userId, scope.organizationId, scope.stationId, scope.installationId, scope.contextRevision])) logout() })
const unsubscribe = appSDK.lifecycle.subscribe(event => {
  if (event === 'hidden' || event === 'dispose') {
    if (!signedOut) { positions.set(route.fullPath, dashboardHost.getScroll()); try { sessionStorage.setItem(scrollStorageKey, JSON.stringify([...positions])) } catch { /* Keep in memory. */ } }
  }
  if (event === 'dispose') logout()
  if (event === 'hidden') { editGeneration++; editingReader.value?.dispose(); editingReader.value = undefined; editLoading.value = false; request++; listRequest++; reader.value?.dispose(); reader.value = null; loading.value = false; listLoading.value = false }
  if (event === 'active') { if (projectID.value) void loadProject(); else void loadList() }
})
onBeforeUnmount(() => { closeEdit(); alive = false; request++; listRequest++; reader.value?.dispose(); unsubscribe(); stopContext() })
</script>

<template>
  <main class="main project-browser" :class="{ 'drama-browser': drama, 'video-browser': video }">
    <div class="workspace">
      <nav v-if="projectID" class="breadcrumbs" aria-label="面包屑">
        <RouterLink :to="listLink">项目管理</RouterLink><span>/</span>
        <template v-if="fromProject"><RouterLink :to="link({ project: fromProject.id, revision: fromProject.revision, tab: 'cast', item: undefined, from: undefined, fromRevision: undefined, fromName: undefined })">{{ fromProject.name }}</RouterLink><span>/</span></template>
        <RouterLink :to="{ ...listLink, query: { ...listLink.query, category: project?.manifest.category || filter || undefined } }">{{ categoryName(project?.manifest.category || filter) }}</RouterLink><span>/</span>
        <template v-if="objectDetail"><RouterLink :to="link({ item: undefined, material: undefined, shot: undefined, scene: undefined, continuity: undefined, deliverable: undefined, file: undefined, fixed: undefined, run: undefined, reference: undefined, candidate: undefined, lighting: undefined, package: undefined, state: undefined, version: undefined, compare: undefined, tab: undefined, kind: undefined })">{{ project?.manifest.name || '项目' }}</RouterLink><span>/</span><RouterLink :to="link({ item: undefined, material: undefined, shot: undefined, scene: undefined, continuity: undefined, deliverable: undefined, file: undefined, fixed: undefined, run: undefined, reference: undefined, candidate: undefined, lighting: undefined, package: undefined, state: undefined, version: undefined, compare: undefined })">{{ activeTab?.label }}</RouterLink><span>/</span><span aria-current="page">{{ objectTitle || '详情' }}</span></template>
        <span v-else aria-current="page">{{ project?.manifest.name || '项目' }}</span>
      </nav>
      <header class="heading">
        <div class="heading-left"><div class="eyebrow">PROJECTS</div><h1>{{ projectID ? (objectTitle || project?.manifest.name || '项目') : listCategory.label }}</h1><p v-if="drama">{{ drama.episodes.length }} 集 · {{ drama.episodes.reduce((n, e) => n + e.scenes.reduce((n, s) => n + s.shots.length, 0), 0) }} 镜 · {{ activeTab?.label }}</p><p v-else-if="!projectID">{{ listCategory.description }}</p></div>
        <div class="reference-row"><button v-if="!projectID" class="btn primary" @click="creating = true">＋ 新建项目</button><button v-if="project?.can_write && !historical" class="btn" @click="editCurrent('rename')">重命名</button><button v-if="project?.can_write && !historical" class="btn" @click="editCurrent('cover')">修改封面</button><RouterLink v-if="project" class="btn" :to="{ path: '/workspace', query: { project: projectID } }">本机工作副本</RouterLink><button class="btn" :disabled="loading || listLoading" @click="retry">刷新</button></div>
      </header>

      <div v-if="project && editRecovery" class="revision-notice"><p>有一项原保存结果待确认，可在此固定修订继续查询和恢复。</p><button class="btn" :disabled="editLoading" @click="editCurrent('rename')">查询或恢复原保存</button></div>
      <template v-if="!projectID">
        <div class="list-tools">
          <input :value="search" type="search" aria-label="搜索项目名称" placeholder="搜索项目名称" @input="router.replace(link({ q: ($event.target as HTMLInputElement).value || undefined, shown: undefined }))">
          <select aria-label="项目排序" :value="sort" @change="router.replace(link({ sort: ($event.target as HTMLSelectElement).value, shown: undefined }))"><option value="updated">最近更新</option><option value="name">名称</option></select>
          <span class="muted" role="status">{{ listLoading ? '正在读取完整项目目录…' : listReady ? '共 ' + filtered.length + ' 个项目' : '' }}</span>
        </div>
        <div v-if="listError" class="state error" role="alert">{{ listError }}<button class="btn" @click="retry">重试</button></div>
        <div v-if="listLoading && !items.length" class="state" role="status">正在读取项目…</div>
        <div v-else-if="!filtered.length && !listError && !listLoading" class="state">{{ search ? '没有匹配的项目。' : '暂无' + listCategory.label + '项目。' }}</div>
        <div class="cards">
          <article v-for="item in shown" :key="item.project_id" class="project-card-wrap"><RouterLink :key="`${item.project_id}:${item.head_revision_id}`" class="project-card" :to="openProject(item)">
            <ProjectCover :project="item" /><div class="card-text"><small>{{ categoryName(item.category) }}</small><h2 :title="item.name">{{ item.name }}</h2><small>更新于 {{ displayTime(item.updated_at) }}</small></div>
          </RouterLink><details v-if="item.can_write" class="card-menu"><summary :aria-label="item.name + ' 的操作'">···</summary><button :disabled="editLoading" @click="editProject(item, 'rename')">重命名</button><button :disabled="editLoading" @click="editProject(item, 'cover')">修改封面</button></details></article>
        </div>
        <div v-if="filtered.length > visibleCount" class="more"><span>已显示 {{ shown.length }} / {{ filtered.length }} 个项目</span><button class="btn" :disabled="listLoading" @click="router.replace(link({ shown: String(visibleCount + 12) }))">加载更多</button></div>
      </template>
      <div v-else-if="loading" class="state" role="status">正在读取项目…</div>
      <div v-else-if="error" class="state error" role="alert">{{ error }}<button class="btn" @click="retry">重试</button></div>
      <template v-else-if="project && reader">
        <section v-if="historical" class="revision-notice" role="status"><div><strong>正在查看历史版本</strong><p>图片、制作版本及选用状态均为当时记录，后续同步的素材不会出现在此版本中。</p></div><RouterLink class="btn" :to="link({ revision: undefined, state: undefined, version: undefined, compare: undefined })">查看最新版本</RouterLink></section>
        <div v-if="objectDetail" class="project-return"><button v-if="previousPage" class="btn" @click="router.back()">← 返回上一页</button><RouterLink class="btn" :to="link({ item: undefined, material: undefined, shot: undefined, scene: undefined, continuity: undefined, deliverable: undefined, file: undefined, fixed: undefined, run: undefined, reference: undefined, candidate: undefined, lighting: undefined, package: undefined, state: undefined, version: undefined, compare: undefined })">返回{{ activeTab?.label }}</RouterLink></div>
        <nav v-if="!objectDetail" class="business-tabs" aria-label="项目业务"><RouterLink v-for="tab in tabs" :key="tab.key" :class="{ active: activeTab?.key === tab.key }" :aria-current="activeTab?.key === tab.key ? 'page' : undefined" :to="menuLink(tab.key)">{{ tab.label }}</RouterLink></nav>
        <ProjectShared v-if="shared" :reader="reader" :tab="activeTab!.key" />
        <div v-else-if="!content" class="state">暂无业务内容。</div>
        <ShortDramaProject v-else-if="drama" :reader="reader" :tab="activeTab?.key || 'script'" />
        <ShortVideoProject v-else-if="video" :reader="reader" :tab="activeTab?.key || 'storyboards'" />
        <div v-else-if="query('item') && !detail" class="state">当前项目修订中没有这项内容。</div>
        <article v-else-if="detail && music && activeTab?.key === 'voice'" class="sections"><section class="panel"><h2>人声模型版本</h2><label>当前查看模型<select :value="detail.key" @change="router.push(link({ item: ($event.target as HTMLSelectElement).value }))"><option v-for="model in music.voice.models" :key="model.key" :value="model.key">{{ model.title }}</option></select></label><p class="prose">{{ value(detail.overview) }}</p><p class="muted">固定项目修订 {{ project.revision_id }}</p><dl v-if="detail.fields.length" class="fields"><template v-for="(field, i) in detail.fields" :key="i"><dt>{{ field.label }}</dt><dd>{{ value(field.value) }}</dd></template></dl></section><section class="panel"><h2>模型试听</h2><ProjectMedia v-for="id in detail.result_file_ids" :key="id" :reader="reader" :file-id="id" kind="audio" :label="detail.title + '试听'" /><p v-if="!detail.result_file_ids.length" class="muted">此模型未登记试听；干声录音不代替模型效果。</p></section><section class="panel"><h2>模型与版本文件</h2><ProjectMedia v-for="id in detail.file_ids" :key="id" :reader="reader" :file-id="id" kind="file" :label="reader.file(id).path" /><p v-if="!detail.file_ids.length" class="muted">此版本未登记模型文件。</p></section></article>
        <article v-else-if="detail" :key="`${project.revision_id}:${activeTab?.key}:${detail.key}`" class="sections">
          <section class="panel"><h2>概述</h2><p class="prose">{{ value(detail.overview) }}</p><p v-if="detail.duration_seconds != null" class="muted">{{ duration(detail.duration_seconds) }}</p><dl v-if="detail.fields.length" class="fields"><template v-for="(field, i) in detail.fields" :key="i"><dt>{{ field.label }}</dt><dd>{{ value(field.value) }}</dd></template></dl></section>
          <section class="panel"><h2>参考图</h2><div v-if="detail.reference_file_ids.length" class="reference-row"><ProjectMedia v-for="(id, i) in detail.reference_file_ids" :key="`${id}:${i}`" :reader="reader" :file-id="id" kind="image" :label="`参考图 ${i + 1}`" /></div><p v-else class="muted">未添加图片</p><ProjectPrompt :text="detail.prompt" /></section>
          <section v-if="activeTab?.key === 'songs'" class="panel"><h2>歌词</h2><p class="prose">{{ value(detail.lyrics) }}</p></section>
          <section class="panel"><h2>{{ resultKind === 'image' ? '结果图' : resultKind === 'audio' ? '歌曲' : '结果视频' }}</h2><div class="sections"><ProjectMedia v-for="(id, i) in detail.result_file_ids" :key="`${id}:${i}`" :reader="reader" :file-id="id" :kind="resultKind" :label="`${detail.title} ${i + 1}`" /><ProjectMedia v-if="!detail.result_file_ids.length" :reader="reader" :kind="resultKind" :label="detail.title" /></div></section>
          <section v-if="detail.file_ids.length" class="panel"><h2>文件</h2><ProjectMedia v-for="id in detail.file_ids" :key="id" :reader="reader" :file-id="id" kind="file" :label="reader.file(id).path" /></section>
        </article>
        <div v-else :key="`${project.revision_id}:${activeTab?.key}`" class="sections">
          <template v-if="character && activeTab?.key === 'setting'">
            <section class="panel"><h2>基本信息</h2><div class="character-basic"><div><ProjectMedia :reader="reader" :file-id="character.portrait_file_id" kind="image" label="半身照" /><ProjectPrompt :text="character.portrait_prompt" /></div><div><dl class="fields"><dt>姓名</dt><dd>{{ value(character.name) }}</dd><dt>概述</dt><dd>{{ value(content.overview) }}</dd><template v-for="(field, i) in character.fields" :key="i"><dt>{{ field.label }}</dt><dd>{{ value(field.value) }}</dd></template></dl></div></div></section>
            <section class="panel"><h2>角色设定图</h2><ProjectMedia :reader="reader" :file-id="character.sheet_file_id" kind="image" label="角色设定图" /><ProjectPrompt :text="character.sheet_prompt" /></section>
          </template>
          <template v-else-if="music && activeTab?.key === 'voice'">
            <section class="panel"><h2>基本信息</h2><dl class="fields"><dt>姓名</dt><dd>{{ value(music.voice.name || music.artist) }}</dd><dt>专辑</dt><dd>{{ value(music.name) }}</dd><dt>概述</dt><dd>{{ value(music.voice.description || content.overview) }}</dd></dl></section>
            <section class="panel"><h2>干声录音</h2><p v-if="!music.voice.recordings.length" class="muted">未添加音频</p><article v-for="recording in music.voice.recordings" :key="recording.key" class="voice-item"><h3>{{ recording.title }}</h3><p class="prose">{{ value(recording.overview) }}</p><ProjectMedia v-for="id in recording.result_file_ids" :key="id" :reader="reader" :file-id="id" kind="audio" :label="recording.title" /><ProjectMedia v-if="!recording.result_file_ids.length" :reader="reader" kind="audio" :label="recording.title" /></article></section>
            <section class="panel"><h2>人声模型</h2><p v-if="!music.voice.models.length" class="muted">未添加人声模型</p><article v-for="model in music.voice.models" :key="model.key" class="voice-item"><h3><RouterLink :to="link({ tab: 'voice', item: model.key })">{{ model.title }} · 查看版本 →</RouterLink></h3><p class="prose">{{ value(model.overview) }}</p><dl v-if="model.fields.length" class="fields"><template v-for="(field, i) in model.fields" :key="i"><dt>{{ field.label }}</dt><dd>{{ value(field.value) }}</dd></template></dl><ProjectMedia v-for="id in model.file_ids" :key="id" :reader="reader" :file-id="id" kind="file" :label="reader.file(id).path" /><p v-if="!model.file_ids.length" class="muted">未添加模型文件</p><ProjectMedia v-for="id in model.result_file_ids" :key="id" :reader="reader" :file-id="id" kind="audio" :label="`${model.title}试听`" /></article></section>
          </template>
          <template v-else>
            <p v-if="music" class="prose">{{ value(content.overview) }}</p>
            <div class="list-tools"><input type="search" :value="query('find')" :aria-label="'搜索' + activeTab?.label" placeholder="搜索名称或概述" @input="router.replace(link({ find: ($event.target as HTMLInputElement).value || undefined }))"><button v-if="character && project.can_write && !historical && activeTab?.key === 'extensions'" class="btn" @click="editCurrent('extension')">新建扩展</button></div><div v-if="!visibleEntries.length" class="state">暂无{{ activeTab?.label }}内容。</div>
            <div class="cards"><RouterLink v-for="(entry, index) in visibleEntries" :key="entry.key" class="project-card" :to="link({ tab: activeTab?.key, item: entry.key })"><ProjectMedia v-if="entry.image_file_id || activeTab?.key !== 'songs'" :reader="reader" :file-id="entry.image_file_id" kind="image" :label="entry.title" thumbnail /><div v-else class="song-glyph" aria-hidden="true">♫</div><div class="card-text"><small v-if="activeTab?.key === 'storyboards'">{{ String(index + 1).padStart(2, '0') }} · {{ duration(entry.duration_seconds) }}</small><h2>{{ entry.title }}</h2><p>{{ value(entry.overview) }}</p></div></RouterLink></div>
          </template>
        </div>
      </template>
      <ProjectEditDialog v-if="editingReader" :reader="editingReader" :mode="editingMode" @close="closeEdit" @saved="savedEdit" />
      <dialog ref="createDialog" class="create-dialog" @cancel.prevent="creating = false"><template v-if="creating"><button class="btn dialog-close" @click="creating = false">关闭</button><CreateProjectPage :modal="true" :initial-category="filter" @close="creating = false" /></template></dialog>
    </div>
  </main>
</template>

<style scoped>
.project-return { display:flex; flex-wrap:wrap; gap:16px; align-items:center; margin:14px 0; }

.drama-browser { line-height:1.55; padding: 27px clamp(20px, 3vw, 52px) 70px; }
.drama-browser > .workspace { max-width: 1480px; margin: auto; }
.drama-browser .heading { align-items: center; border-bottom: 1px solid var(--line); padding-bottom: 20px; margin-bottom: 20px; }
.drama-browser .heading h1 { font-size:30px; line-height:1.28; font-weight:650; margin-bottom:8px; }
.drama-browser .eyebrow { display:block; font-size:12px; font-weight:400; letter-spacing:.12em; margin-bottom:10px; }
.drama-browser .breadcrumbs { font-size:12px; gap:9px; margin-bottom:13px; }
.drama-browser .business-tabs { gap: 22px; border-bottom: 1px solid var(--line); margin: 0 0 30px; }
.drama-browser .business-tabs a { padding: 12px 0; border: 0; border-bottom: 2px solid transparent; border-radius: 0; font-size: 13px; background: transparent; }
.drama-browser .business-tabs a.active { background:transparent; color: var(--ink); border-bottom-color: var(--accent); font-weight: 600; }
.drama-browser :deep(a:focus-visible), .drama-browser :deep(button:focus-visible), .drama-browser :deep(input:focus-visible), .drama-browser :deep(select:focus-visible), .drama-browser :deep(textarea:focus-visible), .drama-browser :deep(summary:focus-visible) { outline:2px solid var(--accent); outline-offset:3px; }
.drama-browser :deep(.btn:hover) { border-color:var(--accent); }
.revision-notice { display:flex; align-items:center; justify-content:space-between; gap:16px; border-left:2px solid var(--accent); background:var(--shell-panel); padding:12px 16px; margin-bottom:22px; font-size:13px; }
.revision-notice p { color:var(--muted); font-size:12px; margin-top:5px; line-height:1.8; }
.revision-notice .btn { flex-shrink:0; }
@media(max-width:800px) { .drama-browser .heading h1 { font-size:25px; }.drama-browser .business-tabs { gap:16px; }.revision-notice { flex-wrap:wrap; } }

.project-browser { overflow-wrap: anywhere; }
.create-dialog { position:fixed; inset:10vh 0 auto; margin:auto; width:min(640px, calc(100vw - 32px)); border:1px solid var(--line); border-radius:12px; color:var(--ink); background:var(--panel); padding:20px; z-index:20; max-height:90vh; overflow:auto; }
.create-dialog::backdrop { background:var(--aside); opacity:.85; }
.project-card-wrap { position:relative; min-width:0; }
.project-card-wrap .card-text { padding-right:62px; }
.project-card-wrap .card-text h2 { display:-webkit-box; -webkit-line-clamp:2; -webkit-box-orient:vertical; overflow:hidden; min-height:2.8em; }
.card-menu[open] { z-index:2; }
.card-menu { position:absolute; right:14px; bottom:14px; border:1px solid var(--line); border-radius:8px; background:var(--panel); padding:2px 8px; }
.card-menu summary { cursor:pointer; list-style:none; font-size:18px; }
.card-menu button { display:block; padding:8px 12px; font-size:12px; }
.card-menu button:hover { color:var(--accent); }
.dialog-close { float:right; }
.list-tools select { border:1px solid var(--line); padding:9px; border-radius:8px; color:var(--ink); background:var(--panel); }
.eyebrow { color: var(--accent); font-size: 11px; font-weight: 700; letter-spacing: .14em; }
.breadcrumbs { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin-bottom: 20px; font-size: 13px; color: var(--muted); }
.breadcrumbs a:hover { color: var(--accent); }
.breadcrumbs [aria-current] { color: var(--ink); }
.heading { gap: 16px; }
.business-tabs { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 22px; }
.business-tabs a { border: 1px solid transparent; padding: 9px 17px; border-radius: 8px; color: var(--muted); }
.business-tabs a.active { color: var(--accent); background: var(--panel); border-color: var(--line); }
.list-tools { display: flex; justify-content: space-between; gap: 18px; flex-wrap: wrap; align-items: start; margin-bottom: 22px; }
.list-tools .business-tabs { margin: 0; }
input { width: 250px; max-width: 100%; padding: 10px 14px; border: 1px solid var(--line); border-radius: 8px; color: var(--ink); background: var(--panel); }
.cards { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 20px; }
.project-card { display: block; min-width: 0; border: 1px solid var(--line); border-radius: 14px; overflow: hidden; background: var(--panel); }
a.project-card:hover, .project-card:has(a:hover) { border-color: var(--accent); }
.card-text { padding: 18px; }
.card-text h2 { font-size: 18px; margin: 4px 0 8px; }
.card-text p { color: var(--muted); display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
small, .muted { color: var(--muted); }
.song-glyph { aspect-ratio: 16 / 10; display: grid; place-items: center; font-size: 56px; line-height: 1; color: var(--accent); background: var(--subtle); }
.sections { display: grid; gap: 24px; }
.panel { padding: 24px; min-width: 0; background: var(--panel); border: 1px solid var(--line); border-radius: 14px; }
.panel > h2 { font-size: 18px; margin-bottom: 20px; }
.prose { white-space: pre-wrap; line-height: 1.8; }
.character-basic { display: grid; grid-template-columns: minmax(180px, 280px) minmax(0, 1fr); gap: 28px; align-items: start; }
.character-basic :deep(.image-open img) { aspect-ratio: 4 / 5; object-fit: cover; object-position: center top; }
.fields { display: grid; grid-template-columns: minmax(80px, 140px) minmax(0, 1fr); }
.fields dt, .fields dd { padding: 12px 10px; border-bottom: 1px solid var(--line); white-space: pre-wrap; }
.fields dt { color: var(--muted); }
.reference-row { display: flex; gap: 16px; overflow-x: auto; padding-bottom: 8px; }
.reference-row > :deep(.project-media) { flex: 0 0 220px; }
.reference-row :deep(.image-open img) { height: 240px; object-fit: cover; object-position: center top; }
.voice-item + .voice-item { border-top: 1px solid var(--line); padding-top: 24px; margin-top: 24px; }
.voice-item h3, .voice-item .prose, .voice-item .fields { margin-bottom: 14px; }
.voice-item :deep(.project-media) + :deep(.project-media) { margin-top: 14px; }
.video-meta { margin: 8px 0 18px; }
.drama-back { justify-self: start; }
.drama-grid { display: grid; grid-template-columns: minmax(220px, 300px) minmax(0, 1fr); gap: 24px; align-items: start; }
.drama-episodes { display: grid; gap: 7px; }
.drama-episodes a { display: grid; padding: 12px 14px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
.drama-episodes a.active { border-color: var(--accent); box-shadow: inset 3px 0 var(--accent); }
.drama-episodes small { color: var(--muted); }
.drama-section-head { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; }
.drama-scene, .drama-continuity, .drama-run, .drama-delivery { border-top: 1px solid var(--line); padding: 18px 0; }
.drama-scene h3, .drama-continuity h3, .drama-delivery h3 { margin-bottom: 5px; }
.drama-shots { display: grid; gap: 6px; margin-top: 12px; }
.drama-shots a { display: grid; grid-template-columns: 95px minmax(0, 1fr) auto; gap: 12px; align-items: center; border: 1px solid var(--line); border-radius: 6px; padding: 10px 12px; background: var(--panel); }
.drama-shots a:hover { border-color: var(--accent); }
.drama-detail { display: grid; grid-template-columns: minmax(0, 1.5fr) minmax(250px, .8fr); gap: 24px; align-items: start; }
.drama-refs { display: flex; flex-wrap: wrap; gap: 7px; }
.drama-continuity .reference-row { margin-top: 10px; }
.drama-run { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 8px; }
.drama-delivery-files { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; margin-top: 12px; }
.drama-delivery-files small { display: block; margin-bottom: 8px; }
.state { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; padding: 30px; border: 1px dashed var(--line); border-radius: 12px; color: var(--muted); margin-bottom: 20px; }
.error { color: var(--danger); }
.more { display: flex; justify-content: center; gap: 16px; align-items: center; margin-top: 24px; color: var(--muted); }
@media (max-width: 1150px) { .cards { grid-template-columns: repeat(2, minmax(0, 1fr)); } .character-basic { grid-template-columns: minmax(140px, 220px) minmax(0, 1fr); gap: 18px; } .drama-grid, .drama-detail { grid-template-columns: minmax(0, 1fr); } }
@media (max-width: 700px) { .project-browser { padding: 20px 16px 40px; } .cards, .character-basic, .drama-delivery-files { grid-template-columns: minmax(0, 1fr); } .panel { padding: 18px; } .fields { grid-template-columns: 84px minmax(0, 1fr); } .business-tabs a { padding: 8px 10px; } .list-tools, input { width: 100%; } .reference-row > :deep(.project-media) { flex-basis: 190px; } .drama-shots a { grid-template-columns: 70px minmax(0, 1fr); } .drama-shots a .muted:last-child { grid-column: 2; } }
.video-browser { padding:27px clamp(20px,3vw,52px) 70px; }
.video-browser > .workspace { max-width:1480px; margin:auto; }
.video-browser .heading { align-items:start; }
.video-browser .heading h1 { font-size:28px; line-height:1.35; }
.video-browser .business-tabs { gap:7px; border-bottom:1px solid var(--line); padding-bottom:12px; margin:22px 0 0; }
.video-browser .business-tabs a { padding:9px 16px; border:0; border-radius:8px; }
.video-browser .business-tabs a.active { background:var(--paper); color:var(--button-ink); }
@media(max-width:700px) { .video-browser { padding:20px 16px 40px; } }
</style>
