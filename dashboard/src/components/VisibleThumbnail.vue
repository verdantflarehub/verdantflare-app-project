<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useVisible } from '../platform/visible'
const props = defineProps<{ cacheKey: string; load: (eligible: () => boolean) => Promise<string>; label: string }>()
const root = ref<HTMLElement | null>(null), visible = useVisible(root), source = ref(''), failed = ref(false)
let generation = 0
const pending = ref(false)
watch(() => props.cacheKey, () => { generation++; source.value = ''; failed.value = false; pending.value = false })
watch([visible, () => props.cacheKey, pending], async () => {
  if (!visible.value || source.value || pending.value || failed.value) return
  const current = generation; pending.value = true
  try { const url = await props.load(() => visible.value && current === generation); if (current === generation) source.value = url }
  catch (cause) { if (current === generation && (cause as Error)?.name !== 'AbortError') failed.value = true }
  finally { if (current === generation) pending.value = false }
}, { flush: 'post' })
onBeforeUnmount(() => { generation++ })
</script>
<template><span ref="root" class="visible-thumbnail"><img v-if="source" :src="source" :alt="label" @error="failed = true; source = ''"><span v-else :aria-label="label" class="placeholder">{{ failed ? '预览暂不可用' : '' }}</span></span></template>
<style scoped>
.visible-thumbnail { display: block; width: 100%; height: 100%; min-height: 80px; background: var(--subtle); }
img { display: block; width: 100%; height: 100%; object-fit: contain; }.placeholder { display: grid; min-height: 80px; height: 100%; place-items: center; color: var(--muted); font-size: 12px; }
</style>
