<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ProjectReader, type Detail, type ProjectItem } from '../platform/project-reader'
import { visibleMusicVersions, currentMusicVersion, musicEntry, musicSourceEntry, musicVersionKinds, musicReview, type MusicGroup, type MusicSource, type MusicVersion } from '../platform/music-production'
import { contentChanges, createProjectEditor, verifyEditReadback, type EditState } from '../platform/project-edit'
import { collectProjects } from '../platform/project-management'
import { callTool } from '../platform/mcp'
import { useHost } from '../platform/host'
import ProjectMedia from './ProjectMedia.vue'
import MusicFixedSource from './MusicFixedSource.vue'
const props = defineProps<{ reader: ProjectReader; detail: Detail; group: MusicGroup }>()
const emit = defineEmits<{ saved: [revision: string] }>()
const route = useRoute(), router = useRouter(), { appSDK, dashboardHost } = useHost()
const allVersions = computed(() => props.detail.production?.versions || [])
const versions = computed(() => visibleMusicVersions(props.detail.production))
const requested = computed(() => typeof route.query.version === 'string' ? route.query.version : '')
const version = computed(() => currentMusicVersion(props.detail.production, requested.value))
const archived = computed(() => !!requested.value && allVersions.value.some(v => v.key === requested.value && v.review.state === 'rejected'))
const missing = computed(() => !!requested.value && !version.value)
const baseReader = shallowRef(props.reader)
const canWrite = computed(() => baseReader.value.project?.can_write === true && baseReader.value.project.revision_id === baseReader.value.project.head_revision_id)
const selected = computed(() => versions.value.find(v => v.key === props.detail.production?.selected_version_key))
const legacy = computed(() => !allVersions.value.length && !requested.value)
const dialog = ref<HTMLDialogElement>(), mode = ref<'register' | 'select'>('register'), error = ref(''), busy = ref(false), hidden = ref(false), pending = ref(false), state = ref<EditState>('unknown')
const label = ref(''), note = ref(''), parent = ref(''), kind = ref<MusicVersion['kind']>(props.group === 'voice' ? 'model' : props.group === 'mvs' ? 'video' : 'demo')
const selectionKey = ref('')
const output = ref(''), samples = ref<string[]>([]), attachments = ref<string[]>([])
const projects = ref<ProjectItem[]>([]), sourceProject = ref(''), sourceKind = ref<MusicSource['kind']>(props.group === 'voice' ? 'recording' : props.group === 'mvs' ? 'song' : 'model'), sourceItem = ref(''), sourceOutput = ref('')
const sourceReader = shallowRef<ProjectReader>(), sourceBusy = ref(false), sourceError = ref(''), sources = ref<MusicSource[]>([])
const fileOptions = computed(() => baseReader.value.project!.manifest.files.filter(f => props.group === 'voice' || (props.group === 'mvs' ? /\.(mp4|webm|mov|mkv)$/i : /\.(mp3|wav|flac|ogg|m4a|aac)$/i).test(f.path)))
const audioOptions = computed(() => baseReader.value.project!.manifest.files.filter(f => /\.(mp3|wav|flac|ogg|m4a|aac)$/i.test(f.path)))
const sourceItems = computed(() => sourceKind.value === 'recording' ? sourceReader.value?.content?.music?.voice.recordings : sourceKind.value === 'model' ? sourceReader.value?.content?.music?.voice.models : sourceReader.value?.content?.music?.songs)
const sourceEntry = computed(() => sourceItems.value?.find(e => e.key === sourceItem.value))
const sourceOutputs = computed(() => [
  ...visibleMusicVersions(sourceEntry.value?.production).map(v => ({ key: 'version:' + v.key, name: `${v.label} · ${musicVersionKinds[v.kind]} · ${musicReview(v.review)}` })),
  ...(sourceEntry.value?.production?.versions.length ? [] : [...new Set([...(sourceEntry.value?.result_file_ids || []), ...(sourceEntry.value?.file_ids || [])])]).map(id => ({ key: 'file:' + id, name: `${sourceReader.value?.file(id).path} · 历史文件（版本与审核未登记）` })),
])
let alive = true, generation = 0, editor: ReturnType<typeof createProjectEditor> | undefined
let focusBefore: HTMLElement | undefined
function sync() { pending.value = !!editor?.pending(); state.value = editor?.state() || 'unknown' }
function restoreDraft() {
  const saved = editor?.pending()?.request.changes.upsert_files?.[0]
  if (!saved) return
  const item = musicEntry(JSON.parse(saved.text).music, props.group, props.detail.key), production = item?.production
  const added = production?.versions.find(v => !allVersions.value.some(old => old.key === v.key))
  if (added) {
    mode.value = 'register'; label.value = added.label; note.value = added.change_note; parent.value = added.parent_version_key || ''; kind.value = added.kind
    output.value = added.file_refs.find(f => f.purpose === 'output')?.file_id || ''
    samples.value = added.file_refs.filter(f => f.purpose === 'sample').map(f => f.file_id)
    attachments.value = added.file_refs.filter(f => f.purpose === 'attachment').map(f => f.file_id)
    sources.value = structuredClone(added.sources)
  } else if (production?.selected_version_key) { mode.value = 'select'; selectionKey.value = production.selected_version_key }
}
function close() { if (!busy.value) { dialog.value?.close(); sourceReader.value?.dispose(); sourceReader.value = undefined; generation++; sourceBusy.value = false; focusBefore?.focus() } }
async function open(value: 'register' | 'select') {
  if (!canWrite.value || busy.value) return
  error.value = ''; mode.value = value; focusBefore = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
  try {
    editor?.dispose(); editor = createProjectEditor(appSDK, window.localStorage, props.reader.project!.project_id); sync(); selectionKey.value = version.value?.key || ''
    if (pending.value) restoreDraft()
    dialog.value?.showModal()
    if (value === 'register' && !pending.value) {
      label.value = ''; note.value = ''; parent.value = version.value?.key || ''; output.value = ''; samples.value = []; attachments.value = []; sources.value = []
      sourceProject.value = ''; sourceItem.value = ''; sourceOutput.value = ''; sourceError.value = ''; sourceReader.value?.dispose(); sourceReader.value = undefined
      sourceBusy.value = true; const token = ++generation
      try { const list = await collectProjects('music', args => callTool('project.list', args), () => alive && !hidden.value && token === generation); if (alive && token === generation) projects.value = list }
      catch (cause) { if (alive && token === generation) sourceError.value = cause instanceof Error ? cause.message : '无法读取音乐来源项目。' }
      finally { if (alive && token === generation) sourceBusy.value = false }
    }
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '无法打开版本编辑。' }
}
async function readSource() {
  generation++; sourceReader.value?.dispose(); sourceReader.value = undefined; sourceItem.value = ''; sourceOutput.value = ''; sourceError.value = ''; sourceBusy.value = false
  if (!sourceProject.value || hidden.value) return
  const token = generation, next = new ProjectReader(sourceProject.value); sourceBusy.value = true
  try { await next.load(); if (next.project?.manifest.category !== 'music') throw Error('来源不是音乐项目。'); if (!alive || token !== generation) { next.dispose(); return }; sourceReader.value = next }
  catch (cause) { next.dispose(); if (alive && token === generation) sourceError.value = cause instanceof Error ? cause.message : '无法读取来源。' }
  finally { if (alive && token === generation) sourceBusy.value = false }
}
function addSource() {
  if (!sourceReader.value?.project || !sourceItem.value || !sourceOutputs.value.some(o => o.key === sourceOutput.value)) return
  const isVersion = sourceOutput.value.startsWith('version:'), value = sourceOutput.value.slice(sourceOutput.value.indexOf(':') + 1)
  const source: MusicSource = { project_id: sourceReader.value.project.project_id, revision_id: sourceReader.value.project.revision_id, kind: sourceKind.value, item_key: sourceItem.value, version_key: isVersion ? value : null, file_id: isVersion ? null : value }
  if (!sources.value.some(s => JSON.stringify(s) === JSON.stringify(source))) sources.value.push(source)
}
async function changes() {
  const p = baseReader.value.project!, doc = p.manifest.domain_documents.find(d => d.document_type === 'project-content')
  if (!doc) throw Error('当前项目未登记业务内容。')
  const raw = JSON.parse((await baseReader.value.document(doc.file_id)).text), item = musicEntry(raw.music, props.group, props.detail.key)
  if (!item) throw Error('当前修订没有此作品。')
  item.production ||= { versions: [], selected_version_key: null }
  if (mode.value === 'select') {
    if (!selectionKey.value || !visibleMusicVersions(item.production).some(v => v.key === selectionKey.value)) throw Error('当前查看版本不可选用。')
    item.production.selected_version_key = selectionKey.value
  } else {
    if (!label.value.trim() || !note.value.trim() || !output.value) throw Error('请填写版本名称、修改说明并选择已有成果。')
    const fixedSources = sources.value.slice()
    for (const source of fixedSources) {
      const r = new ProjectReader(source.project_id, source.revision_id)
      try { await r.load(); if (r.project?.manifest.category !== 'music') throw Error('来源不是音乐项目。'); musicSourceEntry(r.content?.music, source) } finally { r.dispose() }
    }
    const refs = [{ id: output.value, purpose: 'output' as const }, ...samples.value.map(id => ({ id, purpose: 'sample' as const })), ...attachments.value.map(id => ({ id, purpose: 'attachment' as const }))]
    if (new Set(refs.map(f => f.id)).size !== refs.length) throw Error('同一文件请只登记一次用途。')
    item.production.versions.push({ key: 'version_' + crypto.randomUUID().replaceAll('-', ''), label: label.value.trim(), kind: kind.value, basis_revision_id: p.revision_id, parent_version_key: parent.value || null, change_note: note.value.trim(), file_refs: refs.map(r => ({ file_id: r.id, content_ref: { ...baseReader.value.file(r.id).content_ref }, purpose: r.purpose })), review: { state: 'unreviewed', record_file_id: null }, sources: fixedSources })
  }
  return contentChanges(p, JSON.stringify(raw))
}
async function save(status = false) {
  if (!editor || busy.value || hidden.value) return
  busy.value = true; error.value = ''; dashboardHost.setDirty(true)
  try {
    if (!editor.pending()) { if (!canWrite.value) throw Error('请在有编辑权限的最新修订中修改。'); await editor.begin(baseReader.value.project!.revision_id, await changes()) }
    sync(); const result = await (status ? editor.status() : editor.submit()); sync()
    if (result.state === 'committed' && result.revision) {
      const verify = new ProjectReader(props.reader.project!.project_id, result.revision)
      try { await verify.load(); await verifyEditReadback(editor.pending()!.request, verify.project!, id => verify.document(id)) } finally { verify.dispose() }
      await editor.clearTerminal(); sync(); if (alive) { dialog.value?.close(); emit('saved', result.revision) }
    }
  } catch (cause) { if (alive) error.value = cause instanceof Error ? cause.message : '未确认保存结果，原请求已保留。' }
  finally { if (alive) { busy.value = false; sync(); dashboardHost.setDirty(false) } }
}
async function refreshAfterConflict() {
  if (!editor || busy.value || hidden.value) return
  busy.value = true; error.value = ''
  const next = new ProjectReader(props.reader.project!.project_id)
  try {
    await next.load()
    if (!alive || hidden.value) { next.dispose(); return }
    if (!next.project?.can_write || !musicEntry(next.content?.music, props.group, props.detail.key)) throw Error('最新修订没有可编辑的此作品。')
    await editor.clearTerminal()
    if (baseReader.value !== props.reader) baseReader.value.dispose()
    baseReader.value = next; sync(); error.value = '已读取最新修订，填写内容已保留；请核对文件和来源后重新保存。'
  } catch (cause) { next.dispose(); if (alive) error.value = cause instanceof Error ? cause.message : '无法读取最新修订。' }
  finally { if (alive) busy.value = false }
}
const stop = appSDK.lifecycle.subscribe(event => { if (event === 'hidden') { hidden.value = true; generation++; sourceReader.value?.dispose(); sourceReader.value = undefined; sourceBusy.value = false }; if (event === 'active') hidden.value = false; if (event === 'dispose') { alive = false; generation++; editor?.dispose(); sourceReader.value?.dispose() } })
onBeforeUnmount(() => { alive = false; generation++; editor?.dispose(); sourceReader.value?.dispose(); if (baseReader.value !== props.reader) baseReader.value.dispose(); stop() })
</script>

