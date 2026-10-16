<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef } from 'vue'
import { createCreativeSaver } from '../../platform/creative-save'
import type { ProjectReader } from '../../platform/project-reader'
import { useHost } from '../../platform/host'

const props = defineProps<{ reader: ProjectReader }>()
const emit = defineEmits<{ saved: [revision: string] }>()
const { appSDK } = useHost()
const project = props.reader.project!
const original = shallowRef<Record<string, any>>()
const ready = ref(false), pending = ref(false), busy = ref(false), hidden = ref(false), error = ref('')
let saver: ReturnType<typeof createCreativeSaver> | null = null, alive = true
try { saver = createCreativeSaver(appSDK, project.project_id, window.localStorage); pending.value = !!saver.pending() }
catch (reason) { error.value = reason instanceof Error ? reason.message : '无法读取保存恢复记录。' }
const work = computed(() => original.value?.creative_video as Record<string, any> | undefined)
const selectedTakes = computed(() => (work.value?.takes || []).filter((take: any) => take.status === 'selected' && take.file_id && project.manifest.files.some(file => file.file_id === take.file_id)))
const files = computed(() => project.manifest.files.filter(file => !project.manifest.domain_documents.some(doc => doc.file_id === file.file_id)))
const key = ref(''), parent = ref(''), projectFile = ref(''), fpsNum = ref(25), fpsDen = ref(1), useMusic = ref(false)
const clips = ref<{ take_key: string; source_in_frame: number; source_out_frame: number }[]>([{ take_key: '', source_in_frame: 0, source_out_frame: 25 }])
const source = computed(() => work.value?.music_source as { kind: string; file_id: string; selection_purpose?: string } | null)
const sourceKey = ref('')
const musicRight = computed(() => (work.value?.rights || []).find((right: any) => right.subject_kind === 'music' && right.subject_key === sourceKey.value && right.status === 'cleared' && right.evidence_file_id && right.uses?.includes('music_sync') && right.channels?.length && right.territories?.length && (!right.expires_on || right.expires_on >= new Date().toISOString().slice(0, 10))))
const musicEvidence = ref(''), channels = ref(''), territories = ref(''), expiresOn = ref('')
const keyOK = (value: string) => /^[A-Za-z0-9_-]{1,96}$/.test(value)

async function identifySource() {
  const music = source.value as Record<string, any> | null
  if (!music) { sourceKey.value = ''; return }
  const parts = [music.kind, music.project_id, music.revision_id, music.asset_id, music.asset_version_id, music.file_id, music.content_ref?.store_id, music.content_ref?.artifact_id, music.content_ref?.version_id]
  const bytes = new TextEncoder().encode(parts.map(value => value || '').join('|'))
  const hash = await crypto.subtle.digest('SHA-256', bytes)
  sourceKey.value = [...new Uint8Array(hash)].map(value => value.toString(16).padStart(2, '0')).join('')
}

async function load() {
  try {
    const doc = project.manifest.domain_documents.find(item => item.document_type === 'project-content')
    if (!doc) throw new Error('请先建立创意视频业务文档。')
    const read = await props.reader.document(doc.file_id)
    if (!alive || hidden.value) return
    const value = JSON.parse(read.text)
    if (value.category !== 'creative_video' || !value.creative_video) throw new Error('创意视频业务文档格式不正确。')
    original.value = value; ready.value = true; await identifySource()
  } catch (reason) { if (alive && !hidden.value) error.value = reason instanceof Error ? reason.message : '剪辑内容读取失败。' }
}

async function change(update: (work: Record<string, any>) => void) {
  const writer = saver
  if (!writer || !original.value || busy.value || hidden.value || pending.value) return
  busy.value = true; error.value = ''
  try {
    const document = structuredClone(original.value)
    update(document.creative_video)
    const result = await writer.beginDocument(project, document)
    if (alive) emit('saved', result.revision_id)
  } catch (reason) { if (alive) error.value = reason instanceof Error ? reason.message : '保存结果未确认，请恢复原提交。' }
  finally { if (alive) { try { pending.value = !!writer.pending() } catch { error.value = '无法读取保存恢复记录。' }; busy.value = false } }
}
async function recover() {
  const writer = saver
  if (!writer || busy.value || hidden.value) return
  busy.value = true; error.value = ''
  try { const result = await writer.resume(); if (alive) emit('saved', result.revision_id) }
  catch (reason) { if (alive) error.value = reason instanceof Error ? reason.message : '恢复结果未确认。' }
  finally { if (alive) { try { pending.value = !!writer.pending() } catch { error.value = '无法读取保存恢复记录。' }; busy.value = false } }
}

