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
   <section class="panel"><h2>{{ work.title }}</h2><p>{{ work.overview || '未填写概述' }}</p><nav class="versions" aria-label="制作版本"><RouterLink v-for="v in work.versions" :key="v.key" class="btn" :aria-current="version?.key === v.key ? 'page' : undefined" :to="link({ version:v.key, compare:undefined })">{{ v.label }}<span v-if="work.display_version_key === v.key"> · 展示版</span></RouterLink></nav><p class="muted">展示版用于浏览，不代表审核通过或正式选用。</p></section>
   <section v-if="!version" class="state">{{ query('version') ? '当前修订中没有这个制作版本。' : '尚未指定展示版本，请选择一个制作版本。' }}</section>
   <template v-else>
    <section class="panel"><h2>{{ version.label }}</h2><p class="prose">{{ version.change_note }}</p><p v-if="version.parent_version_key" class="muted">来源版本：{{ work.versions.find(v => v.key === version?.parent_version_key)?.label }}</p><div class="image-grid"><div><h3>本版参考图</h3><ProjectMedia v-for="id in version.reference_file_ids" :key="id" :reader="reader" :file-id="id" kind="image" :label="'参考图 · ' + reader.file(id).path" /><p v-if="!version.reference_file_ids.length" class="muted">未登记本版参考图</p></div><div><h3>本版结果</h3><ProjectMedia v-for="f in outputs(version)" :key="f.file_id" :reader="reader" :file-id="f.file_id" kind="image" :label="version.label" /></div></div></section>
    <section v-if="work.versions.length > 1" class="panel"><h2>版本对照</h2><nav class="versions"><RouterLink v-for="v in work.versions.filter(v => v.key !== version?.key)" :key="v.key" class="btn" :to="link({ compare: v.key })">与 {{ v.label }} 对照</RouterLink><RouterLink v-if="query('compare')" class="btn" :to="link({compare:undefined})">收起对照</RouterLink></nav><p v-if="query('compare') && !compare" role="alert">对照版本不存在。</p><div v-if="compare" class="image-grid"><div><h3>{{ version.label }}</h3><ProjectMedia v-for="f in outputs(version)" :key="f.file_id" :reader="reader" :file-id="f.file_id" kind="image" :label="version.label" /></div><div><h3>{{ compare.label }}</h3><ProjectMedia v-for="f in outputs(compare)" :key="f.file_id" :reader="reader" :file-id="f.file_id" kind="image" :label="compare.label" /></div></div></section>
    <section class="panel"><h2>本版执行提示词</h2><p class="muted">针对本次输入的制作记录，仅保存在项目中。</p><p v-if="busy" role="status">正在读取提示词…</p><p v-else-if="error" role="alert">{{ error }} <button class="btn" @click="loadPrompts">重试</button></p><p v-else-if="!texts.length" class="muted">尚未登记执行提示词。</p><article v-for="p in texts" :key="p.id"><h3>{{ reader.file(p.id).path }}</h3><p class="prose">{{ p.text }}</p><button class="btn" @click="copy(p.text)">复制提示词</button><ProjectMedia :reader="reader" :file-id="p.id" kind="file" label="执行提示词原件" /></article><p role="status">{{ copied }}</p></section>
    <section class="panel"><h2>过程与制作记录</h2><template v-for="f in version.file_refs.filter(f => f.purpose !== 'output')" :key="f.file_id"><ProjectDocument v-if="f.purpose === 'record'" :reader="reader" :file-id="f.file_id" :title="reader.file(f.file_id).path" /><ProjectMedia v-else :reader="reader" :file-id="f.file_id" kind="image" :label="reader.file(f.file_id).path" /></template></section>
   </template>
  </template>
  <template v-else><p class="prose">{{ reader.content?.overview }}</p><div v-if="!filtered.length" class="state">暂无图像作品。已有素材可在文件页查看。</div><div class="cards project-grid"><RouterLink v-for="w in filtered" :key="w.key" class="project-card" :to="link({ item:w.key, version:undefined, compare:undefined })"><ProjectMedia :reader="reader" :file-id="outputs(imageDisplayVersion(w))[0]?.file_id" kind="image" :label="w.title" thumbnail /><div class="card-text"><h2>{{ w.title }}</h2><p>{{ w.overview }}</p><small>{{ w.versions.length }} 个制作版本 · {{ imageDisplayVersion(w)?.label || '未指定展示版' }}</small></div></RouterLink></div></template>
 </div>
</template>
<style scoped>
.image-project { display:grid;gap:20px;min-width:0 }.image-grid { display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:24px }.image-grid>div{min-width:0}.versions{display:flex;gap:8px;flex-wrap:wrap}.versions [aria-current=page]{border-color:var(--accent);color:var(--accent)}.prose{white-space:pre-wrap;overflow-wrap:anywhere;line-height:1.8}.muted{color:var(--muted)}article+article{margin-top:24px}@media(max-width:760px){.image-grid{grid-template-columns:1fr}}
</style>
