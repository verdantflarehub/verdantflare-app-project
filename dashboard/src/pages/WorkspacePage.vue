<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute } from 'vue-router'
import { useHost } from '../platform/host'
import { isProjectID } from '../platform/project-reader'
import { conflictPreview, createWorkspaceClient, snapshot, workspaceError, workspaceErrorCode, type ConflictPreview, type Resolution, type WorkspaceRequest, type WorkspaceSnapshot } from '../platform/workspace-client'

const route = useRoute(), { appSDK, dashboardHost } = useHost(), client = createWorkspaceClient(appSDK)
const projectID = computed(() => typeof route.query.project === 'string' ? route.query.project : '')
const copy = ref<WorkspaceSnapshot>(), handle = ref(''), selected = ref<string[]>([])
const busy = ref(false), error = ref(''), notice = ref(''), hidden = ref(false), importRole = ref('reference')
const preview = ref<ConflictPreview>(), choices = ref<Resolution['choices']>([]), mergeDirty = ref(false)
let alive = true, scroll = 0
const canUse = computed(() => !busy.value && !hidden.value && !!copy.value)
const states: Record<string, string> = { clean: '已同步', not_materialized: '未下载', missing: '本机文件缺失', modified: '本机已修改', conflict: '本机路径冲突' }
function remember(next?: WorkspaceSnapshot) {
  copy.value = next; handle.value = next?.workspace_handle || ''; client.remember(projectID.value, handle.value || undefined)
  if (!next) selected.value = []
  else selected.value = selected.value.filter(id => next.files.some(f => f.file_id === id))
}
function failure(reason: unknown) {
  if (!alive) return
  if (reason instanceof DOMException && reason.name === 'AbortError' && hidden.value) return
  error.value = workspaceError(reason)
  if (['PERMISSION_DENIED', 'FORBIDDEN', 'CAPABILITY_DENIED', 'SESSION_NOT_FOUND', 'SESSION_REVOKED', 'CONTEXT_CHANGED'].includes(workspaceErrorCode(reason))) remember()
}
async function load() {
  const value = await client.call({ operation: 'status', workspace_handle: handle.value })
  if (alive) remember(snapshot(value, projectID.value))
}
async function operate(request: WorkspaceRequest) {
  if (busy.value || hidden.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const value = await client.call(request)
    if (!alive) return
    if (value.cancelled === true) { notice.value = '已取消选择。'; return }
    if (request.operation === 'close') { remember(); preview.value = undefined; mergeDirty.value = false; notice.value = '已关闭目录授权，本机内容和恢复记录仍保留。'; return }
    if (request.operation === 'openDirectory' || request.operation === 'status' || request.operation === 'switchToHead') remember(snapshot(value, projectID.value))
    else if (request.operation === 'previewConflict') {
      preview.value = conflictPreview(value, projectID.value)
      choices.value = preview.value.files.map(f => ({ index: f.index, choice: 'head' }))
      mergeDirty.value = false
    } else {
      if (request.operation !== 'fetch') {
        if (value.project_id !== projectID.value || typeof value.revision_id !== 'string' || !isProjectID(value.revision_id)) throw new Error('INVALID_WORKSPACE_RESPONSE')
        notice.value = '已保存项目修订。'; preview.value = undefined; mergeDirty.value = false
      } else notice.value = '文件已下载到所选目录。'
      if (!hidden.value) await load()
    }
  } catch (reason) { failure(reason) }
  finally { if (alive) busy.value = false }
}
function open() { if (isProjectID(projectID.value)) void operate({ operation: 'openDirectory', project_id: projectID.value }) }
function simple(operation: 'status' | 'resume' | 'previewConflict' | 'switchToHead' | 'close') { if (handle.value) void operate({ operation, workspace_handle: handle.value }) }
function save(operation: 'saveTexts' | 'saveFiles') { if (selected.value.length) void operate({ operation, workspace_handle: handle.value, file_ids: [...selected.value] }) }
function importFiles() { void operate({ operation: 'importFiles', workspace_handle: handle.value, role: importRole.value }) }
function choose(index: number, value: string) {
  const file = preview.value?.files.find(f => f.index === index), choice = choices.value.find(c => c.index === index)
  if (!file || !choice || !['head', 'pending', 'text'].includes(value) || (value !== 'head' && !file.can_keep_pending) || (value === 'text' && file.pending.text === undefined)) return
  choice.choice = value as 'head' | 'pending' | 'text'
  if (value === 'text') choice.text = file.pending.text || ''; else delete choice.text
  mergeDirty.value = true
}
function resolve() {
  const p = preview.value
  if (!p) return
  void operate({ operation: 'resolveConflict', workspace_handle: handle.value, resolution: { commit_id: p.commit_id, request_sha256: p.request_sha256, head_revision_id: p.head_revision_id, choices: choices.value.map(c => ({ ...c })) } })
}
function discardPreview() { preview.value = undefined; choices.value = []; mergeDirty.value = false }
function mayLeave() {
  if (!busy.value && !mergeDirty.value) return true
  error.value = busy.value ? '操作仍在进行，请等结果确认后再离开。' : '冲突处理尚未提交。请先提交，或取消本次冲突处理。'
  return false
}
onBeforeRouteLeave(mayLeave); onBeforeRouteUpdate(mayLeave)
watch([busy, mergeDirty], () => dashboardHost.setDirty(busy.value || mergeDirty.value), { flush: 'sync' })
watch(projectID, () => {
  copy.value = undefined; selected.value = []; discardPreview(); error.value = ''; notice.value = ''
  handle.value = client.retained(projectID.value) || ''
  if (!isProjectID(projectID.value)) error.value = '请从项目详情选择有效的项目。'
  else if (handle.value) simple('status')
}, { immediate: true })
const stopLifecycle = appSDK.lifecycle.subscribe(event => {
  if (event === 'hidden') { scroll = dashboardHost.getScroll(); hidden.value = true }
  if (event === 'active') { hidden.value = false; dashboardHost.restoreScroll(scroll); if (handle.value && !busy.value) simple('status') }
  if (event === 'dispose') { alive = false; client.dispose() }
})
const scope = () => { const c = appSDK.context.get(); return JSON.stringify([c.userId, c.organizationId, c.stationId, c.appId, c.installationId, c.contextRevision]) }
const initialContext = scope()
const stopContext = appSDK.context.subscribe(context => {
  if (scope() !== initialContext) { copy.value = undefined; handle.value = ''; discardPreview(); error.value = '会话已变化，请重新打开项目工作副本。'; busy.value = false }
})
onBeforeUnmount(() => { alive = false; stopLifecycle(); stopContext(); client.dispose(); dashboardHost.setDirty(false) })
</script>

