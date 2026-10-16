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
const files = computed(() => project.manifest.files.filter(file => !project.manifest.domain_documents.some(doc => doc.file_id === file.file_id)))
const videoFiles = computed(() => files.value.filter(file => /\.(mp4|mov|webm|mkv)$/i.test(file.path)))
const keyOK = (value: string) => /^[A-Za-z0-9_-]{1,96}$/.test(value)
const variantKey = ref(''), editKey = ref(''), channel = ref(''), territory = ref('CN'), width = ref(1920), height = ref(1080), language = ref('zh-CN')
const packageKey = ref(''), packageVariant = ref(''), mediaFile = ref('')
const reviewTarget = ref(''), reviewKind = ref<'creative' | 'rights' | 'technical' | 'client'>('creative'), reviewKey = ref('')
const decisions = ref<Record<string, string>>({}), records = ref<Record<string, string>>({})
const deliveryRecords = ref<Record<string, string>>({})

async function load() {
  try {
    const doc = project.manifest.domain_documents.find(item => item.document_type === 'project-content')
    if (!doc) throw new Error('请先建立创意视频业务文档。')
    const read = await props.reader.document(doc.file_id)
    if (!alive || hidden.value) return
    const value = JSON.parse(read.text)
    if (value.category !== 'creative_video' || !value.creative_video) throw new Error('创意视频业务文档格式不正确。')
    original.value = value; ready.value = true
  } catch (reason) { if (alive && !hidden.value) error.value = reason instanceof Error ? reason.message : '交付内容读取失败。' }
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
const frames = (edit: any) => edit.items.reduce((sum: number, item: any) => sum + item.source_out_frame - item.source_in_frame, 0)
function addVariant() {
  const key = variantKey.value.trim(), edit = work.value?.edits.find((item: any) => item.key === editKey.value)
  if (!keyOK(key) || work.value?.delivery_variants.some((item: any) => item.key === key) || !edit || edit.state !== 'picture_locked' || ![width.value, height.value].every(value => Number.isSafeInteger(value) && value > 0 && value <= 16384) || !language.value.trim() || !keyOK(channel.value) || !keyOK(territory.value)) { error.value = '请选择锁片版本，填写唯一交付键、有效尺寸、渠道、地域和语言。'; return }
  void change(work => work.delivery_variants.push({ key, edit_key: edit.key, channel: channel.value, territory: territory.value, width: width.value, height: height.value, fps: structuredClone(edit.fps), target_frames: frames(edit), language: language.value.trim(), subtitle_mode: 'none', audio_mode: 'full' }))
}
function addPackage() {
  const key = packageKey.value.trim(), variant = work.value?.delivery_variants.find((item: any) => item.key === packageVariant.value)
  if (!keyOK(key) || work.value?.packages.some((item: any) => item.key === key) || !variant?.edit_key || !videoFiles.value.some(file => file.file_id === mediaFile.value)) { error.value = '请选择交付版和当前项目中的固定视频文件，并填写唯一导出键。'; return }
  void change(work => work.packages.push({ key, variant_key: variant.key, edit_key: variant.edit_key, media_file_id: mediaFile.value, attachment_file_ids: [], review_keys: [], state: 'candidate' }))
}
function addReview() {
  const key = reviewKey.value.trim()
  if (!keyOK(key) || work.value?.reviews.some((item: any) => item.key === key) || !work.value?.packages.some((item: any) => item.key === reviewTarget.value && item.state === 'candidate')) { error.value = '请选择待审核导出版本并填写唯一审核键。'; return }
  void change(work => {
    work.reviews.push({ key, target_kind: 'package', target_key: reviewTarget.value, basis_revision_id: project.revision_id, kind: reviewKind.value, state: 'pending', decision_by: null, record_file_id: null })
    work.packages.find((item: any) => item.key === reviewTarget.value).review_keys.push(key)
  })
}
function decide(review: any) {
  const state = decisions.value[review.key], file = records.value[review.key]
  if (!['approved', 'changes_requested', 'rejected'].includes(state) || !files.value.some(item => item.file_id === file) || !appSDK.context.get().userId) { error.value = '请选择审核结论和当前修订中的审核记录文件。'; return }
  void change(work => Object.assign(work.reviews.find((item: any) => item.key === review.key), { state, decision_by: appSDK.context.get().userId, record_file_id: file }))
}
function advance(pack: any, state: 'ready' | 'delivered') {
  const latest = new Map<string, string>()
  for (const key of pack.review_keys) {
    const review = work.value?.reviews.find((item: any) => item.key === key)
    if (review) latest.set(review.kind, review.state)
  }
  if (['creative', 'rights', 'technical', 'client'].some(kind => latest.get(kind) !== 'approved')) { error.value = '创意、权利、技术和客户的最新审核须分别通过。'; return }
  if (state === 'delivered' && !files.value.some(file => file.file_id === deliveryRecords.value[pack.key])) { error.value = '请选择交付回执文件。'; return }
  void change(work => {
    const target = work.packages.find((item: any) => item.key === pack.key)
    if (state === 'delivered') target.attachment_file_ids.push(deliveryRecords.value[pack.key])
    target.state = state
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
  <section class="delivery-editor">
    <div class="head"><h2>交付与审核</h2><button v-if="pending" class="btn primary" :disabled="busy || hidden" @click="recover">恢复原提交</button></div>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p><p v-if="busy" class="notice" role="status">正在确认项目修订…</p><p v-if="pending && !busy" class="notice">有一项保存结果待确认，请恢复原提交。</p><p v-if="!ready && !error" role="status">正在读取交付内容…</p>
    <template v-if="ready">
      <div class="block"><h3>规划交付版</h3><div class="fields"><label>交付键<input v-model="variantKey" :disabled="pending || busy" placeholder="web-16x9"></label><label>锁片版本<select v-model="editKey" :disabled="pending || busy"><option value="">选择版本</option><option v-for="edit in work?.edits.filter((item: any) => item.state === 'picture_locked')" :key="edit.key" :value="edit.key">{{ edit.key }}</option></select></label><label>渠道代码<input v-model="channel" :disabled="pending || busy" placeholder="web"></label><label>地域代码<input v-model="territory" :disabled="pending || busy" placeholder="CN"></label><label>宽度<input v-model.number="width" type="number" min="1" max="16384" :disabled="pending || busy"></label><label>高度<input v-model.number="height" type="number" min="1" max="16384" :disabled="pending || busy"></label><label>语言<input v-model="language" :disabled="pending || busy"></label></div><button class="btn" :disabled="pending || busy || !variantKey || !editKey || !channel || !territory" @click="addVariant">保存交付版</button></div>
      <div class="block"><h3>登记固定导出</h3><div class="fields"><label>导出键<input v-model="packageKey" :disabled="pending || busy" placeholder="export-v1"></label><label>交付版<select v-model="packageVariant" :disabled="pending || busy"><option value="">选择交付版</option><option v-for="variant in work?.delivery_variants" :key="variant.key" :value="variant.key">{{ variant.key }}</option></select></label><label>视频文件<select v-model="mediaFile" :disabled="pending || busy"><option value="">选择当前文件</option><option v-for="file in videoFiles" :key="file.file_id" :value="file.file_id">{{ file.path }}</option></select></label></div><button class="btn" :disabled="pending || busy || !packageKey || !packageVariant || !mediaFile" @click="addPackage">登记导出</button></div>
      <div v-if="work?.packages.length" class="block"><h3>逐版审核</h3><div class="fields"><label>导出版本<select v-model="reviewTarget" :disabled="pending || busy"><option value="">选择待审核版本</option><option v-for="pack in work.packages.filter((item: any) => item.state === 'candidate')" :key="pack.key" :value="pack.key">{{ pack.key }}</option></select></label><label>审核类型<select v-model="reviewKind" :disabled="pending || busy"><option value="creative">创意</option><option value="rights">权利</option><option value="technical">技术</option><option value="client">客户</option></select></label><label>审核键<input v-model="reviewKey" :disabled="pending || busy" placeholder="review-creative-v1"></label></div><button class="btn" :disabled="pending || busy || !reviewTarget || !reviewKey" @click="addReview">发起审核</button>
        <div v-for="review in work.reviews.filter((item: any) => item.target_kind === 'package')" :key="review.key" class="review-row"><span>{{ review.target_key }} / {{ review.kind }} · {{ review.state }}</span><template v-if="(review.state === 'pending' || review.state === 'in_review') && work.packages.some((item: any) => item.key === review.target_key && item.state === 'candidate')"><select v-model="decisions[review.key]" :aria-label="`${review.key} 审核结论`" :disabled="pending || busy"><option value="">选择结论</option><option value="approved">通过</option><option value="changes_requested">需修改</option><option value="rejected">未通过</option></select><select v-model="records[review.key]" :aria-label="`${review.key} 审核记录`" :disabled="pending || busy"><option value="">选择记录文件</option><option v-for="file in files" :key="file.file_id" :value="file.file_id">{{ file.path }}</option></select><button class="btn" :disabled="pending || busy || !decisions[review.key] || !records[review.key]" @click="decide(review)">提交结论</button></template></div>
      </div>
      <div v-if="work?.packages.length" class="block"><h3>交付状态</h3><div v-for="pack in work.packages" :key="pack.key" class="package-row"><span>{{ pack.key }} · {{ pack.state }}</span><button v-if="pack.state === 'candidate'" class="btn" :disabled="pending || busy" @click="advance(pack, 'ready')">标记待交付</button><template v-if="pack.state === 'ready'"><select v-model="deliveryRecords[pack.key]" :aria-label="`${pack.key} 交付回执`" :disabled="pending || busy"><option value="">选择交付回执</option><option v-for="file in files" :key="file.file_id" :value="file.file_id">{{ file.path }}</option></select><button class="btn" :disabled="pending || busy || !deliveryRecords[pack.key]" @click="advance(pack, 'delivered')">确认交付</button></template></div></div>
    </template>
  </section>
</template>

<style scoped>
.delivery-editor { display:grid; gap:18px; min-width:0; padding:16px 0 24px; border-top:1px solid var(--line); }.head,.review-row,.package-row { display:flex; align-items:center; justify-content:space-between; flex-wrap:wrap; gap:10px; }
h2 { margin:0; font-size:18px; } h3 { margin:0 0 14px; font-size:15px; }.block { padding-top:16px; border-top:1px solid var(--line); }.fields { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:14px; margin-bottom:16px; }
label { display:grid; gap:6px; min-width:0; color:var(--muted); }input,select { min-width:0; min-height:38px; border:1px solid var(--line); border-radius:6px; padding:7px 10px; background:var(--panel); color:var(--ink); }label input,label select { width:100%; }.review-row,.package-row { padding:10px 0; border-bottom:1px solid var(--line); overflow-wrap:anywhere; }.review-row select,.package-row select { max-width:250px; }.notice { padding:10px 12px; border-left:2px solid var(--line); background:var(--subtle); }.error { color:var(--danger); }
@media(max-width:700px) { .fields { grid-template-columns:minmax(0,1fr); }.review-row select,.package-row select { width:100%; max-width:none; } }
</style>
