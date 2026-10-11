<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useHost } from '../../platform/host'
const props = defineProps<{ projectId: string; revisionId: string; target: string }>()
const { appSDK } = useHost(), scope = shallowRef(appSDK.context.get()), note = ref(''), message = ref('')
const unsubscribe = appSDK.context.subscribe(value => { scope.value = value })
onBeforeUnmount(unsubscribe)
const storageKey = computed(() => scope.value.userId && scope.value.organizationId
  ? 'project.drama.tips:' + JSON.stringify([scope.value.organizationId, scope.value.userId, scope.value.stationId, scope.value.installationId, props.projectId, props.revisionId, props.target]) : '')
watch(storageKey, key => { note.value = ''; message.value = ''; if (key) { try { note.value = localStorage.getItem(key) || '' } catch { message.value = '无法读取本机提示。' } } }, { immediate: true })
function save() { try { if (!storageKey.value) throw new Error(); localStorage.setItem(storageKey.value, note.value); message.value = '提示已保存在本机。' } catch { message.value = '保存失败，输入仍保留，请复制备份后重试。' } }
</script>
<template><section class="panel"><h2>修改提示 · Tips</h2><p class="muted">这里只记录建议，不会修改剧本或素材。实际修改由 AI 经 MCP 执行。</p><label>提示内容<textarea v-model="note" rows="4" maxlength="20000" /></label><button class="btn" :disabled="!storageKey" @click="save">保存提示</button><p v-if="message" role="status" class="muted">{{ message }}</p></section></template>
<style scoped>
.panel { background: var(--shell-panel); border: 1px solid var(--line); border-radius: 8px; padding: 20px; min-width: 0; }h2 { font-size: 15px; margin-bottom: 14px; }.muted { color: var(--muted); font-size: 13px; line-height: 1.85; }label { display: grid; gap: 8px; margin: 12px 0; }textarea { width: 100%; resize: vertical; padding: 12px; border-radius: 8px; border: 1px solid var(--line); background: var(--panel); color: var(--ink); font: inherit; }
</style>