<template>
  <main class="main local-copy">
    <nav class="breadcrumbs" aria-label="面包屑"><RouterLink :to="{ path: '/projects', query: { project: projectID } }">项目</RouterLink><span>/</span><span>本机工作副本</span></nav>
    <header class="heading">
      <div class="heading-left"><div class="eyebrow">LOCAL WORKSPACE</div><h1>{{ copy?.manifest.name || '本机工作副本' }}</h1><p>选择本机目录，按需下载、编辑并保存项目文件。</p></div>
      <button v-if="!copy" class="btn primary" :disabled="busy || hidden || !isProjectID(projectID)" @click="open">选择工作目录</button>
      <button v-else class="btn" :disabled="!canUse || mergeDirty" @click="simple('close')">关闭工作副本</button>
    </header>
    <p v-if="busy" class="state" role="status">正在处理…请等待操作结果。</p>
    <p v-if="error" class="state error" role="alert">{{ error }}</p>
    <p v-if="notice" class="state" role="status">{{ notice }}</p>
    <section v-if="!copy" class="panel empty"><h2>从 Studio 桌面应用连接本机目录</h2><p>新目录从项目当前修订建立副本，已有目录保留原修订和恢复记录。文件修改由你明确选择后保存。</p></section>
    <template v-else>
      <section class="panel">
        <div class="revision"><span>本机基准修订</span><code>{{ copy.state.base_revision_id }}</code></div>
        <p class="hint">请在本机编辑器修改下载的文件，再选择要保存的文件。缺失文件不会自动从项目删除。</p>
        <div class="actions">
          <button class="btn" :disabled="!canUse" @click="simple('status')">刷新状态</button>
          <button class="btn" :disabled="!canUse || !selected.length" @click="save('saveTexts')">保存选中文本</button>
          <button class="btn" :disabled="!canUse || !selected.length" @click="save('saveFiles')">保存选中文件</button>
          <label class="inline-label">导入用途<select v-model="importRole" :disabled="!canUse"><option value="reference">参考素材</option><option value="output">输出素材</option><option value="review">审核材料</option></select></label>
          <button class="btn" :disabled="!canUse" @click="importFiles">选择文件并导入保存</button>
        </div>
        <p v-if="!copy.files.length" class="state">此修订还没有文件。</p>
        <ul v-else class="files" aria-label="工作副本文件">
          <li v-for="file in copy.files" :key="file.file_id">
            <label><input v-model="selected" type="checkbox" :value="file.file_id" :disabled="!canUse"><span class="file-path">{{ file.path }}</span></label>
            <span class="file-state" :class="{ changed: file.state !== 'clean' && file.state !== 'not_materialized' }">{{ states[file.state] }}</span>
            <button class="btn" :disabled="!canUse || !['not_materialized', 'missing'].includes(file.state)" @click="operate({ operation: 'fetch', workspace_handle: handle, file_id: file.file_id })">下载文件</button>
          </li>
        </ul>
      </section>
      <section class="panel">
        <h2>提交恢复与修订</h2><p class="hint">保存中断或结果未确认时，恢复原提交。远端已更新造成冲突时，先预览再决定。切换到最新修订要求本机无修改且没有待处理提交。</p>
        <div class="actions"><button class="btn" :disabled="!canUse || mergeDirty" @click="simple('resume')">恢复原提交</button><button class="btn" :disabled="!canUse || mergeDirty" @click="simple('previewConflict')">预览提交冲突</button><button class="btn" :disabled="!canUse || !!preview" @click="simple('switchToHead')">切换到最新修订</button></div>
      </section>
      <section v-if="preview" class="panel">
        <h2>处理已确认的提交冲突</h2><p class="hint">当前目标修订：<code>{{ preview.head_revision_id }}</code>。提交前将重新检查远端状态。</p>
        <div v-for="(file, position) in preview.files" :key="file.index" class="conflict-file">
          <label :for="`choice-${file.index}`">{{ file.pending.path }} · {{ file.remote_changed ? '远端已修改' : '远端未修改' }}</label>
          <select :id="`choice-${file.index}`" :value="choices[position]?.choice" :disabled="!canUse" @change="choose(file.index, ($event.target as HTMLSelectElement).value)">
            <option value="head">采用远端内容</option><option v-if="file.can_keep_pending" value="pending">保留本机提交</option><option v-if="file.can_keep_pending && file.pending.text !== undefined" value="text">编辑合并文本</option>
          </select>
          <textarea v-if="choices[position]?.choice === 'text'" v-model="choices[position]!.text" :disabled="!canUse" :aria-label="`${file.pending.path} 合并文本`" rows="8" @input="mergeDirty = true" />
        </div>
        <div class="actions"><button class="btn primary" :disabled="!canUse" @click="resolve">提交冲突处理</button><button class="btn" :disabled="!canUse" @click="discardPreview">取消本次冲突处理</button></div>
      </section>
    </template>
  </main>
