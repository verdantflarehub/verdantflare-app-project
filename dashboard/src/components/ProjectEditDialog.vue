<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { ProjectReader, type ProjectItem } from '../platform/project-reader'
import { collectProjects, isCategory } from '../platform/project-management'
import { callTool } from '../platform/mcp'
import { contentChanges, createProjectEditor, verifyEditReadback, type EditChanges, type EditState } from '../platform/project-edit'
import { useHost } from '../platform/host'
import ProjectMedia from './ProjectMedia.vue'
import CreateProjectPage from '../pages/CreateProjectPage.vue'
import type { CreateResult } from '../platform/project-create'

const props = defineProps<{ reader: ProjectReader; mode: 'rename' | 'cover' | 'extension' | 'cast' | 'remove-cast'; itemKey?: string }>()
const emit = defineEmits<{ close: []; saved: [revision: string] }>()
const { appSDK, dashboardHost } = useHost()
const baseReader = shallowRef(props.reader)
const project = computed(() => baseReader.value.project!)
const dialog = ref<HTMLDialogElement>(), name = ref(props.mode === 'rename' ? project.value.manifest.name : '')
const image = ref(props.mode === 'cover' ? props.reader.content?.cover_file_id || '' : '')
const description = ref(''), prompt = ref(''), role = ref<'lead' | 'support'>('lead'), source = ref('')
const sources = ref<ProjectItem[]>([]), error = ref(''), busy = ref(false), hidden = ref(false), state = ref<EditState>('unknown'), pending = ref(false)
const sourceCreating = ref(false), childBusy = ref(false)
const title = { rename: '重命名项目', cover: '修改封面', extension: '新建角色扩展', cast: '引用角色', 'remove-cast': '移出片中角色' }[props.mode]
const images = computed(() => project.value.manifest.files.filter(f => /\.(png|jpe?g|webp|gif)$/i.test(f.path)))
let alive = true, editor: ReturnType<typeof createProjectEditor> | undefined
const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
try { editor = createProjectEditor(appSDK, window.localStorage, project.value.project_id); pending.value = !!editor.pending() } catch (cause) { error.value = String(cause instanceof Error ? cause.message : cause) }
async function readContent(): Promise<Record<string, any>> {
  const doc = project.value.manifest.domain_documents.find(d => d.document_type === 'project-content')
  if (doc) return JSON.parse((await baseReader.value.document(doc.file_id)).text) as Record<string, any>
  if (!isCategory(project.value.manifest.category)) throw new Error('当前项目类型不支持业务编辑。')
  return { schema_version: 1, kind: 'project-content', category: project.value.manifest.category, [project.value.manifest.category]: project.value.manifest.category === 'ai_short_drama' ? { episodes: [] } : {} }
}
async function changes(): Promise<EditChanges> {
  if (props.mode === 'rename') return { set_metadata: { name: name.value.trim() } }
  const content = await readContent()
  if (props.mode === 'cover') content.cover_file_id = image.value || null
  if (props.mode === 'extension') {
    const body = content.character
    body.extensions ||= []
    body.extensions.push({ key: 'extension_' + crypto.randomUUID().replaceAll('-', ''), title: name.value.trim(), overview: description.value || null, prompt: prompt.value || null, image_file_id: image.value || null, fields: [], reference_file_ids: [], result_file_ids: image.value ? [image.value] : [], file_ids: [], lyrics: null, duration_seconds: null })
  }
  if (props.mode === 'cast') {
    const selected = sources.value.find(p => p.project_id === source.value)
    if (!selected) throw new Error('请选择来源角色项目。')
    // Reauthorize and pin the actual source head just before constructing the edit.
    const sourceReader = new ProjectReader(selected.project_id)
    try {
      await sourceReader.load()
      if (sourceReader.project?.manifest.category !== 'character') throw new Error('来源不是角色项目。')
      const body = content.video
      body.cast ||= []
      body.cast.push({ key: 'cast_' + crypto.randomUUID().replaceAll('-', ''), name: name.value.trim() || sourceReader.project.manifest.name, role: role.value, portrait_file_id: image.value || null, project_id: selected.project_id, revision_id: sourceReader.project.revision_id })
    } finally { sourceReader.dispose() }
  }
  if (props.mode === 'remove-cast') {
    if (!content.video?.cast?.some((c: { key: string }) => c.key === props.itemKey)) throw new Error('当前修订没有此片中角色。')
    content.video.cast = content.video.cast.filter((c: { key: string }) => c.key !== props.itemKey)
  }
  return contentChanges(project.value, JSON.stringify(content))
}
function sync() { if (editor && alive) { pending.value = !!editor.pending(); state.value = editor.state() } }
async function save(status = false) {
  if (!editor || busy.value || hidden.value) return
  busy.value = true; error.value = ''; dashboardHost.setDirty(true)
  try {
    if (!editor.pending()) {
      if (project.value.can_write !== true || project.value.revision_id !== project.value.head_revision_id) throw new Error('请在有编辑权限的最新项目修订中修改。')
      await editor.begin(project.value.revision_id, await changes())
    }
    sync()
    const result = await (status ? editor.status() : editor.submit())
    sync()
    if (result.state === 'committed' && result.revision) {
      const verify = new ProjectReader(project.value.project_id, result.revision)
      try {
        await verify.load()
        await verifyEditReadback(editor.pending()!.request, verify.project!, id => verify.document(id))
      } finally { verify.dispose() }
      await editor.clearTerminal()
      if (alive) { dashboardHost.setDirty(false); emit('saved', result.revision) }
    }
  } catch (cause) { if (alive) error.value = cause instanceof Error ? cause.message : '暂未确认保存结果，原请求已保留。' }
  finally { if (alive) { sync(); busy.value = false; dashboardHost.setDirty(false) } }
}
async function refreshAfterConflict() {
  if (!editor || busy.value) return
  try { const latest = new ProjectReader(project.value.project_id); await latest.load(); if (!latest.project?.can_write) { latest.dispose(); throw new Error('当前身份没有编辑权限。') }; await editor.clearTerminal(); if (baseReader.value !== props.reader) baseReader.value.dispose(); baseReader.value = latest; sync(); error.value = '已读取最新修订，输入已保留。请核对后再次保存。' }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '无法读取最新修订。' }
}
async function sourceCreated(result: CreateResult) {
  const created = new ProjectReader(result.project_id, result.revision_id)
  try {
    await created.load()
    if (!alive || hidden.value) return
    const p = created.project!
    if (p.manifest.category !== 'character') throw new Error('已创建项目属于其他类型，请选择角色项目。')
    sources.value = [{ project_id: p.project_id, head_revision_id: p.revision_id, category: p.manifest.category, name: p.manifest.name }, ...sources.value.filter(s => s.project_id !== p.project_id)]
    source.value = p.project_id; sourceCreating.value = false
  } catch (cause) { if (alive) error.value = cause instanceof Error ? cause.message : '无法读取新角色。' }
  finally { created.dispose() }
}
function close() { if (!busy.value && !childBusy.value) emit('close') }
onMounted(async () => {
  await nextTick(); dialog.value?.showModal()
  if (props.mode === 'cast') {
    busy.value = true
    try { const list = await collectProjects('character', args => callTool('project.list', args), () => alive && !hidden.value); if (alive) sources.value = list }
    catch (cause) { if (alive) error.value = cause instanceof Error ? cause.message : '无法读取角色项目。' }
    finally { if (alive) busy.value = false }
  }
})
const stop = appSDK.lifecycle.subscribe(event => { hidden.value = event === 'hidden'; if (event === 'dispose') { alive = false; editor?.dispose() } })
onBeforeUnmount(() => { alive = false; editor?.dispose(); if (baseReader.value !== props.reader) baseReader.value.dispose(); stop(); dialog.value?.close(); dashboardHost.setDirty(false); previousFocus?.focus() })
</script>

