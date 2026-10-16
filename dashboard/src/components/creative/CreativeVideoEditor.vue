<script setup lang="ts">
import { onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useHost } from '../../platform/host'
import { callTool } from '../../platform/mcp'
import { createCreativeSaver } from '../../platform/creative-save'
import type { CreativeMusicSource, ProjectItem, ProjectOpen } from '../../platform/project-content'
import type { ProjectReader } from '../../platform/project-reader'

const props = defineProps<{ reader: ProjectReader }>()
const emit = defineEmits<{ saved: [revision: string] }>()
const { appSDK, dashboardHost } = useHost()
const project = props.reader.project!
const storageError = ref('')
let saver: ReturnType<typeof createCreativeSaver> | null = null, hasPending = false
try { saver = createCreativeSaver(appSDK, project.project_id, window.localStorage); hasPending = !!saver.pending() }
catch (reason) { storageError.value = reason instanceof Error ? reason.message : '无法读取保存恢复记录。' }
const original = shallowRef<Record<string, unknown>>()
const direction = ref('promo'), goal = ref(''), client = ref(''), audience = ref(''), message = ref(''), action = ref('')
const source = shallowRef<CreativeMusicSource | null>(null)
const pending = ref(hasPending), busy = ref(false), hidden = ref(false), error = ref(''), ready = ref(false)
const musicProjects = ref<ProjectItem[]>([]), musicFiles = ref<{ file_id: string; path: string; source: CreativeMusicSource; selected: boolean }[]>([])
const musicCursor = ref(''), selectedProject = ref(''), loadingMusic = ref(false), musicError = ref('')
let alive = true, generation = 0
const sourceName = ref('')
const directions = [{ key: 'promo', label: '宣传片' }, { key: 'advertisement', label: '广告' }, { key: 'brand_film', label: '品牌片' }, { key: 'music_video', label: '音乐驱动视频' }]
const valid = () => alive && !hidden.value
const text = (value: unknown) => typeof value === 'string' ? value : ''

async function loadDocument() {
  const current = ++generation
  ready.value = false; error.value = ''
  try {
    const doc = project.manifest.domain_documents.find(item => item.document_type === 'project-content')
    if (doc) {
      const value = JSON.parse((await props.reader.document(doc.file_id)).text) as Record<string, unknown>
      if (!valid() || current !== generation) return
      original.value = value
      const work = value.creative_video as Record<string, unknown>, brief = work.brief as Record<string, unknown>
      direction.value = text(work.direction) || 'promo'; goal.value = text(brief.goal)
      client.value = text(brief.client); audience.value = text(brief.audience)
      message.value = text(brief.message); action.value = text(brief.action)
      source.value = (work.music_source || null) as CreativeMusicSource | null
    }
    if (valid() && current === generation) ready.value = true
  } catch (reason) { if (valid() && current === generation) error.value = reason instanceof Error ? reason.message : '业务文档读取失败。' }
}

async function save(resume = false) {
  const writer = saver
  if (!writer || busy.value || hidden.value) return
  busy.value = true; error.value = ''
  try {
    const result = resume ? await writer.resume() : await writer.begin(project, original.value, direction.value, goal.value, { client: client.value, audience: audience.value, message: message.value, action: action.value }, source.value)
    if (alive) { pending.value = false; dashboardHost.setDirty(false); emit('saved', result.revision_id) }
  } catch (reason) { if (alive) error.value = reason instanceof Error ? reason.message : '保存结果未确认，请恢复原提交。' }
  finally {
    if (alive) {
      try { pending.value = !!writer.pending() } catch (reason) { storageError.value = reason instanceof Error ? reason.message : '无法读取保存恢复记录。' }
      busy.value = false
    }
  }
}

