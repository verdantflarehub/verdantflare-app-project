<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { useHost } from '../platform/host'
import { createAssetImport, importError, type ImportProject, type ImportRecord, type ImportResult, type ImportState, type ImportTarget } from '../platform/asset-import'
import { isProjectID } from '../platform/project-reader'
import { projectTarget, worldTarget } from '../platform/route-links'

const route = useRoute(), router = useRouter(), { appSDK, dashboardHost } = useHost()
const asset = computed(() => typeof route.query.asset === 'string' ? route.query.asset : '')
const version = computed(() => typeof route.query.version === 'string' ? route.query.version : '')
const client = shallowRef<ReturnType<typeof createAssetImport>>()
const projects = ref<ImportProject[]>([]), cursor = ref(''), chosen = ref(''), target = ref<ImportTarget>(), purpose = ref('')
const pending = ref<ImportRecord>(), state = ref<ImportState>('unknown'), result = ref<ImportResult>()
const busy = ref(false), error = ref(''), hidden = ref(false), ready = ref(false)
let alive = true, generation = 0, scroll = 0
let navigationRequest: AbortController | undefined
const short = (v: string) => v.slice(0, 8) + '…'
const existing = computed(() => target.value?.asset_refs.filter(ref => ref.asset_id === asset.value && ref.purpose === purpose.value.trim()) || [])
const stateText: Record<ImportState, string> = { unknown: '结果待确认', preparing: '正在准备，请恢复原提交', committed: '已保存固定引用', conflict: '目标项目修订已变化', failed: '原提交已失败' }
function sync() { pending.value = client.value?.pending() }
const current = (c: NonNullable<typeof client.value>) => alive && client.value === c
async function task(work: (c: NonNullable<typeof client.value>) => Promise<void>) {
  if (!client.value || busy.value || hidden.value) return
  busy.value = true; error.value = ''; const current = generation, c = client.value
  try { await work(c) } catch (reason) { if (alive && current === generation && !hidden.value) error.value = importError(reason) }
  finally { if (alive && current === generation) { sync(); busy.value = false } }
}
function loadMore() { void task(async c => { const data = await c.list(cursor.value); if (!current(c)) return; projects.value = [...new Map([...projects.value, ...data.items].map(p => [p.project_id, p])).values()]; cursor.value = data.next_cursor; ready.value = true }) }
function selectTarget() {
  target.value = undefined
  if (!chosen.value) return
  void task(async c => { const value = await c.open(chosen.value); if (current(c)) target.value = value })
}
function confirm() {
  const selected = target.value, use = purpose.value.trim()
  if (!selected || !use || pending.value) return
  void task(async c => { const journal = await c.begin(selected, use); if (!current(c)) return; pending.value = journal; const value = await c.submit(); if (current(c)) { result.value = value; state.value = 'committed' } })
}
function retry() { void task(async c => { const value = await c.submit(); if (current(c)) { result.value = value; state.value = 'committed' } }) }
function status() { void task(async c => { const next = await c.status(); if (current(c)) { state.value = next.state; result.value = next.result } }) }
function selectAgain() {
  void task(async c => {
    c.clearTerminal(); pending.value = undefined; result.value = undefined; state.value = 'unknown'; target.value = undefined; chosen.value = ''; projects.value = []; cursor.value = ''; ready.value = false
    const data = await c.list(); if (current(c)) { projects.value = data.items; cursor.value = data.next_cursor; ready.value = true }
  })
}
async function finish() {
  if (!client.value || !result.value || busy.value) return
  const next = projectTarget(result.value.project_id, result.value.revision_id)
  try { client.value.clearTerminal(); sync(); await router.push(next.path) } catch (reason) { error.value = importError(reason) }
}
async function backToWorld() {
  if (busy.value || hidden.value || !client.value) return
  navigationRequest?.abort(); const controller = new AbortController(); navigationRequest = controller
  try { const next = worldTarget(asset.value, version.value); await appSDK.capabilities.call('navigation.openApp', { appId: next.appId, path: next.path }, { signal: controller.signal }) }
  catch { if (alive && !controller.signal.aborted) error.value = '无法返回数字世界，请确认应用可访问。原提交记录仍保留。' }
}
function mayLeave() { if (!busy.value) return true; error.value = '操作仍在进行，请等结果确认后再离开。'; return false }
onBeforeRouteLeave(mayLeave); onBeforeRouteUpdate(mayLeave)
watch(busy, value => dashboardHost.setDirty(value), { flush: 'sync' })
watch([asset, version], () => {
  generation++; client.value?.dispose(); client.value = undefined; projects.value = []; cursor.value = ''; target.value = undefined; chosen.value = ''; purpose.value = ''; pending.value = undefined; state.value = 'unknown'; result.value = undefined; error.value = ''; ready.value = false
  if (!isProjectID(asset.value) || !isProjectID(version.value)) { error.value = '资产地址缺少有效的固定版本，请从数字世界重新选择。'; return }
  try { client.value = createAssetImport(appSDK, window.localStorage, asset.value, version.value); sync(); if (!pending.value) loadMore() }
  catch (reason) { error.value = importError(reason) }
}, { immediate: true })
const unsubscribe = appSDK.lifecycle.subscribe(event => {
  if (event === 'hidden') { scroll = dashboardHost.getScroll(); hidden.value = true; navigationRequest?.abort() }
  if (event === 'active') { hidden.value = false; dashboardHost.restoreScroll(scroll); if (!pending.value && !ready.value && !busy.value) loadMore() }
  if (event === 'dispose') { alive = false; generation++; navigationRequest?.abort() }
})
const scope = () => { const c = appSDK.context.get(); return JSON.stringify([c.userId, c.organizationId, c.stationId, c.appId, c.installationId, c.contextRevision]) }
const initialScope = scope()
const stopContext = appSDK.context.subscribe(() => {
  if (scope() === initialScope) return
  generation++; client.value?.dispose(); client.value = undefined; projects.value = []; target.value = undefined; pending.value = undefined; result.value = undefined; busy.value = false; error.value = '会话已变化，请重新打开应用。原提交恢复记录仍保留。'
})
onBeforeUnmount(() => { alive = false; generation++; navigationRequest?.abort(); client.value?.dispose(); unsubscribe(); stopContext(); dashboardHost.setDirty(false) })
</script>

