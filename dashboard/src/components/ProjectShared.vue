<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ProjectReader, readerError, type ProjectOpen } from '../platform/project-reader'
import { callTool } from '../platform/mcp'
import { readProjectHistory } from '../platform/project-revisions'
import { useHost } from '../platform/host'
import { nativeRunTarget, worldTarget } from '../platform/route-links'
import ProjectMedia from './ProjectMedia.vue'
import ProjectCandidateDetail from './ProjectCandidateDetail.vue'
import ProjectSourceDetail from './ProjectSourceDetail.vue'
import ProjectRevisionDetail from './ProjectRevisionDetail.vue'
import { projectSources } from '../platform/project-source'
import DramaDocument from './drama/DramaDocument.vue'

const props = defineProps<{ reader: ProjectReader; tab: string }>()
const route = useRoute(), { appSDK } = useHost()
const q = (key: string) => typeof route.query[key] === 'string' ? route.query[key] as string : ''
const project = computed(() => props.reader.project!)
const fileType = (path: string) => /\.(md|txt|json)$/i.test(path) ? 'document' : kind(path)
const files = computed(() => project.value.manifest.files.filter(f => f.path.toLocaleLowerCase().includes(q('find').toLocaleLowerCase()) && (!q('fileType') || q('fileType') === fileType(f.path))))
const file = computed(() => project.value.manifest.files.find(f => f.file_id === q('file')))
const fileInfo = ref<{ mime: string; size: number; sha256: string }>(), fileBusy = ref(false)
let fileGeneration = 0
const revisions = ref<ProjectOpen[]>([]), busy = ref(false), error = ref('')
const currentRevision = computed(() => revisions.value.find(r => r.revision_id === q('fixed')))
const parentRevision = computed(() => revisions.value.find(r => r.revision_id === currentRevision.value?.parent_revision_id))
const runs = computed(() => project.value.manifest.run_refs || [])
const run = computed(() => runs.value.find(r => `${r.service_id}:${r.run_id}` === q('run')))
const nativeRun = computed(() => run.value ? nativeRunTarget(run.value.service_id, run.value.run_id) : null)
const openingRun = ref(false)
const refs = computed(() => project.value.manifest.asset_refs || [])
const sources = computed(() => projectSources(props.reader.content))
const source = computed(() => sources.value.find(s => s.key === q('reference')))
const reference = computed(() => refs.value.find(r => `${r.asset_id}:${r.asset_version_id}:${r.purpose}` === q('reference')))
const referenceMaterials = computed(() => reference.value ? (props.reader.content?.ai_short_drama?.globals || []).filter(g => g.asset_id === reference.value!.asset_id && g.asset_version_id === reference.value!.asset_version_id) : [])
const referenceUsages = computed(() => (props.reader.content?.ai_short_drama?.usages || []).filter(u => referenceMaterials.value.some(g => g.key === u.global_key)))
const referenceContinuity = computed(() => reference.value ? (props.reader.content?.ai_short_drama?.continuity || []).filter(c => c.asset_id === reference.value!.asset_id && c.asset_version_id === reference.value!.asset_version_id) : [])
const candidates = computed(() => props.reader.content?.ai_short_drama?.world_candidates || [])
const candidate = computed(() => candidates.value.find(c => c.key === q('candidate')))
let generation = 0, alive = true, hidden = false
const activeEpoch = ref(0)
watch(() => [props.reader, props.tab, q('file'), activeEpoch.value], async () => { const token = ++fileGeneration; fileInfo.value = undefined; fileBusy.value = false; if (!file.value || hidden || props.tab !== 'files') return; fileBusy.value = true; try { const info = await props.reader.download(file.value.file_id); if (alive && !hidden && token === fileGeneration) fileInfo.value = info.version } catch (cause) { if (alive && !hidden && token === fileGeneration) error.value = readerError(cause) } finally { if (alive && token === fileGeneration) fileBusy.value = false } }, { immediate: true })
function link(change: Record<string, string | undefined>) { return { path: route.path, query: { ...route.query, ...change } } }
function kind(path: string): 'file' | 'image' | 'audio' | 'video' {
  return /\.(png|jpe?g|webp|gif)$/i.test(path) ? 'image' : /\.(mp3|wav|flac|ogg|m4a)$/i.test(path) ? 'audio' : /\.(mp4|webm)$/i.test(path) ? 'video' : 'file'
}
const time = (value?: string) => value && Number.isFinite(Date.parse(value)) ? new Date(value).toLocaleString('zh-CN') : '时间未知'
async function history() {
  const token = ++generation
  revisions.value = []; error.value = ''; busy.value = true
  const owner = project.value
  try {
    await readProjectHistory(owner.project_id, owner.head_revision_id || owner.revision_id,
      revision => callTool<ProjectOpen>('project.open', { project_id: owner.project_id, revision_id: revision }),
      () => alive && !hidden && token === generation, item => revisions.value.push(item))
  } catch (cause) { if (alive && token === generation) error.value = readerError(cause) }
  finally { if (alive && token === generation) busy.value = false }
}
async function openWorld() {
  if (!reference.value) return
  error.value = ''
  try { const target = worldTarget(reference.value.asset_id, reference.value.asset_version_id); await appSDK.capabilities.call('navigation.openApp', { appId: target.appId, path: target.path }) }
  catch { if (alive) error.value = '无法打开固定资产，请确认数字世界已安装且当前身份有权访问。' }
}
async function openRun() {
  const target = nativeRun.value
  if (!target || openingRun.value || hidden) return
  openingRun.value = true; error.value = ''
  try { await appSDK.capabilities.call('navigation.openApp', { appId: target.appId, path: target.path }) }
  catch { if (alive) error.value = '无法打开原任务，请确认原服务入口可用且当前身份有权访问；原任务 ID 已保留。' }
  finally { if (alive) openingRun.value = false }
}
watch(() => [props.reader, props.tab], () => { generation++; error.value = ''; if (props.tab === 'revisions') void history() }, { immediate: true })
const stop = appSDK.lifecycle.subscribe(event => {
  if (event === 'hidden') { hidden = true; generation++; fileGeneration++; busy.value = false; fileBusy.value = false }
  if (event === 'active') { hidden = false; activeEpoch.value++; if (props.tab === 'revisions') void history() }
  if (event === 'dispose') { alive = false; generation++; fileGeneration++; revisions.value = []; fileInfo.value = undefined }
})
onBeforeUnmount(() => { alive = false; generation++; fileGeneration++; stop() })
</script>