function addClip() { clips.value.push({ take_key: '', source_in_frame: 0, source_out_frame: 25 }) }
function addEdit() {
  const editKey = key.value.trim()
  if (!keyOK(editKey) || work.value?.edits.some((item: any) => item.key === editKey)) { error.value = '剪辑版本键须唯一，且只含字母、数字、下划线或连字符。'; return }
  if (![fpsNum.value, fpsDen.value].every(value => Number.isSafeInteger(value) && value > 0)) { error.value = '帧率分子和分母必须为正整数。'; return }
  if (!clips.value.length || clips.value.some(clip => !selectedTakes.value.some((take: any) => take.key === clip.take_key) || !Number.isSafeInteger(clip.source_in_frame) || !Number.isSafeInteger(clip.source_out_frame) || clip.source_in_frame < 0 || clip.source_out_frame <= clip.source_in_frame)) { error.value = '请逐项选择已选镜头并填写有效的源入/出帧。'; return }
  if (useMusic.value && (source.value?.kind !== 'project_file' || !musicRight.value)) { error.value = '音乐尚无本项目清权证据。'; return }
  void change(work => {
    let cursor = 0
    const items = clips.value.map(clip => {
      const take = work.takes.find((item: any) => item.key === clip.take_key)
      const file = project.manifest.files.find(item => item.file_id === take.file_id)!
      const item = { take_key: clip.take_key, media_ref: { basis_revision_id: project.revision_id, file_id: file.file_id, content_ref: file.content_ref }, source_in_frame: clip.source_in_frame, source_out_frame: clip.source_out_frame, timeline_in_frame: cursor }
      cursor += clip.source_out_frame - clip.source_in_frame
      return item
    })
    work.edits.push({ key: editKey, parent_key: parent.value || null, project_file_id: projectFile.value || null, music_source: useMusic.value ? structuredClone(work.music_source) : null, fps: { num: fpsNum.value, den: fpsDen.value }, items, state: 'rough' })
  })
}
function advanceEdit(editKey: string, state: 'review' | 'picture_locked') {
  void change(work => { work.edits.find((item: any) => item.key === editKey).state = state })
}
function recordMusicRight() {
  const music = source.value
  const split = (value: string) => value.split(/[,，\s]+/).map(item => item.trim()).filter(Boolean)
  const allowed = /^[A-Za-z0-9_-]{1,96}$/
  const channelList = split(channels.value), territoryList = split(territories.value)
  if (!music || !sourceKey.value || !musicEvidence.value || !channelList.length || !territoryList.length || [...channelList, ...territoryList].some(item => !allowed.test(item)) || (expiresOn.value && expiresOn.value < new Date().toISOString().slice(0, 10))) { error.value = '请选择有效的授权文件、渠道、地域和期限。'; return }
  void change(work => {
    const current = work.rights.find((right: any) => right.subject_kind === 'music' && right.subject_key === sourceKey.value)
    const clearance = { status: 'cleared', evidence_file_id: musicEvidence.value, channels: channelList, territories: territoryList, uses: ['music_sync'], expires_on: expiresOn.value || null }
    if (current) Object.assign(current, clearance)
    else work.rights.push({ key: `music-${sourceKey.value}`, subject_kind: 'music', subject_key: sourceKey.value, ...clearance })
  })
}

const stop = appSDK.lifecycle.subscribe(event => {
  if (event === 'hidden') hidden.value = true
  if (event === 'active') { hidden.value = false; if (!ready.value && !error.value) void load() }
  if (event === 'dispose') hidden.value = true
})
void load()
onBeforeUnmount(() => { alive = false; stop() })
</script>

