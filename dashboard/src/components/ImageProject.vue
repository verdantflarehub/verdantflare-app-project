<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import type { ProjectReader } from '../platform/project-reader'
import { readerError } from '../platform/project-reader'
import { imageDisplayVersion, type ImageVersion } from '../platform/image-content'
import ProjectMedia from './ProjectMedia.vue'
import ProjectDocument from './drama/DramaDocument.vue'
const props = defineProps<{ reader: ProjectReader; tab: string }>()
const route = useRoute()
const query = (k: string) => typeof route.query[k] === 'string' ? route.query[k] as string : ''
const body = computed(() => props.reader.content?.image)
const work = computed(() => body.value?.works.find(w => w.key === query('item')))
const version = computed(() => work.value && (query('version') ? work.value.versions.find(v => v.key === query('version')) : imageDisplayVersion(work.value)))
const compare = computed(() => work.value?.versions.find(v => v.key === query('compare')))
const filtered = computed(() => body.value?.works.filter(w => (w.title + ' ' + (w.overview || '')).toLowerCase().includes(query('find').toLowerCase())) || [])
const link = (changes: Record<string,string|undefined>) => ({ path: '/projects', query: { ...route.query, ...changes } })
const outputs = (v?: ImageVersion) => v?.file_refs.filter(f => f.purpose === 'output') || []
const selectedOutput = ref(''), showReferences = ref(false), showProcess = ref(false)
const output = computed(() => outputs(version.value).find(f => f.file_id === selectedOutput.value) || outputs(version.value)[0])
watch(() => version.value, () => { selectedOutput.value = ''; showReferences.value = false; showProcess.value = false })
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
onBeforeUnmount(() => { serial++ })
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
   <nav v-if="!version" class="versions" aria-label="制作版本"><RouterLink v-for="v in work.versions" :key="v.key" class="btn" :to="link({ version:v.key, compare:undefined })">{{ v.label }}</RouterLink></nav>
   <section v-if="!version" class="state">{{ query('version') ? '当前修订中没有这个制作版本。' : '尚未指定展示版本，请选择一个制作版本。' }}</section>
   <template v-else>
    <section class="result-layout" aria-label="制作成果">
     <div class="result-preview"><ProjectMedia :reader="reader" :file-id="output?.file_id" kind="image" :label="'制作成果 · ' + version.label" /><p class="muted">完整缩略展示 · 点击查看大图</p></div>
     <div class="result-info panel"><h2>制作版本</h2><nav class="versions" aria-label="制作版本"><RouterLink v-for="v in work.versions" :key="v.key" class="btn" :aria-current="version.key === v.key ? 'page' : undefined" :to="link({ version:v.key, compare:undefined })">{{ v.label }}<span v-if="work.display_version_key === v.key"> · 展示版</span></RouterLink></nav><p class="prose">{{ version.change_note }}</p><p v-if="version.parent_version_key" class="muted">来源版本：{{ work.versions.find(v => v.key === version?.parent_version_key)?.label }}</p><h3>输出文件</h3><div class="versions" aria-label="输出文件"><button v-for="f in outputs(version)" :key="f.file_id" class="btn output-choice" :aria-pressed="output?.file_id === f.file_id" @click="selectedOutput = f.file_id">{{ reader.file(f.file_id).path }}</button></div><ProjectMedia v-if="output" :reader="reader" :file-id="output.file_id" kind="file" :label="reader.file(output.file_id).path" /><p class="muted">展示版用于浏览，不代表审核通过或正式选用。</p></div>
    </section>
    <section class="panel reference-panel"><button class="disclosure" :aria-expanded="showReferences" @click="showReferences = !showReferences">{{ showReferences ? '收起' : '展开' }}参考与对照 · {{ version.reference_file_ids.length }} 张</button><div v-if="showReferences" class="image-grid"><div v-for="id in version.reference_file_ids" :key="id"><h3>{{ reader.file(id).path }}</h3><ProjectMedia :reader="reader" :file-id="id" kind="image" :label="'参考图 · ' + reader.file(id).path" /></div><p v-if="!version.reference_file_ids.length" class="muted">未登记本版参考图</p></div></section>
    <section v-if="work.versions.length > 1" class="panel"><h2>版本对照</h2><nav class="versions"><RouterLink v-for="v in work.versions.filter(v => v.key !== version?.key)" :key="v.key" class="btn" :to="link({ compare: v.key })">与 {{ v.label }} 对照</RouterLink><RouterLink v-if="query('compare')" class="btn" :to="link({compare:undefined})">收起对照</RouterLink></nav><p v-if="query('compare') && !compare" role="alert">对照版本不存在。</p><div v-if="compare" class="image-grid"><div><h3>{{ version.label }}</h3><ProjectMedia :reader="reader" :file-id="output?.file_id" kind="image" :label="version.label" /></div><div><h3>{{ compare.label }}</h3><ProjectMedia :reader="reader" :file-id="outputs(compare)[0]?.file_id" kind="image" :label="compare.label" /></div></div></section>
    <section class="panel"><h2>本版执行提示词</h2><p class="muted">针对本次输入的制作记录，仅保存在项目中。</p><p v-if="busy" role="status">正在读取提示词…</p><p v-else-if="error" role="alert">{{ error }} <button class="btn" @click="loadPrompts">重试</button></p><p v-else-if="!texts.length" class="muted">尚未登记执行提示词。</p><article v-for="p in texts" :key="p.id"><h3>{{ reader.file(p.id).path }}</h3><p class="prose">{{ p.text }}</p><button class="btn" @click="copy(p.text)">复制提示词</button><ProjectMedia :reader="reader" :file-id="p.id" kind="file" label="执行提示词原件" /></article><p role="status">{{ copied }}</p></section>
    <section class="panel"><button class="disclosure" :aria-expanded="showProcess" @click="showProcess = !showProcess">{{ showProcess ? '收起' : '展开' }}过程与制作记录</button><div v-if="showProcess" class="process-items"><template v-for="f in version.file_refs.filter(f => f.purpose !== 'output')" :key="f.file_id"><ProjectDocument v-if="f.purpose === 'record'" :reader="reader" :file-id="f.file_id" :title="reader.file(f.file_id).path" /><ProjectMedia v-else :reader="reader" :file-id="f.file_id" kind="image" :label="reader.file(f.file_id).path" /></template></div></section>
   </template>
  </template>
  <template v-else><p class="prose">{{ reader.content?.overview }}</p><div v-if="!filtered.length" class="state">暂无图像作品。已有素材可在文件页查看。</div><div class="cards project-grid"><RouterLink v-for="w in filtered" :key="w.key" class="project-card" :to="link({ item:w.key, version:undefined, compare:undefined })"><ProjectMedia :reader="reader" :file-id="outputs(imageDisplayVersion(w))[0]?.file_id" kind="image" :label="w.title" thumbnail contain /><div class="card-text"><h2>{{ w.title }}</h2><p>{{ w.overview }}</p><small>{{ w.versions.length }} 个制作版本 · {{ imageDisplayVersion(w)?.label || '未指定展示版' }}</small></div></RouterLink></div></template>
 </div>
