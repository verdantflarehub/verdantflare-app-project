<script setup lang="ts">
import { computed, onBeforeUnmount, shallowRef, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ProjectReader, readerError } from '../platform/project-reader'
import { projectSourceRoute, type ProjectSource } from '../platform/project-source'
import { useHost } from '../platform/host'
import ProjectMedia from './ProjectMedia.vue'

const props = defineProps<{ reader: ProjectReader; source: ProjectSource }>()
const route = useRoute(), { appSDK } = useHost()
const sourceReader = shallowRef<ProjectReader>(), busy = ref(false), error = ref('')
const target = computed(() => projectSourceRoute(props.reader.project!, props.source, route.query))
let generation = 0, alive = true, hidden = false
function clear() { generation++; sourceReader.value?.dispose(); sourceReader.value = undefined; busy.value = false }
async function load() {
  clear(); error.value = ''; if (hidden || !alive) return
  const token = generation, source = props.source, next = new ProjectReader(source.project_id, source.revision_id)
  busy.value = true; sourceReader.value = next
  try {
    await next.load()
    if (next.project?.manifest.category !== source.category) throw Error('来源项目类型与固定引用不一致。')
    if (source.category === 'music' && !next.content?.music?.songs.some(s => s.key === source.sourceItem)) throw Error('来源固定修订没有这首歌曲。')
  } catch (cause) {
    next.dispose()
    if (alive && token === generation) { sourceReader.value = undefined; error.value = readerError(cause) }
  } finally { if (alive && token === generation) busy.value = false }
}
watch(() => [props.reader, props.source.key, props.source.project_id, props.source.revision_id], () => void load(), { immediate: true })
const stop = appSDK.lifecycle.subscribe(event => {
  if (event === 'hidden') { hidden = true; clear() }
  if (event === 'active') { hidden = false; void load() }
  if (event === 'dispose') { alive = false; clear() }
})
onBeforeUnmount(() => { alive = false; clear(); stop() })
</script>

<template>
  <section class="panel"><h2>{{ source.name }}</h2><dl><dt>来源类型</dt><dd>{{ source.category === 'music' ? '音乐' : '角色' }}项目固定修订</dd><dt>用途</dt><dd>{{ source.purpose }}</dd><dt>来源项目</dt><dd>{{ source.project_id }}</dd><dt>固定修订</dt><dd>{{ source.revision_id }}</dd><template v-if="source.sourceItem"><dt>固定歌曲</dt><dd>{{ source.sourceItem }}</dd></template></dl>
    <ProjectMedia v-if="source.preview_file_id" :reader="reader" :file-id="source.preview_file_id" kind="image" :label="source.name" />
    <p v-if="busy" class="muted" role="status">正在核对来源固定修订…</p>
    <div v-else-if="error" role="alert"><p>{{ error }}</p><button class="btn" @click="load">重试来源读取</button></div>
    <template v-else-if="sourceReader?.project"><p>当前权限：已读取此固定修订。</p><p>{{ sourceReader.project.manifest.name }}</p><p class="muted">{{ sourceReader.content?.overview || '来源未填写概述。' }}</p><RouterLink class="btn" :to="target">打开来源详情</RouterLink></template>
    <h3>许可与适配</h3><p class="muted">项目读取权限不代表素材已获使用许可。此关系未登记使用范围与期限，请核对来源说明。</p>
  </section>
  <section class="panel"><h2>本项目使用位置</h2><RouterLink class="btn" :to="{ path: route.path, query: { ...route.query, reference: undefined, tab: source.category === 'music' ? 'music' : 'cast', item: source.category === 'music' ? undefined : source.item, material: source.category === 'music' ? source.item : undefined } }">{{ source.purpose }} · {{ source.name }}</RouterLink></section>
</template>

<style scoped>
.panel { padding:20px; border:1px solid var(--line); border-radius:12px; background:var(--shell-panel); min-width:0; }h2 { font-size:18px; margin-bottom:12px; }h3 { margin:16px 0 8px; }.muted,dt { color:var(--muted); }dl { display:grid; grid-template-columns:80px minmax(0,1fr); gap:12px; margin:16px 0; }[role=alert] { color:var(--danger); }
</style>
