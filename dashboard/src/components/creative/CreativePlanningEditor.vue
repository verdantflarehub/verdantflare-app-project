<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef } from 'vue'
import { createCreativeSaver } from '../../platform/creative-save'
import type { ProjectReader } from '../../platform/project-reader'
import { useHost } from '../../platform/host'

const props = defineProps<{ reader: ProjectReader; view: 'story' | 'shots' }>()
const emit = defineEmits<{ saved: [revision: string] }>()
const { appSDK } = useHost()
const project = props.reader.project!
const original = shallowRef<Record<string, any>>()
const ready = ref(false), pending = ref(false), busy = ref(false), hidden = ref(false), error = ref('')
let saver: ReturnType<typeof createCreativeSaver> | null = null, alive = true
try { saver = createCreativeSaver(appSDK, project.project_id, window.localStorage); pending.value = !!saver.pending() }
catch (reason) { error.value = reason instanceof Error ? reason.message : '无法读取保存恢复记录。' }

const conceptKey = ref(''), conceptFile = ref('')
const sequenceKey = ref(''), sequenceLabel = ref(''), shotKey = ref(''), plannedFrames = ref(75), storyboardFile = ref('')
const takeShot = ref(''), takeKey = ref(''), takeFile = ref(''), takeRun = ref('')
const takeAssignments = ref<Record<string, string>>({})
const work = computed(() => original.value?.creative_video as Record<string, any> | undefined)
const files = computed(() => project.manifest.files.filter(file => !project.manifest.domain_documents.some(doc => doc.file_id === file.file_id)))
const videoFiles = computed(() => files.value.filter(file => /\.(mp4|mov|webm|mkv)$/i.test(file.path)))
const runs = project.manifest.run_refs || []
const keyOK = (key: string) => /^[A-Za-z0-9_-]{1,96}$/.test(key)

