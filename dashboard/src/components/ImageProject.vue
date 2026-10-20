<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { ProjectReader } from '../platform/project-reader'
import { readerError } from '../platform/project-reader'
import { imageDisplayVersion, type ImageVersion } from '../platform/image-content'
import ProjectMedia from './ProjectMedia.vue'
import ProjectDocument from './drama/DramaDocument.vue'
const props = defineProps<{ reader: ProjectReader; tab: string }>()
const route = useRoute()
const router = useRouter()
const query = (k: string) => typeof route.query[k] === 'string' ? route.query[k] as string : ''
const body = computed(() => props.reader.content?.image)
const work = computed(() => query('item') ? body.value?.works.find(w => w.key === query('item')) : filtered.value[0])
const version = computed(() => work.value && (query('version') ? work.value.versions.find(v => v.key === query('version')) : imageDisplayVersion(work.value)))
const compare = computed(() => work.value?.versions.find(v => v.key === query('compare')))
const filtered = computed(() => body.value?.works.filter(w => (w.title + ' ' + (w.overview || '')).toLowerCase().includes(query('find').toLowerCase())) || [])
const link = (changes: Record<string,string|undefined>) => ({ path: '/projects', query: { ...route.query, ...changes } })
const outputs = (v?: ImageVersion) => v?.file_refs.filter(f => f.purpose === 'output') || []
const format = (id: string) => {
 const extension = props.reader.file(id).path.split('.').pop()?.toUpperCase() || '图片'
 return extension === 'JPG' ? 'JPEG' : extension
}
const outputLabel = (id: string) => outputs(version.value).filter(f => format(f.file_id) === format(id)).length > 1 ? props.reader.file(id).path : format(id)
const selectedOutput = ref(''), showReferences = ref(true), showProcess = ref(true), showPrompts = ref(true)
const downloadBusy = ref(false), downloadError = ref('')
let downloadSerial = 0
const output = computed(() => outputs(version.value).find(f => f.file_id === selectedOutput.value) || outputs(version.value)[0])
watch(() => version.value, () => { selectedOutput.value = ''; showReferences.value = true; showProcess.value = true; showPrompts.value = true; downloadSerial++; downloadBusy.value = false; downloadError.value = '' })
function chooseVersion(event: Event) { void router.replace(link({ version:(event.target as HTMLSelectElement).value, compare:undefined })) }
function chooseWork(event: Event) { void router.replace(link({ item:(event.target as HTMLSelectElement).value, version:undefined, compare:undefined })) }
function chooseOutput(event: Event) { selectedOutput.value = (event.target as HTMLSelectElement).value; downloadSerial++; downloadBusy.value = false; downloadError.value = '' }
async function downloadOutput() {
 if (!output.value || downloadBusy.value) return
 const id = output.value.file_id, current = ++downloadSerial
 downloadBusy.value = true; downloadError.value = ''
 try {
  const result = await props.reader.download(id)
  if (current !== downloadSerial) return
  const anchor = document.createElement('a'); anchor.href = result.content_path; anchor.download = props.reader.file(id).path.split('/').pop() || 'image'; anchor.click()
 } catch (cause) { if (current === downloadSerial) downloadError.value = readerError(cause) }
 finally { if (current === downloadSerial) downloadBusy.value = false }
}
const texts = ref<{id:string;text:string}[]>([]), busy = ref(false), error = ref(''), copied = ref('')
let serial = 0
async function loadPrompts() {
 const current = ++serial; texts.value = []; error.value = ''; copied.value = ''; busy.value = true
 try {
  const items = await Promise.all((version.value?.prompt_file_ids || []).map(async id => {
   const doc = await props.reader.document(id)
   if (!['text/plain', 'text/markdown'].includes(doc.mime)) throw Error('执行提示词不是可展示的文本，请从文件页查看原件。')
   return { id, text: doc.text }
  }))
  if (serial === current) texts.value = items
 } catch (cause) { if (serial === current) error.value = readerError(cause) }
 finally { if (serial === current) busy.value = false }
}
watch(() => [props.reader, version.value], loadPrompts, { immediate:true })
onBeforeUnmount(() => { serial++; downloadSerial++ })
async function copy(text:string) { try { await navigator.clipboard.writeText(text); copied.value = '已复制' } catch { copied.value = '复制失败，请选择文字复制。' } }
</script>
<template>
 <div class="image-project">
  <template v-if="tab === 'image_materials'">
   <section class="panel"><h2>参考图</h2><div class="image-grid"><ProjectMedia v-for="id in body?.reference_file_ids" :key="id" :reader="reader" :file-id="id" kind="image" :label="reader.file(id).path" /></div><p v-if="!body?.reference_file_ids.length" class="muted">尚未添加参考图。</p></section>
   <section class="panel"><h2>制作说明</h2><ProjectDocument v-for="id in body?.process_file_ids" :key="id" :reader="reader" :file-id="id" :title="reader.file(id).path" /><p v-if="!body?.process_file_ids.length" class="muted">尚未添加制作说明。</p></section>
   <section v-for="w in body?.works" :key="w.key" class="panel"><h2>{{ w.title }} · 执行提示词</h2><div v-for="v in w.versions" :key="v.key"><h3>{{ v.label }}</h3><RouterLink class="btn" :to="link({ tab:'works', item:w.key, version:v.key, compare:undefined })">查看制作版本</RouterLink><ProjectDocument v-for="id in v.prompt_file_ids" :key="id" :reader="reader" :file-id="id" :title="reader.file(id).path" /></div></section>
  </template>
  <div v-else-if="query('item') && !work" class="state">当前项目修订中没有这件作品。</div>
  <template v-else-if="work">
   <label v-if="filtered.length > 1" class="field work-choice">作品<select :value="work.key" @change="chooseWork"><option v-for="w in filtered" :key="w.key" :value="w.key">{{ w.title }}</option></select></label>
   <nav v-if="!version" class="versions" aria-label="制作版本"><RouterLink v-for="v in work.versions" :key="v.key" class="btn" :to="link({ version:v.key, compare:undefined })">{{ v.label }}</RouterLink></nav>
   <section v-if="!version" class="state">{{ query('version') ? '当前修订中没有这个制作版本。' : '尚未指定展示版本，请选择一个制作版本。' }}</section>
   <template v-else>
    <section class="result-layout" aria-label="制作成果">
     <div class="result-preview"><ProjectMedia :reader="reader" :file-id="output?.file_id" kind="image" :label="'制作成果 · ' + version.label" /><p class="muted">点击图片查看大图</p></div>
     <div class="result-info"><p v-if="work.overview" class="result-lead">{{ work.overview }}</p><label class="field">版本<select :value="version.key" aria-label="制作版本" @change="chooseVersion"><option v-for="v in work.versions" :key="v.key" :value="v.key">{{ v.label }}</option></select></label><label v-if="outputs(version).length > 1" class="field">格式<select :value="output?.file_id" aria-label="输出文件" @change="chooseOutput"><option v-for="f in outputs(version)" :key="f.file_id" :value="f.file_id">{{ outputLabel(f.file_id) }}</option></select></label><button class="btn primary download-result" :disabled="downloadBusy || !output" @click="downloadOutput">{{ downloadBusy ? '正在准备…' : '下载图片' }}</button><p v-if="downloadError" role="alert">{{ downloadError }}</p></div>
    </section>
    <section class="detail-section"><button class="disclosure" :aria-expanded="showReferences" @click="showReferences = !showReferences"><span>参考照片</span><span aria-hidden="true">{{ showReferences ? '−' : '＋' }}</span></button><div v-if="showReferences" class="image-grid"><div v-for="id in version.reference_file_ids" :key="id"><h3>{{ reader.file(id).path }}</h3><ProjectMedia :reader="reader" :file-id="id" kind="image" :label="'参考图 · ' + reader.file(id).path" /></div><p v-if="!version.reference_file_ids.length" class="muted">未登记本版参考图</p></div></section>
    <section class="detail-section"><button class="disclosure" :aria-expanded="showPrompts" @click="showPrompts = !showPrompts"><span>提示词</span><span aria-hidden="true">{{ showPrompts ? '−' : '＋' }}</span></button><div v-if="showPrompts" class="section-body"><p class="muted">本版执行提示词 · 仅用于当前项目</p><p v-if="busy" role="status">正在读取提示词…</p><p v-else-if="error" role="alert">{{ error }} <button class="btn" @click="loadPrompts">重试</button></p><p v-else-if="!texts.length" class="muted">尚未登记执行提示词。</p><article v-for="p in texts" :key="p.id"><h3>{{ reader.file(p.id).path }}</h3><p class="prose">{{ p.text }}</p><button class="btn" @click="copy(p.text)">复制提示词</button><ProjectMedia :reader="reader" :file-id="p.id" kind="file" label="执行提示词原件" /></article><p role="status">{{ copied }}</p></div></section>
    <section class="detail-section"><button class="disclosure" :aria-expanded="showProcess || !!query('compare')" @click="showProcess = !showProcess; query('compare') && router.replace(link({compare:undefined}))"><span>制作记录</span><span aria-hidden="true">{{ showProcess || query('compare') ? '−' : '＋' }}</span></button><div v-if="showProcess || query('compare')" class="process-items"><p class="prose">{{ version.change_note }}</p><p v-if="version.parent_version_key" class="muted">来源版本：{{ work.versions.find(v => v.key === version?.parent_version_key)?.label }}</p><p class="muted">展示版本用于浏览，不代表审核通过或正式选用。</p><div v-if="work.versions.length > 1"><h3>版本对照</h3><nav class="versions"><RouterLink v-for="v in work.versions.filter(v => v.key !== version?.key)" :key="v.key" :to="link({ compare:v.key })">与 {{ v.label }} 对照</RouterLink><RouterLink v-if="query('compare')" :to="link({compare:undefined})">收起对照</RouterLink></nav></div><p v-if="query('compare') && !compare" role="alert">对照版本不存在。</p><div v-if="compare" class="image-grid"><div><h3>{{ version.label }}</h3><ProjectMedia :reader="reader" :file-id="output?.file_id" kind="image" :label="version.label" /></div><div><h3>{{ compare.label }}</h3><ProjectMedia :reader="reader" :file-id="outputs(compare)[0]?.file_id" kind="image" :label="compare.label" /></div></div><template v-for="f in version.file_refs.filter(f => f.purpose !== 'output')" :key="f.file_id"><ProjectDocument v-if="f.purpose === 'record'" :reader="reader" :file-id="f.file_id" :title="reader.file(f.file_id).path" /><ProjectMedia v-else :reader="reader" :file-id="f.file_id" kind="image" :label="reader.file(f.file_id).path" /></template></div></section>
   </template>
  </template>
  <div v-else class="state">{{ query('find') ? '没有匹配的作品。' : '暂无图像作品。已有素材可在文件页查看。' }}</div>
 </div>