<template>
  <section class="panel music-production">
    <header><h2>{{ group === 'voice' ? '模型版本与试听' : group === 'songs' ? '音频成果与版本' : 'MV 成果与版本' }}</h2><button v-if="canWrite" class="btn" :disabled="busy" @click="open('register')">登记已有制作版本</button></header>
    <p>实际选用：{{ selected?.label || '未选定' }}</p>
    <label v-if="versions.length">当前查看 <select aria-label="当前查看制作版本" :value="version?.key || requested" @change="router.push({ path: route.path, query: { ...route.query, version: ($event.target as HTMLSelectElement).value } })"><option v-if="missing" :value="requested">当前修订缺少此版本</option><option v-for="v in versions" :key="v.key" :value="v.key">{{ v.label }} · {{ musicVersionKinds[v.kind] }}</option></select></label>
    <p v-if="archived" role="status">此版本已归档，不作为当前试听。</p>
    <p v-else-if="missing" role="status">当前固定修订没有此制作版本，不以其他版本替代。</p>
    <template v-else-if="version">
      <p>{{ musicVersionKinds[version.kind] }} · {{ musicReview(version.review) }}</p>
      <div class="outputs"><ProjectMedia v-for="f in version.file_refs.filter(f => f.purpose === (group === 'voice' ? 'sample' : 'output'))" :key="f.file_id" :reader="reader" :file-id="f.file_id" :kind="group === 'mvs' ? 'video' : 'audio'" :label="version.label" /></div>
      <p v-if="group === 'voice' && !version.file_refs.some(f => f.purpose === 'sample')" class="muted">此模型未登记试听；干声录音不代替模型效果。</p>
      <button v-if="canWrite && version.key !== selected?.key" class="btn" @click="open('select')">选用当前版本</button>
      <h3>本轮修改</h3><dl><dt>父版本</dt><dd>{{ versions.find(v => v.key === version?.parent_version_key)?.label || '未登记' }}</dd><dt>修改内容</dt><dd class="prose">{{ version.change_note }}</dd><dt>依据修订</dt><dd><RouterLink :to="{ path: '/projects', query: { category: 'music', project: reader.project!.project_id, revision: version.basis_revision_id, tab: 'revisions', fixed: version.basis_revision_id } }">查看对应固定修订</RouterLink></dd></dl>
      <h3>{{ group === 'voice' ? '本人干声来源' : group === 'songs' ? '人声来源' : '音乐来源' }}</h3>
      <MusicFixedSource v-for="(source, i) in version.sources" :key="i" :source="source" :owner-revision="reader.project!.revision_id" /><p v-if="!version.sources.length" class="muted">尚未登记固定来源。</p>
      <h3>版本文件</h3><ProjectMedia v-for="f in version.file_refs" :key="f.file_id" :reader="reader" :file-id="f.file_id" kind="file" :label="reader.file(f.file_id).path" />
      <RouterLink v-if="version.review.record_file_id" class="btn" :to="{ path: route.path, query: { ...route.query, tab: 'files', item: undefined, version: undefined, file: version.review.record_file_id } }">查看审核记录</RouterLink>
    </template>
    <template v-else-if="legacy">
      <p class="muted">尚未登记制作版本、选用及审核记录。{{ group === 'songs' ? '未添加母带。' : group === 'mvs' ? '尚未登记固定音乐来源。' : '尚未登记固定干声来源。' }}</p>
      <ProjectMedia v-for="id in detail.result_file_ids" :key="id" :reader="reader" :file-id="id" :kind="group === 'mvs' ? 'video' : 'audio'" :label="detail.title + (group === 'voice' ? '试听' : '历史成果')" />
      <p v-if="!detail.result_file_ids.length" class="muted">{{ group === 'voice' ? '此模型未登记试听；干声录音不代替模型效果。' : '未添加成果。' }}</p>
    </template>
    <p v-if="!requested && allVersions.length && !versions.length" role="status">暂无可试听版本。</p>
    <p v-if="group === 'songs' && versions.length && !versions.some(v => v.kind === 'master')" class="muted">未添加母带。</p>
    <p v-if="error && !dialog?.open" role="alert">{{ error }}</p>
  </section>
  <dialog ref="dialog" aria-labelledby="music-production-title" @cancel.prevent="close" @click="event => { if (event.target === dialog) close() }">
    <form @submit.prevent="save()"><header><h2 id="music-production-title">{{ pending ? '恢复原保存' : mode === 'select' ? '选用当前版本' : '登记已有制作版本' }}</h2><button type="button" class="btn" :disabled="busy" @click="close">关闭</button></header>
      <p v-if="error" role="alert">{{ error }}</p>
      <fieldset v-if="!pending && mode === 'register'" :disabled="busy">
        <label>版本名称<input v-model="label" required maxlength="256"></label>
        <label>类型<select v-model="kind"><option v-if="group === 'voice'" value="model">人声模型</option><option v-if="group === 'mvs'" value="video">MV</option><template v-if="group === 'songs'"><option value="demo">Demo</option><option value="audio">音频版本</option><option value="master">母带</option></template></select></label>
        <label>父版本<select v-model="parent"><option value="">无父版本</option><option v-for="v in versions" :key="v.key" :value="v.key">{{ v.label }}</option></select></label>
        <label>修改说明<textarea v-model="note" required rows="3" /></label>
        <label>已有成果<select v-model="output" required><option value="">请选择当前项目文件</option><option v-for="f in fileOptions" :key="f.file_id" :value="f.file_id">{{ f.path }}</option></select></label>
        <label v-if="group === 'voice'">模型试听<select v-model="samples" multiple><option v-for="f in audioOptions" :key="f.file_id" :value="f.file_id">{{ f.path }}</option></select></label>
        <label>工程与附件<select v-model="attachments" multiple><option v-for="f in baseReader.project!.manifest.files" :key="f.file_id" :value="f.file_id">{{ f.path }}</option></select></label>
        <fieldset><legend>固定来源（可稍后补充到新版本）</legend>
          <label>音乐项目<select v-model="sourceProject" :disabled="sourceBusy" @change="readSource"><option value="">请选择来源项目</option><option v-for="p in projects" :key="p.project_id" :value="p.project_id">{{ p.name }}</option></select></label>
          <label>来源类型<select v-model="sourceKind" @change="sourceItem = ''; sourceOutput = ''"><option v-if="group !== 'mvs'" value="recording">干声录音</option><option v-if="group === 'songs'" value="model">人声模型</option><option v-if="group === 'mvs'" value="song">歌曲</option></select></label>
          <p v-if="sourceBusy" role="status">正在读取来源…</p><p v-if="sourceError" role="alert">{{ sourceError }}</p>
          <label>业务对象<select v-model="sourceItem" @change="sourceOutput = ''"><option value="">请选择</option><option v-for="e in sourceItems" :key="e.key" :value="e.key">{{ e.title }}</option></select></label>
          <label>固定成果<select v-model="sourceOutput"><option value="">请选择版本或历史文件</option><option v-for="o in sourceOutputs" :key="o.key" :value="o.key">{{ o.name }}</option></select></label>
          <button type="button" class="btn" :disabled="!sourceOutput || sourceBusy" @click="addSource">加入来源</button>
          <ul><li v-for="(s, i) in sources" :key="i">{{ s.kind }} · {{ s.item_key }} · {{ s.version_key || s.file_id }} <button type="button" class="btn" @click="sources.splice(i, 1)">移除</button></li></ul>
        </fieldset><p class="muted">只登记已有文件，保存后保持未审核。</p>
      </fieldset>
      <p v-else-if="!pending">选用“{{ versions.find(v => v.key === selectionKey)?.label || selectionKey }}”作为此作品的当前成果，审核状态保持原记录。</p>
      <p v-if="pending" class="muted">已保存原请求，查询或恢复时复用同一请求。</p>
      <footer><button v-if="pending" type="button" class="btn" :disabled="busy || hidden" @click="save(true)">查询原保存</button><button v-if="state === 'conflict' || state === 'failed'" type="button" class="btn" :disabled="busy || hidden" @click="refreshAfterConflict">保留输入，读取最新修订</button><button class="btn primary" type="submit" :disabled="busy || hidden || state === 'conflict' || state === 'failed'">{{ busy ? '正在保存…' : pending ? '恢复原保存' : '保存' }}</button></footer>
    </form>
  </dialog>