async function load() {
  try {
    const doc = project.manifest.domain_documents.find(item => item.document_type === 'project-content')
    if (!doc) throw new Error('请先在任务书页建立业务文档。')
    const read = await props.reader.document(doc.file_id)
    if (!alive || hidden.value) return
    const value = JSON.parse(read.text)
    if (value.category !== 'creative_video' || !value.creative_video) throw new Error('创意视频业务文档格式不正确。')
    original.value = value; ready.value = true
    takeShot.value ||= value.creative_video.shots?.[0]?.key || ''
  } catch (reason) { if (alive && !hidden.value) error.value = reason instanceof Error ? reason.message : '业务文档读取失败。' }
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

function addConcept() {
  const key = conceptKey.value.trim()
  if (!keyOK(key) || work.value?.concepts.some((item: any) => item.key === key)) { error.value = '方案键须为唯一的字母、数字或连字符。'; return }
  void change(work => { work.concepts.push({ key, file_id: conceptFile.value || null, status: 'candidate' }) })
}
function selectConcept(key: string) {
  void change(work => {
    for (const concept of work.concepts) if (concept.status === 'selected') concept.status = 'candidate'
    work.concepts.find((item: any) => item.key === key).status = 'selected'
    work.selected_concept_key = key
  })
}
function addShot() {
  const sequence = sequenceKey.value.trim(), key = shotKey.value.trim()
  if (!keyOK(sequence) || !keyOK(key) || !Number.isSafeInteger(plannedFrames.value) || plannedFrames.value < 1 || work.value?.shots.some((item: any) => item.key === key)) { error.value = '请填写唯一的序列和镜头键，以及正整数帧数。'; return }
  void change(work => {
    let target = work.sequences.find((item: any) => item.key === sequence)
    if (!target) { target = { key: sequence, label: sequenceLabel.value.trim() || sequence, shot_keys: [] }; work.sequences.push(target) }
    target.shot_keys.push(key)
    work.shots.push({ key, planned_frames: plannedFrames.value, storyboard_file_id: storyboardFile.value || null, take_keys: [] })
  })
}
function addTake() {
  const key = takeKey.value.trim(), shot = takeShot.value
  if (!keyOK(key) || !work.value?.shots.some((item: any) => item.key === shot) || work.value?.takes.some((item: any) => item.key === key)) { error.value = '请选定镜头并填写唯一的候选键。'; return }
  void change(work => {
    const run = runs.find(item => `${item.service_id}:${item.run_id}` === takeRun.value)
    work.takes.push({ key, shot_key: shot, file_id: takeFile.value || null, run_ref: run || null, status: 'candidate' })
    work.shots.find((item: any) => item.key === shot).take_keys.push(key)
  })
}
function selectTake(key: string) {
  const take = work.value?.takes.find((item: any) => item.key === key)
  if (!take?.file_id) { error.value = '候选尚无固定媒体文件。'; return }
  void change(work => {
    for (const item of work.takes) if (item.shot_key === take.shot_key && item.status === 'selected') item.status = 'rejected'
    work.takes.find((item: any) => item.key === key).status = 'selected'
  })
}
function fillTake(key: string) {
  const fileID = takeAssignments.value[key]
  if (!fileID || !videoFiles.value.some(file => file.file_id === fileID)) { error.value = '请选择当前项目修订中的视频文件。'; return }
  void change(work => {
    const take = work.takes.find((item: any) => item.key === key)
    if (!take || take.file_id || take.status !== 'candidate') throw new Error('此候选已固定媒体或不再允许填入。')
    take.file_id = fileID
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
  <section class="planning-editor">
    <div class="heading"><h2>{{ view === 'story' ? '创意与分镜' : '镜头候选' }}</h2><button v-if="pending" class="btn primary" :disabled="busy || hidden" @click="recover">恢复原提交</button></div>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="busy" class="notice" role="status">正在确认项目修订…</p>
    <p v-if="pending && !busy" class="notice">有一项保存结果待确认，请恢复原提交。</p>
    <p v-if="!ready && !error" role="status">正在读取创意视频内容…</p>
    <template v-if="ready && view === 'story'">
      <div class="editor-block"><h3>新增创意方案</h3><div class="fields"><label>方案键<input v-model="conceptKey" :disabled="pending || busy" placeholder="concept-a"></label><label>方案文件<select v-model="conceptFile" :disabled="pending || busy"><option value="">暂不关联</option><option v-for="file in files" :key="file.file_id" :value="file.file_id">{{ file.path }}</option></select></label></div><button class="btn" :disabled="pending || busy || !conceptKey.trim()" @click="addConcept">添加方案</button></div>
      <div v-if="work?.concepts.length" class="editor-block"><h3>方案选用</h3><div v-for="concept in work.concepts" :key="concept.key" class="line"><span>{{ concept.key }} · {{ concept.status }}</span><button v-if="concept.status !== 'selected'" class="btn" :disabled="pending || busy" @click="selectConcept(concept.key)">选用</button></div></div>
      <div class="editor-block"><h3>规划镜头</h3><div class="fields"><label>序列键<input v-model="sequenceKey" :disabled="pending || busy" list="creative-sequences" placeholder="opening"><datalist id="creative-sequences"><option v-for="sequence in work?.sequences" :key="sequence.key" :value="sequence.key" /></datalist></label><label>序列名称<input v-model="sequenceLabel" :disabled="pending || busy"></label><label>镜头键<input v-model="shotKey" :disabled="pending || busy" placeholder="shot-01"></label><label>计划帧数<input v-model.number="plannedFrames" type="number" min="1" step="1" :disabled="pending || busy"></label><label>分镜文件<select v-model="storyboardFile" :disabled="pending || busy"><option value="">暂不关联</option><option v-for="file in files" :key="file.file_id" :value="file.file_id">{{ file.path }}</option></select></label></div><button class="btn" :disabled="pending || busy || !sequenceKey.trim() || !shotKey.trim()" @click="addShot">添加镜头</button></div>
    </template>
    <template v-if="ready && view === 'shots'">
      <div class="editor-block"><h3>新增镜头候选</h3><div class="fields"><label>所属镜头<select v-model="takeShot" :disabled="pending || busy"><option value="">选择镜头</option><option v-for="shot in work?.shots" :key="shot.key" :value="shot.key">{{ shot.key }}</option></select></label><label>候选键<input v-model="takeKey" :disabled="pending || busy" placeholder="take-01"></label><label>媒体文件<select v-model="takeFile" :disabled="pending || busy"><option value="">待制作</option><option v-for="file in videoFiles" :key="file.file_id" :value="file.file_id">{{ file.path }}</option></select></label><label>原生任务<select v-model="takeRun" :disabled="pending || busy"><option value="">无</option><option v-for="run in runs" :key="`${run.service_id}:${run.run_id}`" :value="`${run.service_id}:${run.run_id}`">{{ run.service_id }} · {{ run.run_id }}</option></select></label></div><button class="btn" :disabled="pending || busy || !takeShot || !takeKey.trim()" @click="addTake">添加候选</button></div>
      <div v-if="work?.takes.length" class="editor-block"><h3>候选选用</h3><div v-for="take in work.takes" :key="take.key" class="line"><span>{{ take.shot_key }} / {{ take.key }} · {{ take.status }}</span><div v-if="take.status === 'candidate' && !take.file_id" class="commands"><select v-model="takeAssignments[take.key]" :aria-label="`${take.key} 媒体文件`" :disabled="pending || busy"><option value="">选择视频文件</option><option v-for="file in videoFiles" :key="file.file_id" :value="file.file_id">{{ file.path }}</option></select><button class="btn" :disabled="pending || busy || !takeAssignments[take.key]" @click="fillTake(take.key)">固定媒体</button></div><button v-else-if="take.status === 'candidate' && take.file_id" class="btn" :disabled="pending || busy" @click="selectTake(take.key)">选用</button></div></div>
    </template>
  </section>
</template>

<style scoped>
.planning-editor { display:grid; gap:18px; padding:16px 0 24px; border-top:1px solid var(--line); min-width:0; }
.heading,.line,.commands { display:flex; align-items:center; justify-content:space-between; flex-wrap:wrap; gap:10px; }
h2 { margin:0; font-size:18px; } h3 { margin:0 0 14px; font-size:15px; }
.editor-block { padding-top:16px; border-top:1px solid var(--line); }.editor-block:first-of-type { border-top:0; }
.fields { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:14px; margin-bottom:16px; }
label { display:grid; gap:6px; min-width:0; color:var(--muted); }
input,select { min-width:0; width:100%; min-height:38px; border:1px solid var(--line); border-radius:6px; padding:7px 10px; background:var(--panel); color:var(--ink); }
.line { padding:9px 0; border-bottom:1px solid var(--line); overflow-wrap:anywhere; }
.notice { padding:10px 12px; border-left:2px solid var(--line); background:var(--subtle); }.error { color:var(--danger); }
@media(max-width:700px) { .fields { grid-template-columns:minmax(0,1fr); } }
</style>
