<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { useHost } from '../platform/host'
import { createBodyLimit, createCategories, createError, createProjectCreator, createRequestBytes, type CreateRecord, type CreateResult, type CreateState } from '../platform/project-create'
import { projectTarget } from '../platform/route-links'

const props = defineProps<{ modal?: boolean; initialCategory?: string; returnToCaller?: boolean }>()
const emit = defineEmits<{ close: []; created: [result: CreateResult]; busy: [busy: boolean] }>()
const route = useRoute()
const initialCategory = createCategories.some(c => c.key === (props.initialCategory || route.query.category)) ? String(props.initialCategory || route.query.category) : 'character'
const router = useRouter(), { appSDK, dashboardHost } = useHost()
const initialText = '# 制作审核记录\n\n## 目标\n'
const name = ref(''), category = ref(initialCategory), entryPath = ref('review.md'), entryText = ref(initialText)
const editor = ref<HTMLTextAreaElement>(), client = shallowRef<ReturnType<typeof createProjectCreator>>()
const pending = ref<CreateRecord>(), result = ref<CreateResult>(), state = ref<CreateState>('unknown')
const busy = ref(false), hidden = ref(false), error = ref('')
let alive = true, generation = 0, scroll = 0
const stateText: Record<CreateState, string> = { unknown: '创建结果待确认', preparing: '创建仍在准备', committed: '项目已创建', conflict: '原创建发生冲突', failed: '原创建已失败' }
const bodyBytes = computed(() => createRequestBytes({ commit_id: '01900000-0000-7000-8000-000000000000', name: name.value.trim(), category: category.value, entry_path: entryPath.value.trim(), entry_text: entryText.value }))
const draftChanged = computed(() => !pending.value && (name.value !== '' || category.value !== initialCategory || entryPath.value !== 'review.md' || entryText.value !== initialText))
const current = (c: NonNullable<typeof client.value>) => alive && client.value === c
function sync() { pending.value = client.value?.pending(); state.value = client.value?.state() || 'unknown' }
async function task(work: (c: NonNullable<typeof client.value>) => Promise<void>) {
  if (!client.value || busy.value || hidden.value) return
  busy.value = true; error.value = ''; const revision = generation, c = client.value
  try { await work(c) } catch (reason) { if (alive && revision === generation) error.value = createError(reason) }
  finally { if (alive && revision === generation) { sync(); busy.value = false } }
}
function create() {
  if (pending.value || bodyBytes.value > createBodyLimit) return
  void task(async c => {
    await c.begin({ name: name.value, category: category.value, entry_path: entryPath.value, entry_text: entryText.value })
    if (!current(c)) return
    sync(); const value = await c.submit(); if (current(c)) result.value = value.result
  })
}
function recover() { void task(async c => { const value = await c.submit(); if (current(c)) result.value = value.result }) }
function resetDraft() { name.value = ''; category.value = initialCategory; entryPath.value = 'review.md'; entryText.value = initialText }
function createAgain() { void task(async c => { await c.clearTerminal(); if (current(c)) { result.value = undefined; resetDraft(); sync() } }) }
async function viewProject() {
  if (!client.value || !result.value || state.value !== 'committed' || busy.value || hidden.value) return
  const c = client.value, target = projectTarget(result.value.project_id, result.value.revision_id)
  if (props.returnToCaller) {
    void task(async original => { const value = result.value!; await original.clearTerminal(); if (current(original)) emit('created', value) })
    return
  }
  try { if (current(c)) await router.push(target.path); emit('close') } catch { if (current(c)) error.value = '项目已创建，暂时无法打开，请稍后重试。' }
}
function mayLeave() { if (!busy.value) return true; error.value = '正在确认创建结果，请稍后再离开。'; return false }
onBeforeRouteLeave(mayLeave); onBeforeRouteUpdate(mayLeave)
watch([busy, draftChanged], () => dashboardHost.setDirty(busy.value || draftChanged.value), { flush: 'sync' })
watch(busy, value => emit('busy', value), { flush: 'sync' })
watch([editor, entryText], async () => { await nextTick(); if (editor.value) { editor.value.style.height = 'auto'; editor.value.style.height = editor.value.scrollHeight + 'px' } }, { flush: 'post' })
try { client.value = createProjectCreator(appSDK, window.localStorage); sync() } catch (reason) { error.value = createError(reason) }
const scope = () => { const c = appSDK.context.get(); return JSON.stringify([c.userId, c.organizationId, c.stationId, c.appId, c.installationId, c.contextRevision]) }
const initialScope = scope()
function revoke() { generation++; client.value?.dispose(); client.value = undefined; pending.value = undefined; result.value = undefined; resetDraft(); state.value = 'unknown'; busy.value = false; dashboardHost.setDirty(false) }
const stopContext = appSDK.context.subscribe(() => { if (scope() !== initialScope) { revoke(); error.value = '会话已变化，请重新打开项目应用。原创建记录已保留。' } })
const stopLifecycle = appSDK.lifecycle.subscribe(event => {
  if (event === 'hidden') { scroll = dashboardHost.getScroll(); hidden.value = true }
  if (event === 'active') { hidden.value = false; dashboardHost.restoreScroll(scroll) }
  if (event === 'dispose') { alive = false; revoke() }
})
onBeforeUnmount(() => { alive = false; generation++; client.value?.dispose(); stopContext(); stopLifecycle(); dashboardHost.setDirty(false) })
</script>