</template>
<style scoped>
.image-project { display:grid;gap:20px;min-width:0 }.image-grid { display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:24px }.image-grid>div{min-width:0}.versions{display:flex;gap:8px;flex-wrap:wrap}.versions [aria-current=page]{border-color:var(--accent);color:var(--accent)}.prose{white-space:pre-wrap;overflow-wrap:anywhere;line-height:1.8}.muted{color:var(--muted)}article+article{margin-top:24px}@media(max-width:760px){.image-grid{grid-template-columns:1fr}}
.image-project{max-width:1100px}.result-layout{display:grid;grid-template-columns:minmax(0,460px) minmax(0,1fr);gap:36px;align-items:start}.result-preview{text-align:center}.result-preview>p{font-size:12px;margin-top:10px}.result-info{min-width:0;max-width:400px;padding:24px 0}.result-lead{font-size:16px;line-height:1.8;margin-bottom:28px}.field{display:grid;gap:8px;color:var(--muted);font-size:13px;margin:18px 0}.field select{width:100%;padding:10px 12px;border:1px solid var(--line);border-radius:8px;background:var(--panel);color:var(--ink)}.work-choice{max-width:460px;margin-top:0}.download-result{margin-top:16px}.detail-section{border-top:1px solid var(--line);padding-top:4px}.detail-section .disclosure{display:flex;justify-content:space-between;width:100%;padding:16px 0;font-weight:500;font-size:15px}.section-body{padding:0 0 20px}.section-body h3{margin:16px 0 10px}
.image-project :deep(.image-open){display:flex;justify-content:center;align-items:center;padding:20px}.image-project :deep(.image-open img){width:auto;max-width:100%;max-height:min(560px,calc(100vh - 220px));object-fit:contain}.image-project :deep(.image){max-width:460px}.image-grid{grid-template-columns:repeat(2,minmax(0,392px));margin-top:16px}.disclosure{text-align:left;cursor:pointer;color:var(--ink)}.disclosure:focus-visible{outline:2px solid var(--accent);outline-offset:4px}.process-items{display:grid;gap:16px;margin:16px 0 24px}
@media(max-width:760px){.result-layout{grid-template-columns:1fr;gap:16px}.result-preview{max-width:460px;width:100%}.result-info{padding:0}.image-grid{grid-template-columns:1fr}.image-project :deep(.image-open img){max-height:480px}}
</style>
