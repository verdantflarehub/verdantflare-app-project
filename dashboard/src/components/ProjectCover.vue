<script setup lang="ts">
import { onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { ProjectReader, type ProjectItem } from '../platform/project-reader'
import ProjectMedia from './ProjectMedia.vue'
import { useVisible } from '../platform/visible'
import { coverFile, defaultCoverPosition, type CoverPosition } from '../platform/cover-position'
const props = defineProps<{ project: ProjectItem }>()
const root = ref<HTMLElement | null>(null), visible = useVisible(root)
const reader = shallowRef<ProjectReader | null>(null), fileID = ref<string | null>(null)
const position = ref<CoverPosition>(defaultCoverPosition())
let generation = 0
watch(() => [props.project.project_id, props.project.head_revision_id], () => {
  generation++; reader.value?.dispose(); reader.value = null; fileID.value = null
})
watch(() => [visible.value, props.project.project_id, props.project.head_revision_id], async () => {
  if (!visible.value || reader.value) return
  const current = ++generation, next = new ProjectReader(props.project.project_id, props.project.head_revision_id)
  reader.value = next
  try {
    await next.load()
    if (current !== generation) return
    fileID.value = coverFile(next.content)
    position.value = next.content?.cover_position || defaultCoverPosition()
  } catch { if (current === generation) { next.dispose(); reader.value = null } }
})
onBeforeUnmount(() => { generation++; reader.value?.dispose() })
</script>

<template>
  <div ref="root" class="project-cover">
    <ProjectMedia
      v-if="reader && fileID"
      :reader="reader"
      :file-id="fileID"
      kind="image"
      :label="project.name"
      thumbnail
      :contain="project.category === 'image'"
      :object-position="position"
    />
    <span v-else aria-hidden="true">{{
      project.category === "music"
        ? "♫"
        : project.category === "video"
          ? "▷"
          : "◉"
    }}</span>
  </div>
</template>

<style scoped>
.project-cover {
  aspect-ratio: 16 / 10;
  display: grid;
  place-items: center;
  background: var(--subtle);
  border-bottom: 1px solid var(--line);
}
.project-cover > span {
  font-size: 56px;
  line-height: 1;
  color: var(--accent);
}
.project-cover > :deep(.project-media) {
  width: 100%;
  border-radius: 0;
}
</style>
