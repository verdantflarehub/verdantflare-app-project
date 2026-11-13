<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { Detail } from '../platform/project-content'
import type { ProjectReader } from '../platform/project-reader'
import { placeResultCards, resultCardRatio, resultColumnCount } from '../platform/result-cards'
import ProjectMedia from './ProjectMedia.vue'
import ProjectPrompt from './ProjectPrompt.vue'
const props = defineProps<{ reader: ProjectReader; detail: Detail }>()
const ratios = ref<Record<string, number>>({})
const items = computed(() => props.detail.result_file_ids.map((fileId, index) => {
  const meta = props.detail.result_meta?.[fileId]
  return { fileId, label: `结果 ${index + 1}${meta?.label ? ` · ${meta.label}` : ''}`, prompt: meta?.prompt }
}))
const gallery = computed(() => items.value.map(({ fileId, label }) => ({ fileId, label })))
const root = ref<HTMLElement | null>(null)
const width = ref(0)
const count = computed(() => resultColumnCount(width.value))
const columns = ref<number[][]>([])
let observer: ResizeObserver | undefined
watch(root, element => {
  observer?.disconnect()
  if (!element) return
  width.value = element.clientWidth
  observer = new ResizeObserver(entries => { width.value = entries[0].contentRect.width })
  observer.observe(element)
})
watch([items, count], () => {
  columns.value = placeResultCards(items.value.map(item => ratios.value[item.fileId] || 9 / 16), count.value)
}, { immediate: true })
const cards = computed(() => columns.value.map(column => column.map(index => ({ ...items.value[index], index }))))
onBeforeUnmount(() => observer?.disconnect())
</script>

<template>
  <div v-if="items.length" ref="root" class="result-cards" :style="{ '--columns': count }">
    <div v-for="(column, columnIndex) in cards" :key="columnIndex" class="result-column">
    <figure v-for="item in column" :key="item.fileId" :style="{ '--card-ratio': ratios[item.fileId] || 9 / 16 }">
      <ProjectMedia :reader="reader" :file-id="item.fileId" kind="image" :label="item.label" thumbnail contain :edge="640"
        :gallery="{ id: `extension-${detail.key}`, index: item.index, items: gallery }"
        @image-size="(width, height) => ratios[item.fileId] = resultCardRatio(width, height)" />
      <figcaption :title="item.label">{{ item.label }}</figcaption>
      <ProjectPrompt v-if="item.prompt?.trim()" :text="item.prompt" />
    </figure>
    </div>
  </div>
  <p v-else class="muted">未添加图片</p>
</template>

<style scoped>
.result-cards { display: grid; grid-template-columns: repeat(var(--columns), minmax(0, 240px)); align-items: start; gap: 12px; }
.result-column { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
figure { min-width: 0; margin: 0; }
.result-cards :deep(.thumbnail.contain) { height: auto; aspect-ratio: var(--card-ratio); transition: transform 150ms ease; }
.result-cards :deep(.image-open) { position: absolute; inset: 0; }
.result-cards :deep(.thumbnail.contain) { position: relative; }
.result-cards :deep(.image .media-state) { min-height: 0; position: absolute; inset: 0; }
.result-cards :deep(.thumbnail.contain:hover) { transform: translateY(-2px); }
figcaption { margin-top: 10px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
</style>