<template>
  <dialog ref="dialog" class="edit-dialog" aria-labelledby="edit-title" @cancel.prevent="close">
    <header><h2 id="edit-title">{{ title }}</h2><button class="btn" :disabled="busy || childBusy" @click="close">关闭</button></header>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <p v-if="busy" role="status">正在确认结果…</p>
    <template v-if="pending"><p v-if="editor?.pending()?.request.changes.set_metadata">原名称输入：{{ editor?.pending()?.request.changes.set_metadata?.name }}</p><details v-if="editor?.pending()?.request.changes.upsert_files"><summary>查看原保存内容</summary><pre>{{ editor?.pending()?.request.changes.upsert_files?.[0].text }}</pre></details><p>原保存{{ ({ unknown: '结果待确认', preparing: '正在准备', committed: '已完成，等待回读', conflict: '发生冲突', failed: '已被拒绝' })[state] }}。恢复将使用已保留的原请求。</p><div class="actions"><button class="btn" :disabled="busy || hidden" @click="save(true)">查询结果</button><button v-if="state === 'unknown' || state === 'preparing' || state === 'committed'" class="btn primary" :disabled="busy || hidden" @click="save()">恢复原保存</button><button v-if="state === 'conflict' || state === 'failed'" class="btn" :disabled="busy || hidden" @click="refreshAfterConflict">保留原记录并读取最新修订</button></div></template>
    <section v-else-if="sourceCreating"><CreateProjectPage modal initial-category="character" return-to-caller @created="sourceCreated" @busy="childBusy = $event" /><button class="btn" :disabled="childBusy" @click="sourceCreating = false">返回引用角色</button></section>
    <form v-else @submit.prevent="save()">
      <label v-if="mode === 'rename' || mode === 'extension' || mode === 'cast'">{{ mode === 'cast' ? '本片角色名称（可选）' : '名称' }}<input v-model="name" :required="mode !== 'cast'" maxlength="256" :disabled="busy || hidden" autofocus></label>
      <template v-if="mode === 'extension'"><label>概述<textarea v-model="description" rows="3" :disabled="busy || hidden" /></label><label>提示词<textarea v-model="prompt" rows="3" :disabled="busy || hidden" /></label></template>
      <template v-if="mode === 'cast'"><label>来源角色<select v-model="source" required :disabled="busy || hidden"><option value="" disabled>选择角色项目</option><option v-for="p in sources" :key="p.project_id" :value="p.project_id">{{ p.name }}</option></select></label><label>本片身份<select v-model="role" :disabled="busy || hidden"><option value="lead">主角</option><option value="support">配角</option></select></label><button type="button" class="btn source-create" :disabled="busy || hidden" @click="sourceCreating = true">新建角色项目 →</button></template>
      <template v-if="mode === 'cover' || mode === 'extension' || mode === 'cast'"><label>{{ mode === 'cover' ? '项目封面' : '使用当前项目已有图片（可选）' }}<select v-model="image" :disabled="busy || hidden"><option value="">{{ mode === 'cover' ? '不设置封面' : '暂不选择' }}</option><option v-for="f in images" :key="f.file_id" :value="f.file_id">{{ f.path }}</option></select></label><ProjectMedia v-if="image" :reader="baseReader" :file-id="image" kind="image" label="所选图片" /><p v-else-if="!images.length" class="muted">当前项目没有可选图片。</p></template>
      <p v-if="mode === 'remove-cast'">移出当前片中的角色引用，来源角色项目保留。</p>
      <div class="actions"><button type="button" class="btn" :disabled="busy" @click="close">取消</button><button type="submit" class="btn primary" :disabled="busy || hidden || !editor">保存</button></div>
    </form>
  </dialog>
</template>

<style scoped>
.edit-dialog { position:fixed; inset:5vh 0 auto; width:min(620px,calc(100vw - 32px)); max-height:90vh; margin:auto; padding:24px; border:1px solid var(--line); border-radius:14px; background:var(--panel); color:var(--ink); overflow:auto; }.edit-dialog::backdrop { background:var(--aside); opacity:.85; }header { display:flex; align-items:center; justify-content:space-between; gap:16px; margin-bottom:20px; }h2 { font-size:20px; }form,label { display:grid; gap:12px; }form { gap:18px; }input,select,textarea { padding:10px 12px; border:1px solid var(--line); border-radius:8px; background:var(--subtle); color:var(--ink); width:100%; }textarea { resize:vertical; }.actions { display:flex; gap:12px; justify-content:flex-end; flex-wrap:wrap; margin-top:20px; }.error { color:var(--danger); margin-bottom:16px; }.muted { color:var(--muted); }.source-create { color:var(--accent); }.edit-dialog :deep(.image-open img) { max-height:240px; width:auto; object-fit:contain; }
</style>