async function loadMusicProjects(more = false) {
  if (!valid() || loadingMusic.value) return
  loadingMusic.value = true; musicError.value = ''
  try {
    const args: Record<string, unknown> = { category: 'music', limit: 50 }
    if (more && musicCursor.value) args.cursor = musicCursor.value
    const response = await callTool<{ items: ProjectItem[]; next_cursor?: string }>('project.list', args)
    if (!valid()) return
    musicProjects.value = more ? [...musicProjects.value, ...response.items] : response.items
    musicCursor.value = response.next_cursor || ''
  } catch (reason) { if (valid()) musicError.value = reason instanceof Error ? reason.message : '音乐项目读取失败。' }
  finally { if (alive) loadingMusic.value = false }
}

async function openMusicProject(id: string) {
  selectedProject.value = id; musicFiles.value = []; musicError.value = ''; sourceName.value = ''
  if (!id) return
  loadingMusic.value = true
  try {
    const opened = await callTool<ProjectOpen>('project.open', { project_id: id })
    if (!valid() || selectedProject.value !== id) return
    if (opened.project_id !== id || opened.manifest.category !== 'music') throw new Error('来源项目不是音乐项目。')
    const files: typeof musicFiles.value = []
    for (const file of opened.manifest.files) {
      if (!/\.(wav|mp3|flac|m4a|ogg|aac)$/i.test(file.path)) continue
      try {
        const response = await callTool<{ version: { mime: string; size: number } }>('artifact.read', { mode: 'metadata', content_ref: file.content_ref, access: { project_id: id, project_revision_id: opened.revision_id } })
        if (response.version.mime.startsWith('audio/') && response.version.size > 0) {
          const selection = opened.manifest.selections?.find(item => item.file_ids.includes(file.file_id))
          files.push({ file_id: file.file_id, path: file.path, selected: !!selection, source: { kind: 'project_file', project_id: id, revision_id: opened.revision_id, file_id: file.file_id, content_ref: file.content_ref, ...(selection ? { selection_purpose: selection.purpose } : {}) } })
        }
      } catch { /* A listed file may not be readable by this identity. */ }
    }
    if (valid() && selectedProject.value === id) musicFiles.value = files
  } catch (reason) { if (valid() && selectedProject.value === id) musicError.value = reason instanceof Error ? reason.message : '来源项目读取失败。' }
  finally { if (alive) loadingMusic.value = false }
}

function chooseMusic(file: typeof musicFiles.value[number]) {
  source.value = file.source
  sourceName.value = file.path
  dashboardHost.setDirty(true)
}

const stopLifecycle = appSDK.lifecycle.subscribe(event => {
  if (event === 'hidden') hidden.value = true
  if (event === 'active') { hidden.value = false; if (!ready.value && !error.value) void loadDocument() }
  if (event === 'dispose') hidden.value = true
})
watch([direction, goal, client, audience, message, action], () => { if (ready.value && !pending.value) dashboardHost.setDirty(true) })
void loadDocument()
onBeforeUnmount(() => { alive = false; generation++; stopLifecycle(); dashboardHost.setDirty(false) })
</script>