<template>
  <main class="main use-asset">
    <header class="heading"><div class="heading-left"><div class="eyebrow">WORLD → PROJECT</div><h1>带入项目</h1><p>选择项目与用途，保存数字世界资产的固定版本。</p></div><button class="btn" :disabled="busy || hidden || !client" @click="backToWorld">返回数字世界</button></header>
    <section class="panel"><h2>所选资产版本</h2><dl><dt>资产</dt><dd><code>{{ asset }}</code></dd><dt>固定版本</dt><dd><code>{{ version }}</code></dd></dl><p class="hint">资产后续发布新版本不会自动改变这次保存的引用。</p></section>
    <p v-if="busy" class="state" role="status">正在处理…</p><p v-if="error" class="state error" role="alert">{{ error }}</p>
    <section v-if="pending" class="panel">
      <h2>{{ stateText[state] }}</h2><dl><dt>目标项目</dt><dd><code>{{ pending.request.project_id }}</code></dd><dt>提交基准</dt><dd><code>{{ pending.request.expected_revision_id }}</code></dd><dt>用途</dt><dd>{{ pending.request.purpose }}</dd></dl>
      <p v-if="state === 'unknown' || state === 'preparing'" class="hint">提交恢复记录已保存。查询结果或重试会使用同一个提交编号和原始内容。</p>
      <p v-if="state === 'conflict' || state === 'failed'" class="hint">原提交已到达终态。重新选择后将读取项目当前修订，再由你确认保存。</p>
      <div class="actions">
        <button v-if="state !== 'committed'" class="btn" :disabled="busy || hidden" @click="status">查询提交结果</button>
        <button v-if="state === 'unknown' || state === 'preparing'" class="btn primary" :disabled="busy || hidden" @click="retry">重试原提交</button>
        <button v-if="state === 'conflict' || state === 'failed'" class="btn" :disabled="busy || hidden" @click="selectAgain">重新选择并检查修订</button>
        <button v-if="state === 'committed' && result" class="btn primary" :disabled="busy || hidden" @click="finish">完成并查看项目</button>
      </div>
    </section>
    <section v-else-if="client" class="panel">
      <h2>选择可写项目</h2><p v-if="ready && !projects.length" class="hint">当前身份没有可写项目。请先创建项目或取得项目编辑权限。</p>
      <label>目标项目<select v-model="chosen" :disabled="busy || hidden" @change="selectTarget"><option value="">请选择项目</option><option v-for="project in projects" :key="project.project_id" :value="project.project_id">{{ project.name }} · {{ project.category }} · {{ short(project.project_id) }}</option></select></label>
      <button v-if="cursor" class="btn more" :disabled="busy || hidden" @click="loadMore">加载更多项目</button>
      <template v-if="target"><p class="hint">确认目标：{{ target.name }}。当前修订 <code>{{ target.revision_id }}</code></p><label>引用用途<input v-model="purpose" :disabled="busy || hidden" maxlength="256" placeholder="例如：主角形象、背景音乐"></label><p v-if="existing.length" class="hint">此项目已为相同用途引用该资产。本次将替换为所选固定版本，历史修订保持原引用。</p><button class="btn primary confirm" :disabled="busy || hidden || !purpose.trim()" @click="confirm">确认带入并保存项目修订</button></template>
      <button v-if="!ready && !busy" class="btn more" :disabled="hidden" @click="loadMore">重新读取项目</button>
    </section>
  </main>
</template>

<style scoped>
.eyebrow { font-size: 11px; letter-spacing: 2px; color: var(--accent); }
.panel { padding: 24px; border: 1px solid var(--line); border-radius: 12px; background: var(--shell-panel); margin-bottom: 20px; min-width: 0; }
h2 { font-size: 17px; margin-bottom: 18px; }dl { display: grid; grid-template-columns: 90px minmax(0,1fr); gap: 10px 16px; }dt,.hint { color: var(--muted); }dd,code { overflow-wrap: anywhere; }.hint { margin: 16px 0; }
label { display: grid; gap: 8px; color: var(--muted); margin-top: 16px; }select,input { width: min(100%,680px); color: var(--ink); border: 1px solid var(--line); background: var(--subtle); border-radius: 7px; padding: 9px 12px; }
.actions { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 20px; }.more,.confirm { margin-top: 20px; }.state { padding: 16px; margin-bottom: 20px; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); color: var(--muted); }.error { color: var(--danger); }
</style>
