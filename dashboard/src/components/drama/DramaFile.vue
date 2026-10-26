<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ProjectReader } from '../../platform/project-reader'
import ProjectMedia, { type MediaGallery } from '../ProjectMedia.vue'
const props = defineProps<{ reader: ProjectReader; fileId: string; label: string; hideLabel?: boolean; thumbnail?: boolean; gallery?: MediaGallery }>()
const kind = ref<'image' | 'audio' | 'video' | 'file' | null>(null), error = ref('')
const caption = computed(() => {
  try {
    const path = props.reader.file(props.fileId).path
    return path.split('/').pop() || props.label
  } catch { return props.label }
})
function inferKindFromPath(fileId: string): 'image' | 'audio' | 'video' | null {
  try {
    const ext = props.reader.file(fileId).path.split('.').pop()?.toLowerCase() || ''
    if (['png', 'jpg', 'jpeg', 'webp', 'gif'].includes(ext)) return 'image'
    if (['mp4', 'webm', 'mov'].includes(ext)) return 'video'
    if (['mp3', 'wav', 'flac', 'm4a', 'aac'].includes(ext)) return 'audio'
  } catch { /* Keep an explicit file entry. */ }
  return null
}
async function inspect() {
  kind.value = null; error.value = ''
  const inferred = inferKindFromPath(props.fileId)
  if (inferred) {
    kind.value = inferred
    return
  }
  // Auxiliary and unknown formats remain explicit file entries; never probe every file.
  kind.value = 'file'
}
watch(() => [props.reader, props.fileId], inspect, { immediate: true })
</script>
<template>
  <div class="drama-file" :class="{ tile: thumbnail }">
    <p v-if="!hideLabel && !thumbnail" class="muted">{{ label }}</p>
    <div v-if="thumbnail && kind" class="review-frame">
      <ProjectMedia :reader="reader" :file-id="fileId" :kind="kind" :label="caption" :thumbnail="kind === 'image' || kind === 'video'" contain :edge="640" :gallery="kind === 'image' ? gallery : undefined" />
    </div>
    <ProjectMedia v-else-if="kind" :reader="reader" :file-id="fileId" :kind="kind" :label="label" :gallery="kind === 'image' ? gallery : undefined" />
    <p v-else-if="error" role="alert">{{ error }} <button class="btn" @click="inspect">重试</button></p>
    <p v-else role="status">正在读取文件信息…</p>
    <p v-if="thumbnail && kind" class="tile-caption" :title="caption">{{ caption }}</p>
  </div>
</template>
<style scoped>
.drama-file { min-width: 0; }
.muted { color: var(--muted); margin-bottom: 8px; }
.tile { display: grid; gap: 6px; min-width: 0; min-height: 0; }
.review-frame { aspect-ratio: 4 / 3; min-height: 0; overflow: hidden; display: grid; place-items: center; background: var(--subtle); border: 1px solid var(--line); border-radius: 8px; }
.review-frame :deep(.project-media),
.review-frame :deep(.media-state),
.review-frame :deep(.thumb-frame) { width: 100%; height: 100%; min-height: 0; max-height: 100%; }
.review-frame :deep(.project-media.video .media-state),
.review-frame :deep(.project-media.file .media-state),
.review-frame :deep(.project-media.image .media-state) { min-height: 0; height: 100%; aspect-ratio: auto; padding: 8px; }
.review-frame :deep(img),
.review-frame :deep(video) { width: 100%; height: 100%; max-height: 100%; object-fit: contain; object-position: center; }
.tile-caption { margin: 0; font-size: 12px; line-height: 1.4; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