</template>

<style scoped>
.breadcrumbs { display: flex; flex-wrap: wrap; gap: 10px; color: var(--muted); margin-bottom: 20px; }
.eyebrow { font-size: 11px; letter-spacing: 2px; color: var(--accent); }
.panel { padding: 24px; border: 1px solid var(--line); border-radius: 12px; background: var(--shell-panel); margin-bottom: 20px; min-width: 0; }
h2 { font-size: 17px; margin-bottom: 12px; }.empty p,.hint { color: var(--muted); }.hint { margin: 12px 0 18px; }
.revision { display: flex; flex-wrap: wrap; gap: 12px; }.revision span { color: var(--muted); }code { overflow-wrap: anywhere; }
.actions { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }.inline-label { display: flex; gap: 8px; align-items: center; color: var(--muted); }
select,textarea { color: var(--ink); background: var(--subtle); border: 1px solid var(--line); border-radius: 6px; padding: 6px 10px; }
.files { list-style: none; margin-top: 20px; }.files li { display: flex; flex-wrap: wrap; align-items: center; gap: 16px; padding: 14px 0; border-top: 1px solid var(--line); }.files label { flex: 1; min-width: 180px; display: flex; align-items: center; gap: 12px; }.file-path { overflow-wrap: anywhere; }.file-state { font-size: 12px; color: var(--muted); }.file-state.changed { color: var(--amber); }
.state { margin: 0 0 20px; padding: 16px; background: var(--panel); border: 1px solid var(--line); border-radius: 8px; color: var(--muted); }.error { color: var(--danger); }
.conflict-file { display: grid; gap: 12px; margin: 18px 0; }.conflict-file label { overflow-wrap: anywhere; }.conflict-file select { justify-self: start; }.conflict-file textarea { width: 100%; resize: vertical; }
</style>
