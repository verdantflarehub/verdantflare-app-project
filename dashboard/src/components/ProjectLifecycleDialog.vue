<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref } from 'vue'
import { useHost } from '../platform/host'
import type { ProjectItem } from '../platform/project-content'
import { LifecycleError, lifecycleKey, newLifecycle, readLifecycle, runLifecycle, type LifecycleRecord } from '../platform/project-lifecycle'
const emit = defineEmits<{ changed: [] }>()
const { appSDK } = useHost(), context = appSDK.context.get(), key = lifecycleKey(context)
const dialog = ref<HTMLDialogElement>(), record = ref<LifecycleRecord | null>(null), busy = ref(false), error = ref(''), persisted = ref(false)
let alive = true
try { record.value = readLifecycle(localStorage, key); persisted.value = !!record.value } catch (cause) { error.value = String(cause) }
async function show() { await nextTick(); if (alive) dialog.value?.showModal() }
function open(item: ProjectItem, action: 'delete' | 'restore') {
  if (!record.value || !persisted.value) { try { record.value = newLifecycle(item, action); error.value = '' } catch (cause) { error.value = String(cause) } }
  void show()
}
defineExpose({ open })
async function submit() {
  if (!record.value || busy.value || !alive) return
  busy.value = true; error.value = ''
  try {
    const existing = readLifecycle(localStorage, key)
    if (existing && existing.request.change_id !== record.value.request.change_id) { record.value = existing; persisted.value = true; throw new LifecycleError('另一页面有原请求待确认，请先查询。', false) }
    const recovering = persisted.value
    localStorage.setItem(key, JSON.stringify(record.value)); persisted.value = true
    await runLifecycle(appSDK, record.value, recovering)
    localStorage.removeItem(key)
    if (!alive) return
    record.value = null; persisted.value = false; dialog.value?.close(); emit('changed')
  } catch (cause) {
    if (!alive) return
    error.value = cause instanceof Error ? cause.message : '结果尚未确认，请查询原请求。'
    if (cause instanceof LifecycleError && cause.definitive) { localStorage.removeItem(key); persisted.value = false; record.value = null; emit('changed') }
  } finally { if (alive) busy.value = false }
}
function close() { dialog.value?.close(); if (!persisted.value) record.value = null }
const stops = [appSDK.context.subscribe(c => { if (JSON.stringify(c) !== JSON.stringify(context)) { alive = false; dialog.value?.close(); record.value = null; error.value = '' } }), appSDK.lifecycle.subscribe(event => { if (event === 'hidden' || event === 'dispose') dialog.value?.close() })]
onBeforeUnmount(() => { alive = false; stops.forEach(stop => stop()) })
</script>
<template>
  <div v-if="persisted" class="revision-notice" role="status"><span>有一项删除或恢复结果待确认。</span><button class="btn" @click="show">查询原请求</button></div>
  <p v-if="error && !record" role="alert">{{ error }}</p>
  <dialog ref="dialog" aria-labelledby="lifecycle-title" @cancel.prevent="close">
    <h2 id="lifecycle-title">{{ record?.action === 'restore' ? '恢复项目' : '删除项目' }}</h2>
    <p>“{{ record?.name }}”</p>
    <p>{{ record?.action === 'restore' ? '恢复后可继续打开和编辑，成员权限保持不变。' : '删除后无法打开或编辑，可从“已删除项目”中恢复。已保存的修订和共享素材会保留。' }}</p>
    <p v-if="error" role="alert">{{ error }}</p>
    <div class="actions"><button class="btn" @click="close">{{ persisted ? '稍后查询' : '取消' }}</button><button v-if="record" class="btn primary" :disabled="busy" @click="submit">{{ busy ? '正在确认…' : persisted ? '查询或重试原请求' : record.action === 'restore' ? '恢复项目' : '删除项目' }}</button></div>
  </dialog>
</template>
<style scoped>
dialog { position: fixed; inset: 10vh 0 auto; margin: auto; max-width: 520px; width: calc(100% - 48px); max-height: 80vh; overflow: auto; border: 1px solid var(--line); border-radius: 12px; padding: 24px; color: var(--ink); background: var(--panel); }
dialog::backdrop { background: var(--aside); opacity: .85; }
p { line-height: 1.7; overflow-wrap: anywhere; }
.actions { display: flex; justify-content: flex-end; gap: 12px; margin-top: 24px; }
</style>