<template>
  <main :class="modal ? 'create-form' : 'main'">
    <header class="heading"><div class="heading-left"><div class="eyebrow">PROJECTS</div><h1>新建项目</h1><p>填写名称与类型，开始创作。</p></div><RouterLink v-if="!modal" class="btn" :to="{ path: '/projects', query: { category: initialCategory } }">返回项目管理</RouterLink></header>
    <p v-if="error" class="state error" role="alert">{{ error }}</p>
    <p v-if="busy" class="state" role="status">正在确认创建结果…</p>
    <section v-if="pending" class="panel">
      <h2>{{ stateText[state] }}</h2>
      <dl><dt>项目名称</dt><dd>{{ pending.request.name }}</dd><dt>分类</dt><dd>{{ createCategories.find(c => c.key === pending!.request.category)?.label }}</dd><dt>入口文档</dt><dd>{{ pending.request.entry_path }}</dd></dl>
      <details><summary>查看原入口内容</summary><pre>{{ pending.request.entry_text }}</pre></details>
      <p v-if="state === 'unknown' || state === 'preparing'" class="hint">恢复时使用已保存的原内容。确认结果前，请继续恢复这次创建。</p>
      <p v-if="state === 'conflict' || state === 'failed'" class="hint">原创建已确认结束，可以保留原记录并开始新的创建。</p>
      <div class="actions">
        <button v-if="state === 'unknown' || state === 'preparing'" class="btn primary" :disabled="busy || hidden" @click="recover">{{ pending.result ? '查询创建结果' : '恢复上次创建' }}</button>
        <button v-if="state === 'committed' && result" class="btn primary" :disabled="busy || hidden" @click="viewProject">{{ returnToCaller ? '使用此角色' : '查看项目' }}</button>
        <button v-if="state === 'committed' || state === 'conflict' || state === 'failed'" class="btn" :disabled="busy || hidden" @click="createAgain">继续新建</button>
      </div>
    </section>
    <form v-else-if="client" class="panel" @submit.prevent="create">
      <h2>项目信息</h2>
      <label>项目名称<input v-model="name" :disabled="busy || hidden" required placeholder="填写项目名称"><span class="hint">最多 256 个字符</span></label>
      <label>项目分类<select v-model="category" :disabled="busy || hidden || returnToCaller"><option v-for="item in createCategories.filter(c => ['character', 'music', 'video', 'ai_short_drama'].includes(c.key))" :key="item.key" :value="item.key">{{ item.label }}</option></select></label>
      <details v-if="!modal"><summary>初始审核文档</summary><label>入口文档路径<input v-model="entryPath" :disabled="busy || hidden" required placeholder="review.md"><span class="hint">使用项目内的相对路径，以 .md 或 .txt 结尾。</span></label>
      <label>入口文档内容<textarea ref="editor" v-model="entryText" :disabled="busy || hidden" rows="8" spellcheck="false" /></label>
      <p class="hint" :class="{ error: bodyBytes > createBodyLimit }">保存大小 {{ (bodyBytes / 1024).toFixed(1) }} / 1024 KiB<span v-if="bodyBytes > createBodyLimit">，请缩短入口内容。</span></p></details>
      <button class="btn primary" type="submit" :disabled="busy || hidden || !name.trim() || !entryPath.trim() || bodyBytes > createBodyLimit">创建项目</button>
    </form>
  </main>
</template>

<style scoped>
.eyebrow { font-size: 11px; letter-spacing: 2px; color: var(--accent); }
.panel { padding: 24px; border: 1px solid var(--line); border-radius: 12px; background: var(--shell-panel); margin-bottom: 20px; min-width: 0; }
h2 { font-size: 17px; margin-bottom: 18px; }dl { display: grid; grid-template-columns: 90px minmax(0,1fr); gap: 10px 16px; }dt,.hint { color: var(--muted); }dd,pre { overflow-wrap: anywhere; }.hint { margin: 12px 0; }
label { display: grid; gap: 8px; color: var(--muted); margin-top: 16px; }label .hint { margin: 0; font-size: 12px; }select,input,textarea { width: 100%; color: var(--ink); border: 1px solid var(--line); background: var(--subtle); border-radius: 7px; padding: 9px 12px; }select,input { max-width: 680px; }textarea { overflow: hidden; resize: none; }select:focus-visible,textarea:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
.actions { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 20px; }.state { padding: 16px; margin-bottom: 20px; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); color: var(--muted); }.error { color: var(--danger); }details { margin-top: 20px; }summary { cursor: pointer; color: var(--muted); }pre { margin-top: 12px; white-space: pre-wrap; }
</style>