<template>
  <section class="creative-editor">
    <div class="editor-heading"><h2>任务书</h2><div class="commands"><button v-if="pending" class="btn primary" :disabled="busy || hidden || !!storageError" @click="save(true)">恢复原提交</button><button v-else class="btn primary" :disabled="busy || hidden || !!storageError || !ready || !goal.trim()" @click="save()">保存修订</button></div></div>
    <p v-if="storageError" class="notice error" role="alert">{{ storageError }}</p>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="busy" class="notice" role="status">正在确认项目修订…</p>
    <p v-if="pending && !busy" class="notice">有一项保存结果待确认，请恢复原提交后再修改。</p>
    <div v-if="!ready && !error" class="notice" role="status">正在读取任务书…</div>
    <form v-if="ready" class="editor-fields" @submit.prevent="save()">
      <label>创作方向<select v-model="direction" :disabled="busy || pending"><option v-for="item in directions" :key="item.key" :value="item.key">{{ item.label }}</option></select></label>
      <label>传播目标<input v-model="goal" :disabled="busy || pending" maxlength="256" required></label>
      <label>委托方<input v-model="client" :disabled="busy || pending" maxlength="2000"></label>
      <label>目标受众<input v-model="audience" :disabled="busy || pending" maxlength="2000"></label>
      <label>核心信息<input v-model="message" :disabled="busy || pending" maxlength="2000"></label>
      <label>期望行动<input v-model="action" :disabled="busy || pending" maxlength="2000"></label>
    </form>
    <div v-if="ready" class="music-source">
      <h3>音乐项目来源</h3>
      <p v-if="source?.kind === 'project_file'">已固定音乐项目修订 {{ source.revision_id }}{{ sourceName ? ` · ${sourceName}` : '' }}</p>
      <p v-else-if="source?.kind === 'world_asset'">已固定数字世界音乐版本</p>
      <p v-else class="muted">未登记音乐来源</p>
      <div class="commands"><button class="btn" :disabled="busy || pending || loadingMusic" @click="loadMusicProjects()">选择音乐项目</button><button v-if="source" class="btn" :disabled="busy || pending" @click="source = null; dashboardHost.setDirty(true)">解除候选</button></div>
      <div v-if="musicProjects.length || loadingMusic || musicError" class="picker">
        <p v-if="musicError" class="notice error" role="alert">{{ musicError }}</p>
        <label>来源项目<select :value="selectedProject" :disabled="loadingMusic || pending" @change="openMusicProject(($event.target as HTMLSelectElement).value)"><option value="">选择项目</option><option v-for="item in musicProjects" :key="item.project_id" :value="item.project_id">{{ item.name }}</option></select></label>
        <button v-if="musicCursor" class="btn" :disabled="loadingMusic" @click="loadMusicProjects(true)">加载更多</button>
        <p v-if="loadingMusic" role="status">正在核对固定音频文件…</p>
        <p v-else-if="selectedProject && !musicFiles.length" class="muted">当前项目没有可读取的音频文件。</p>
        <div v-for="file in musicFiles" :key="file.file_id" class="source-row"><span>{{ file.path }} · {{ file.selected ? '来源已选用' : '来源未选用' }}</span><button class="btn" :disabled="pending || busy" @click="chooseMusic(file)">选作候选</button></div>
      </div>
      <p v-if="source" class="muted">登记来源仅供策划；正式剪辑与交付须核对固定文件和本项目使用许可。来源注明选用目的时还须核对该选用。</p>
    </div>
  </section>
</template>

<style scoped>
.creative-editor { display:grid; gap:16px; padding:16px 0 26px; border-top:1px solid var(--line); }
.editor-heading,.commands,.source-row { display:flex; justify-content:space-between; align-items:center; gap:12px; flex-wrap:wrap; }
h2 { margin:0; font-size:18px; } h3 { margin:0 0 12px; font-size:15px; }
.editor-fields { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:16px; }
label { display:grid; gap:7px; min-width:0; color:var(--muted); }
input,select { min-width:0; width:100%; min-height:38px; border:1px solid var(--line); border-radius:6px; padding:7px 10px; background:var(--panel); color:var(--ink); }
.music-source { padding-top:18px; border-top:1px solid var(--line); overflow-wrap:anywhere; }
.music-source p { margin:0 0 12px; }.picker { display:grid; gap:12px; padding-top:15px; }
.source-row { border-bottom:1px solid var(--line); padding:9px 0; }.source-row span { min-width:0; overflow-wrap:anywhere; }
.notice { padding:10px 12px; border-left:2px solid var(--line); background:var(--subtle); margin:0; }.error { color:var(--danger); }
.muted { color:var(--muted); }
@media(max-width:700px) { .editor-fields { grid-template-columns:minmax(0,1fr); } }
</style>