</template>
<style scoped>
.music-production,.outputs { display:grid; gap:16px; min-width:0; }.panel { padding:20px; border:1px solid var(--line); border-radius:12px; background:var(--shell-panel); overflow-wrap:anywhere; }header,footer { display:flex; justify-content:space-between; align-items:center; gap:12px; flex-wrap:wrap; }h2 { font-size:18px; margin:0; }h3 { font-size:16px; margin:8px 0 0; }.muted,dt { color:var(--muted); }label { display:grid; gap:8px; }input,select,textarea { max-width:100%; min-width:0; padding:8px 12px; border:1px solid var(--line); border-radius:8px; color:var(--ink); background:var(--panel); font:inherit; }dl { display:grid; grid-template-columns:80px minmax(0,1fr); gap:12px; }.prose { white-space:pre-wrap; }dialog { width:min(680px,calc(100vw - 32px)); max-height:90vh; overflow:auto; margin:auto; padding:24px; color:var(--ink); background:var(--panel); border:1px solid var(--line); border-radius:12px; }dialog::backdrop { background:color-mix(in srgb, var(--paper) 55%, transparent); }form { display:grid; gap:16px; }fieldset { min-width:0; padding:16px; border:1px solid var(--line); display:grid; gap:12px; }[role=alert] { color:var(--danger); }
</style>
