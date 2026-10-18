<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ProjectReader } from '../platform/project-reader'
import ProjectMedia from './ProjectMedia.vue'
import DramaDocument from './drama/DramaDocument.vue'

const props = defineProps<{ reader: ProjectReader; fileId: string }>()
const expanded = ref(false)
const file = computed(() => props.reader.file(props.fileId))
// Extension chooses a viewer only; ProjectReader checks the actual Artifact MIME and bytes.
const kind = computed(() => {
  const path = file.value.path
  if (/\.(md|txt|json)$/i.test(path)) return 'document'
  if (/\.(png|jpe?g|webp|gif)$/i.test(path)) return 'image'
  if (/\.(mp3|wav|flac|ogg|m4a)$/i.test(path)) return 'audio'
  if (/\.(mp4|webm)$/i.test(path)) return 'video'
  return 'file'
})
</script>

<template>
  <details class="project-attachment" @toggle="expanded = ($event.target as HTMLDetailsElement).open">
    <summary :title="file.path">{{ file.path.split('/').slice(-2).join(' / ') }}</summary>
    <div v-if="expanded" class="attachment-content">
      <DramaDocument v-if="kind === 'document'" :reader="reader" :file-id="fileId" :title="file.path.split('/').pop() || file.path" initially-expanded />
      <ProjectMedia v-else :reader="reader" :file-id="fileId" :kind="kind" :label="file.path" />
      <ProjectMedia v-if="kind !== 'file'" :reader="reader" :file-id="fileId" kind="file" label="原文件" />
    </div>
  </details>
</template>

<style scoped>
.project-attachment { border-bottom: 1px solid var(--line); padding: 12px 0; min-width: 0; }
summary { cursor: pointer; overflow-wrap: anywhere; line-height: 1.6; }
.attachment-content { display: grid; gap: 12px; margin-top: 12px; min-width: 0; }
</style>
