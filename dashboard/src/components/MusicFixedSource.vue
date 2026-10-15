<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ProjectReader, readerError } from '../platform/project-reader'
import { musicSourceEntry, musicVersionRoute, musicVersionKinds, musicReview, type MusicSource } from '../platform/music-production'
import { useHost } from '../platform/host'
const props = defineProps<{ source: MusicSource; ownerRevision: string }>()
const route = useRoute(), { appSDK } = useHost(), reader = shallowRef<ProjectReader>(), busy = ref(false), error = ref('')
let epoch = 0, alive = true, hidden = false
const entry = computed(() => reader.value?.content?.music && musicSourceEntry(reader.value.content.music, props.source))
const version = computed(() => entry.value?.production?.versions.find(v => v.key === props.source.version_key))
const target = computed(() => {
  const value = musicVersionRoute(route.query, props.source), owner = new URL(route.fullPath, 'https://studio.invalid')
  owner.searchParams.set('revision', props.ownerRevision); owner.searchParams.delete('fromMusic')
  return { ...value, query: { ...value.query, fromMusic: owner.pathname + owner.search } }
})
function clear() { epoch++; reader.value?.dispose(); reader.value = undefined; busy.value = false }
async function load() {
  clear(); error.value = ''; if (!alive || hidden) return
  const token = epoch, next = new ProjectReader(props.source.project_id, props.source.revision_id); busy.value = true
  try {
    await next.load()
    if (next.project?.manifest.category !== 'music') throw Error('来源不是音乐项目。')
    musicSourceEntry(next.content?.music, props.source)
    if (!alive || token !== epoch) { next.dispose(); return }; reader.value = next
  } catch (cause) { next.dispose(); if (alive && token === epoch) error.value = readerError(cause) }
  finally { if (alive && token === epoch) busy.value = false }
}
watch(() => props.source, () => void load(), { immediate: true })
const stop = appSDK.lifecycle.subscribe(event => {
  if (event === 'hidden') { hidden = true; clear() }
  if (event === 'active') { hidden = false; void load() }
  if (event === 'dispose') { alive = false; clear() }
})
onBeforeUnmount(() => { alive = false; clear(); stop() })
</script>
<template>
  <article class="music-source">
    <p v-if="busy" role="status">正在读取固定来源…</p>
    <div v-else-if="error" role="alert">{{ error }} <button class="btn" @click="load">重试</button></div>
    <template v-else-if="reader?.project && entry">
      <strong>{{ entry.title }}</strong><p class="muted">{{ reader.project.manifest.name }} · {{ source.kind === 'recording' ? '干声录音' : source.kind === 'model' ? '人声模型' : '歌曲' }}</p>
      <p v-if="version">{{ version.label }} · {{ musicVersionKinds[version.kind] }} · {{ musicReview(version.review) }}</p>
      <p v-else class="muted">历史文件，未登记制作版本与审核。{{ source.kind === 'song' ? '不能据此确认母带或已批准音乐。' : '' }}</p>
      <RouterLink class="btn" :to="target">查看固定来源</RouterLink><small>固定修订 {{ source.revision_id }}</small>
    </template>
  </article>
</template>
<style scoped>
.music-source { padding:16px 0; border-top:1px solid var(--line); display:grid; gap:8px; overflow-wrap:anywhere; }.muted,small { color:var(--muted); }.btn { justify-self:start; }[role=alert] { color:var(--danger); }
</style>