</template>
<style scoped>
.image-project { display:grid;gap:20px;min-width:0 }.image-grid { display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:24px }.image-grid>div{min-width:0}.versions{display:flex;gap:8px;flex-wrap:wrap}.versions [aria-current=page]{border-color:var(--accent);color:var(--accent)}.prose{white-space:pre-wrap;overflow-wrap:anywhere;line-height:1.8}.muted{color:var(--muted)}article+article{margin-top:24px}@media(max-width:760px){.image-grid{grid-template-columns:1fr}}
.image-project{max-width:960px}.result-layout{display:grid;grid-template-columns:minmax(0,392px) minmax(0,1fr);gap:28px;align-items:start}.result-preview{text-align:center}.result-preview>p{font-size:12px;margin-top:10px}.result-info{min-width:0}.result-info h2,.result-info h3{margin-bottom:12px}.result-info p{margin:16px 0}.result-info h3{margin-top:24px}.output-choice{overflow-wrap:anywhere;white-space:normal;text-align:left}.output-choice[aria-pressed=true]{color:var(--accent);border-color:var(--accent)}
.image-project :deep(.image-open){display:flex;justify-content:center;align-items:center;padding:16px}.image-project :deep(.image-open img){width:auto;max-width:100%;max-height:min(480px,calc(100vh - 260px));object-fit:contain}.image-project :deep(.image){max-width:392px}.image-grid{grid-template-columns:repeat(2,minmax(0,392px));margin-top:16px}.disclosure{text-align:left;cursor:pointer;font-weight:600;color:var(--ink);padding:4px 0}.disclosure:focus-visible{outline:2px solid var(--accent);outline-offset:4px}.process-items{display:grid;gap:16px;margin-top:16px}.image-project .project-grid{grid-template-columns:repeat(auto-fill,minmax(min(100%,280px),1fr))}.image-project .project-card{max-width:360px}
@media(max-width:760px){.result-layout{grid-template-columns:1fr}.result-preview{max-width:392px;width:100%}.image-grid{grid-template-columns:1fr}.image-project :deep(.image-open img){max-height:400px}}
</style>
