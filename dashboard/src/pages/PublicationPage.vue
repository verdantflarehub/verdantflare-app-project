<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { useHost } from '../platform/host'
import { ProjectReader } from '../platform/project-reader'
import { createProjectPublication, receiptPointer, type PublicationIntent, type PublicationReceiptPointer } from '../platform/project-publication'
import { publicationError, publicationIdentity } from '../platform/publication-client'
import { projectTarget } from '../platform/route-links'
const route = useRoute(), router = useRouter(), { appSDK, dashboardHost } = useHost()
const query = (key: string) => typeof route.query[key] === 'string' ? route.query[key] as string : ''
const routeKey = computed(() => JSON.stringify(['project','prepared','candidate','commit','asset','version'].map(query)))
const client = shallowRef<ReturnType<typeof createProjectPublication>>(), pointer = shallowRef<PublicationReceiptPointer>(), pending = shallowRef<PublicationIntent>()
const busy = ref(false), hidden = ref(false), error = ref(''), name = ref(''), confirmed = ref(false), completed = ref('')
let alive = true, generation = 0, scroll = 0
async function snapshot(project: string, revision?: string) { const reader = new ProjectReader(project, revision); try { await reader.load(); if (!reader.project || !reader.content) throw Error(); return { project: reader.project, content: reader.content } } finally { reader.dispose() } }
async function task(work: (c: NonNullable<typeof client.value>, p: PublicationReceiptPointer) => Promise<void>) {
  if (!client.value || !pointer.value || busy.value || hidden.value) return
  const c = client.value, p = pointer.value, current = generation; busy.value = true; error.value = ''
  try { await work(c, p) } catch (reason) { if (alive && current === generation && !hidden.value) error.value = publicationError(reason) }
  finally { if (alive && current === generation) { pending.value = c.pending(); busy.value = false } }
}
function verify() {
  const record = client.value?.pending(), p = pointer.value
  if (record?.association && p && record.project_id === p.project_id && record.prepared_revision_id === p.prepared_revision_id && record.candidate_key === p.candidate_key && record.commit_id === p.commit_id && record.published_asset_id === p.asset_id && record.published_version_id === p.asset_version_id) {
    name.value = record.request?.name || '原入库操作'; confirmed.value = true; return
  }
  void task(async (c, p) => { const receipt = await c.recoverPointer(p); if (client.value === c && alive) { name.value = receipt.result.manifest.name; confirmed.value = true } })
}
function save() { void task(async (c, p) => { const result = await c.associate(p); if (client.value === c && alive) completed.value = result.revision_id }) }
function finish() { if (pointer.value && completed.value) void router.push(projectTarget(pointer.value.project_id, completed.value).path) }
function mayLeave() { if (!busy.value) return true; error.value = '操作仍在进行，请等结果确认后再离开。'; return false }
onBeforeRouteLeave(mayLeave); onBeforeRouteUpdate(mayLeave)
watch(busy, value => dashboardHost.setDirty(value), { flush: 'sync' })
watch(routeKey, () => {
  generation++; client.value?.dispose(); client.value = undefined; pointer.value = undefined; pending.value = undefined; error.value = ''; name.value = ''; confirmed.value = false; completed.value = ''
  try {
    const p = receiptPointer({ project_id: query('project'), prepared_revision_id: query('prepared'), candidate_key: query('candidate'), commit_id: query('commit'), asset_id: query('asset'), asset_version_id: query('version') })
    pointer.value = p; client.value = createProjectPublication(appSDK, localStorage, p.project_id, p.candidate_key, revision => snapshot(p.project_id, revision)); pending.value = client.value.pending(); verify()
  } catch (reason) { error.value = publicationError(reason) }
}, { immediate: true })
const initial = publicationIdentity(appSDK.context.get())
const stopContext = appSDK.context.subscribe(value => { if (publicationIdentity(value) !== initial) { generation++; client.value?.dispose(); client.value = undefined; pending.value = undefined; confirmed.value = false; name.value = ''; busy.value = false; error.value = '会话已变化，请重新打开应用。原操作记录仍保留。' } })
const stopLifecycle = appSDK.lifecycle.subscribe(event => {
  if (event === 'hidden') { hidden.value = true; scroll = dashboardHost.getScroll() }
  if (event === 'active') { hidden.value = false; dashboardHost.restoreScroll(scroll); if (!confirmed.value && !busy.value) verify() }
  if (event === 'dispose') { alive = false; generation++ }
})
onBeforeUnmount(() => { alive = false; generation++; client.value?.dispose(); stopContext(); stopLifecycle(); dashboardHost.setDirty(false) })
</script>
<template>
  <main class="main publication">
    <header class="heading"><div class="heading-left"><div class="eyebrow">项目 · 数字世界</div><h1>保存入库关联</h1><p>核对数字世界的真实入库结果，并同步到项目修订。</p></div><RouterLink class="btn" to="/projects">返回项目列表</RouterLink></header>
    <p v-if="busy" role="status" class="state">正在核对原操作…</p><p v-if="error" role="alert" class="state error">{{ error }}</p>
    <section v-if="confirmed && pointer" class="panel"><h2>{{ name }}</h2><p class="hint">固定版本已经入库：<code>{{ pointer.asset_version_id }}</code></p><p class="hint">保存只补充项目关联。若项目已更新，将核对原候选并保留其他修改。</p><p v-if="pending?.association?.state === 'conflict'" class="hint">已确认原保存发生修订冲突。再次恢复将读取最新修订并检查候选。</p><div class="actions"><button v-if="!completed" class="btn primary" :disabled="busy || hidden" @click="save">{{ pending?.association ? '恢复原项目关联' : '确认保存项目关联' }}</button><button v-else class="btn primary" :disabled="busy || hidden" @click="finish">关联已保存，查看项目</button></div></section>
    <button v-if="client && !confirmed && !busy" class="btn" :disabled="hidden" @click="verify">重新核对入库回执</button>
  </main>
</template>
<style scoped>
.eyebrow { color: var(--accent); font-size: 11px; letter-spacing: .12em; }.panel { padding: 24px; margin-bottom: 20px; min-width: 0; border: 1px solid var(--line); border-radius: 8px; background: var(--shell-panel); }h2 { font-size: 17px; margin-bottom: 16px; }.hint { color: var(--muted); margin: 12px 0; overflow-wrap: anywhere; }.actions { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 18px; }.state { padding: 16px; margin-bottom: 20px; border: 1px solid var(--line); border-radius: 8px; color: var(--muted); }.error { color: var(--danger); }
</style>