<template>
  <section class="edit-editor">
    <div class="head"><h2>剪辑版本</h2><button v-if="pending" class="btn primary" :disabled="busy || hidden" @click="recover">恢复原提交</button></div>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p><p v-if="busy" class="notice" role="status">正在确认项目修订…</p><p v-if="pending && !busy" class="notice">有一项保存结果待确认，请恢复原提交。</p><p v-if="!ready && !error" role="status">正在读取剪辑内容…</p>
    <template v-if="ready">
      <div v-if="source" class="block"><h3>音乐使用</h3><p>{{ source.kind === 'project_file' ? '已固定来源音乐项目的音频版本' : '已关联数字世界音乐候选' }} · {{ musicRight ? '本项目已有清权记录' : '本项目尚无清权记录' }}</p><div v-if="!musicRight" class="fields"><label>授权文件<select v-model="musicEvidence" :disabled="pending || busy"><option value="">选择文件</option><option v-for="file in files" :key="file.file_id" :value="file.file_id">{{ file.path }}</option></select></label><label>渠道代码<input v-model="channels" :disabled="pending || busy" placeholder="web social"></label><label>地域代码<input v-model="territories" :disabled="pending || busy" placeholder="CN"></label><label>有效期<input v-model="expiresOn" type="date" :disabled="pending || busy"></label></div><button v-if="!musicRight" class="btn" :disabled="pending || busy || !musicEvidence" @click="recordMusicRight">登记清权证据</button></div>
      <div class="block"><h3>新剪辑版本</h3><div class="fields"><label>版本键<input v-model="key" :disabled="pending || busy" placeholder="edit-v1"></label><label>基于版本<select v-model="parent" :disabled="pending || busy"><option value="">首版</option><option v-for="edit in work?.edits" :key="edit.key" :value="edit.key">{{ edit.key }}</option></select></label><label>剪辑工程<select v-model="projectFile" :disabled="pending || busy"><option value="">暂无工程文件</option><option v-for="file in files" :key="file.file_id" :value="file.file_id">{{ file.path }}</option></select></label><label>帧率分子<input v-model.number="fpsNum" type="number" min="1" step="1" :disabled="pending || busy"></label><label>帧率分母<input v-model.number="fpsDen" type="number" min="1" step="1" :disabled="pending || busy"></label></div>
        <div v-for="(clip, index) in clips" :key="index" class="clip-row"><label>镜头候选<select v-model="clip.take_key" :disabled="pending || busy"><option value="">选择已选候选</option><option v-for="take in selectedTakes" :key="take.key" :value="take.key">{{ take.shot_key }} / {{ take.key }}</option></select></label><label>源入帧<input v-model.number="clip.source_in_frame" type="number" min="0" step="1" :disabled="pending || busy"></label><label>源出帧<input v-model.number="clip.source_out_frame" type="number" min="1" step="1" :disabled="pending || busy"></label><button class="btn" :disabled="pending || busy || clips.length === 1" title="移除片段" @click="clips.splice(index, 1)">移除</button></div>
        <div class="commands"><button class="btn" :disabled="pending || busy" @click="addClip">添加片段</button><label v-if="source?.kind === 'project_file'" class="music-toggle"><input v-model="useMusic" type="checkbox" :disabled="pending || busy || !musicRight">使用固定音乐</label><button class="btn primary" :disabled="pending || busy || !key.trim() || !selectedTakes.length" @click="addEdit">保存新版本</button></div>
      </div>
      <div v-if="work?.edits.length" class="block"><h3>版本状态</h3><div v-for="edit in work.edits" :key="edit.key" class="line"><span>{{ edit.key }} · {{ edit.state || '粗剪' }} · {{ edit.items.length }} 个片段</span><div class="commands"><button v-if="edit.state === 'rough'" class="btn" :disabled="pending || busy" @click="advanceEdit(edit.key, 'review')">送审</button><button v-if="edit.state === 'review'" class="btn" :disabled="pending || busy" @click="advanceEdit(edit.key, 'picture_locked')">锁片</button></div></div></div>
    </template>
  </section>
</template>

<style scoped>
.edit-editor { display:grid; gap:18px; min-width:0; padding:16px 0 24px; border-top:1px solid var(--line); }.head,.commands,.line { display:flex; align-items:center; justify-content:space-between; flex-wrap:wrap; gap:10px; }
h2 { margin:0; font-size:18px; } h3 { margin:0 0 14px; font-size:15px; }.block { padding-top:16px; border-top:1px solid var(--line); }.block p { color:var(--muted); }
.fields { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:14px; margin-bottom:16px; }.clip-row { display:grid; grid-template-columns:minmax(0,2fr) minmax(90px,1fr) minmax(90px,1fr) auto; align-items:end; gap:10px; padding:10px 0; border-top:1px solid var(--line); }
label { display:grid; gap:6px; min-width:0; color:var(--muted); }input,select { width:100%; min-width:0; min-height:38px; border:1px solid var(--line); border-radius:6px; padding:7px 10px; background:var(--panel); color:var(--ink); }.music-toggle { display:flex; align-items:center; gap:7px; }.music-toggle input { width:16px; min-height:16px; }.line { padding:10px 0; border-bottom:1px solid var(--line); }.notice { padding:10px 12px; border-left:2px solid var(--line); background:var(--subtle); }.error { color:var(--danger); }
@media(max-width:700px) { .fields,.clip-row { grid-template-columns:minmax(0,1fr); } }
</style>