<template>
  <div class="shared-page">
    <p v-if="error" class="empty error" role="alert">{{ error }}<button v-if="tab === 'revisions'" class="btn" @click="history">重试</button></p>
    <template v-if="tab === 'files'">
      <template v-if="q('file')"><section v-if="file" class="panel"><h2>{{ file.path }}</h2><p class="muted">{{ file.role || '未登记用途' }} · 固定内容版本 {{ file.content_ref.version_id }}</p><p v-if="fileBusy" class="muted" role="status">正在读取文件信息…</p><dl v-if="fileInfo"><dt>格式</dt><dd>{{ fileInfo.mime }}</dd><dt>大小</dt><dd>{{ fileInfo.size.toLocaleString('zh-CN') }} 字节</dd><dt>SHA-256</dt><dd>{{ fileInfo.sha256 }}</dd></dl><DramaDocument v-if="fileType(file.path) === 'document'" :reader="reader" :file-id="file.file_id" :title="file.path" initially-expanded /><ProjectMedia v-else :reader="reader" :file-id="file.file_id" :kind="kind(file.path)" :label="file.path" /><ProjectMedia v-if="kind(file.path) !== 'file' || fileType(file.path) === 'document'" :reader="reader" :file-id="file.file_id" kind="file" label="下载原文件" /></section><p v-else class="empty">文件不在当前项目修订中。</p></template>
      <template v-else><header><h2>文件</h2><input type="search" :value="q('find')" placeholder="搜索文件路径" aria-label="搜索文件路径" @input="$router.replace(link({ find: ($event.target as HTMLInputElement).value || undefined }))"><select aria-label="按文件扩展类型筛选" :value="q('fileType')" @change="$router.replace(link({ fileType: ($event.target as HTMLSelectElement).value || undefined }))"><option value="">全部类型</option><option value="document">文档</option><option value="image">图片</option><option value="audio">音频</option><option value="video">视频</option><option value="file">其他文件</option></select></header><div class="rows"><RouterLink v-for="f in files" :key="f.file_id" :to="link({ file: f.file_id })"><strong>{{ f.path }}</strong><small>{{ f.role || '未登记用途' }}</small></RouterLink></div><p v-if="!files.length" class="empty">暂无匹配文件。</p></template>
    </template>
    <template v-else-if="tab === 'revisions'">
      <p v-if="busy" class="muted" role="status">正在读取固定修订…</p>
      <ProjectRevisionDetail v-if="q('fixed') && currentRevision" :revision="currentRevision" :parent="parentRevision" :history-busy="busy" />
      <p v-else-if="q('fixed') && !busy" class="empty">当前目录未找到此修订。</p>
      <div v-else class="rows"><RouterLink v-for="r in revisions" :key="r.revision_id" :to="link({ fixed: r.revision_id })"><strong>{{ time(r.created_at) }}{{ r.revision_id === project.head_revision_id ? ' · 最新修订' : '' }}</strong><small>{{ r.revision_id }}</small></RouterLink></div>
    </template>
    <template v-else-if="tab === 'runs'">
      <section v-if="q('run') && run" class="panel"><h2>原生运行</h2><dl><dt>服务</dt><dd>{{ run.service_id }}</dd><dt>运行</dt><dd>{{ run.run_id }}</dd><dt>状态</dt><dd>需由原生服务查询，项目引用不代表运行成功。</dd></dl><button v-if="nativeRun" class="btn" :disabled="openingRun" @click="openRun">{{ openingRun ? '正在打开原任务…' : '打开原任务查询与恢复' }}</button><p v-else class="muted">此服务尚未提供可识别的原生任务入口，请凭上述原任务 ID 查询。</p></section><p v-else-if="q('run')" class="empty">运行不在当前项目修订中。</p><template v-else><h2>运行</h2><div class="rows"><RouterLink v-for="r in runs" :key="`${r.service_id}:${r.run_id}`" :to="link({ run: `${r.service_id}:${r.run_id}` })"><strong>{{ r.service_id }}</strong><small>{{ r.run_id }}</small></RouterLink></div><p v-if="!runs.length" class="empty">暂无关联运行。</p></template>
    </template>
    <template v-else-if="tab === 'references'">
      <ProjectSourceDetail v-if="source" :reader="reader" :source="source" />
      <template v-else-if="q('reference') && reference"><section class="panel"><h2>固定资产引用</h2><dl><dt>来源类型</dt><dd>数字世界固定资产版本</dd><dt>用途</dt><dd>{{ reference.purpose }}</dd><dt>资产</dt><dd>{{ reference.asset_id }}</dd><dt>版本</dt><dd>{{ reference.asset_version_id }}</dd></dl><button class="btn" @click="openWorld">打开来源详情</button><h3>许可与适配</h3><p class="muted">本项目引用未保存源包预览、许可范围与期限。请在来源固定版本详情核对实际读取权限、源文件及使用限制。</p></section><section class="panel"><h2>本项目使用位置</h2><div class="rows"><RouterLink v-for="g in referenceMaterials" :key="g.key" :to="link({ reference: undefined, tab: g.kind, material: g.key })"><strong>{{ g.name }}</strong><small>{{ g.description || '查看本项目关联资料' }}</small></RouterLink><RouterLink v-for="c in referenceContinuity" :key="c.key" :to="link({ reference: undefined, tab: 'continuity', continuity: c.key })"><strong>{{ c.name }}</strong><small>{{ c.description || '查看连续性记录' }}</small></RouterLink></div><ul v-if="referenceUsages.length"><li v-for="u in referenceUsages" :key="u.key">{{ u.description || u.key }} · 集 {{ u.target.episode_key }}<template v-if="u.target.scene_key"> / 场 {{ u.target.scene_key }}</template><template v-if="u.target.shot_key"> / 镜 {{ u.target.shot_key }}</template></li></ul><p v-if="!referenceMaterials.length && !referenceContinuity.length" class="muted">当前固定修订未登记业务对象与此资产版本的使用位置映射。</p></section></template><p v-else-if="q('reference')" class="empty">资产引用不在当前修订中。</p>
      <template v-else-if="q('candidate')"><ProjectCandidateDetail v-if="candidate" :reader="reader" :candidate="candidate" /><p v-else class="empty">当前修订没有此入库候选。</p></template>
      <template v-else><h2>资产引用</h2><h3>直接项目来源</h3><div class="rows"><RouterLink v-for="s in sources" :key="s.key" :to="link({ reference: s.key })"><strong>{{ s.name }} · {{ s.purpose }}</strong><small>{{ s.category === 'music' ? '音乐' : '角色' }}项目 · {{ s.revision_id }}</small></RouterLink></div><p v-if="!sources.length" class="muted">暂无直接项目来源。</p><h3>数字世界固定资产</h3><div class="rows"><RouterLink v-for="r in refs" :key="`${r.asset_id}:${r.asset_version_id}:${r.purpose}`" :to="link({ reference: `${r.asset_id}:${r.asset_version_id}:${r.purpose}` })"><strong>{{ r.purpose }}</strong><small>{{ r.asset_id }} · {{ r.asset_version_id }}</small></RouterLink></div><p v-if="!refs.length" class="empty">暂无固定资产引用。</p><h2>入库候选</h2><div class="rows"><RouterLink v-for="c in candidates" :key="c.key" :to="link({ candidate: c.key })"><strong>{{ c.name }}</strong><small>{{ c.description || '查看入库依据与状态' }}</small></RouterLink></div><p v-if="!candidates.length" class="empty">暂无入库候选。</p></template>
    </template>
  </div>
</template>

<style scoped>
.shared-page { display:grid; gap:20px; min-width:0; overflow-wrap:anywhere; }header { display:flex; align-items:center; gap:20px; flex-wrap:wrap; }h2 { font-size:18px; margin:0 0 12px; }.panel { padding:24px; border:1px solid var(--line); border-radius:12px; background:var(--shell-panel); min-width:0; }.muted,small,dt { color:var(--muted); }input,select { padding:10px 14px; border:1px solid var(--line); border-radius:8px; background:var(--panel); color:var(--ink); max-width:100%; }.rows { display:grid; gap:10px; }.rows a { display:grid; padding:16px 20px; gap:6px; border:1px solid var(--line); border-radius:10px; background:var(--shell-panel); }.rows a:hover { border-color:var(--accent); }.empty { padding:24px; border:1px dashed var(--line); border-radius:10px; color:var(--muted); }.error { color:var(--danger); }dl { display:grid; grid-template-columns:80px minmax(0,1fr); gap:12px; margin:20px 0; }
</style>
