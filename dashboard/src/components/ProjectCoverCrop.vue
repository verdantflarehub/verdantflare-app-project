<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import type { ProjectReader } from '../platform/project-reader'
import { defaultCoverPosition, dragCover, type CoverPosition } from '../platform/cover-position'
import ProjectMedia from './ProjectMedia.vue'
const props = defineProps<{ reader: ProjectReader; fileId: string; position: CoverPosition; disabled: boolean }>()
const emit = defineEmits<{ 'update:position': [position: CoverPosition] }>()
const preview = ref<HTMLElement>()
let dragging: { id: number; x: number; y: number; width: number; height: number; naturalWidth: number; naturalHeight: number; position: CoverPosition } | undefined
function start(event: PointerEvent) {
  if (props.disabled || dragging || event.button !== 0 || !preview.value) return
  const image = preview.value.querySelector('img'); if (!image?.complete || !image.naturalWidth) return
  const rect = preview.value.getBoundingClientRect()
  dragging = { id: event.pointerId, x: event.clientX, y: event.clientY, width: rect.width, height: rect.height, naturalWidth: image.naturalWidth, naturalHeight: image.naturalHeight, position: { ...props.position } }
  preview.value.setPointerCapture(event.pointerId); event.preventDefault()
}
function move(event: PointerEvent) {
  const d = dragging; if (!d || d.id !== event.pointerId || props.disabled) return
  emit('update:position', dragCover(d.position, event.clientX - d.x, event.clientY - d.y, d.width, d.height, d.naturalWidth, d.naturalHeight))
}
function stop() { const id = dragging?.id; dragging = undefined; if (id !== undefined && preview.value?.hasPointerCapture(id)) preview.value.releasePointerCapture(id) }
watch(() => [props.fileId, props.disabled], stop)
onBeforeUnmount(stop)
function slider(axis: 'x' | 'y', event: Event) { if (!props.disabled) emit('update:position', { ...props.position, [axis]: Number((event.target as HTMLInputElement).value) }) }
</script>

<template>
  <div class="cover-crop"><div ref="preview" class="crop-preview" role="group" aria-label="16:10 封面裁切预览，可拖动调整位置" @pointerdown="start" @pointermove="move" @pointerup="stop" @pointercancel="stop" @lostpointercapture="stop"><ProjectMedia :reader="reader" :file-id="fileId" kind="image" label="所选封面" thumbnail :object-position="position" /></div><p>拖动调整展示位置，或使用下方滑杆。</p><label>横向位置 {{ position.x }}%<input type="range" aria-label="封面横向位置" min="0" max="100" step="1" :value="position.x" :disabled="disabled" @input="slider('x', $event)"></label><label>纵向位置 {{ position.y }}%<input type="range" aria-label="封面纵向位置" min="0" max="100" step="1" :value="position.y" :disabled="disabled" @input="slider('y', $event)"></label><button type="button" class="btn" :disabled="disabled" @click="emit('update:position', defaultCoverPosition())">恢复默认位置</button></div>
</template>

<style scoped>
.cover-crop { display:grid; gap:10px; color:var(--muted); }.crop-preview { width:100%; aspect-ratio:16/10; border:1px solid var(--line); border-radius:8px; overflow:hidden; touch-action:none; cursor:grab; }.crop-preview:active { cursor:grabbing; }.crop-preview :deep(.project-media) { width:100%; height:100%; }.crop-preview :deep(img) { user-select:none; -webkit-user-drag:none; }label { display:grid; gap:6px; }input { width:100%; accent-color:var(--accent); }.btn { justify-self:start; }
</style>
