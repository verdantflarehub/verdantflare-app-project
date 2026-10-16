<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ProjectReader, readerError, type ProjectOpen, type ProjectContent } from '../platform/project-reader'
import { revisionChanges, businessChanges } from '../platform/project-revision-diff'
import { useHost } from '../platform/host'

const props = defineProps<{ revision: ProjectOpen; parent?: ProjectOpen; historyBusy: boolean }>()
const route = useRoute(), { appSDK } = useHost()
const busy = ref(false), error = ref(''), afterContent = shallowRef<ProjectContent | null>(null), beforeContent = shallowRef<ProjectContent | null>(null)
const ready = ref(false)
const comparable = computed(() => !props.revision.parent_revision_id || !!props.parent)
const diff = computed(() => comparable.value ? revisionChanges(props.parent || null, props.revision) : null)
const business = computed(() => ready.value ? businessChanges(beforeContent.value, afterContent.value) : [])
const relationshipGroups = computed(() => diff.value ? [{ label: '资产引用', entries: diff.value.assets }, { label: '关联运行', entries: diff.value.runs }, { label: '领域文档', entries: diff.value.documents }, { label: '清单选用', entries: diff.value.selections }] : [])
let generation = 0, alive = true, hidden = false, readers: ProjectReader[] = []
function clear() { generation++; readers.forEach(r => r.dispose()); readers = []; busy.value = false; ready.value = false; beforeContent.value = null; afterContent.value = null }
async function load() {
  clear(); error.value = ''; if (!alive || hidden || !comparable.value) return
  const token = generation, after = new ProjectReader(props.revision.project_id, props.revision.revision_id), before = props.parent ? new ProjectReader(props.parent.project_id, props.parent.revision_id) : null
  readers = before ? [after, before] : [after]; busy.value = true
  try {
    await Promise.all(readers.map(r => r.load()))
    if (!alive || token !== generation) return
    afterContent.value = after.content; beforeContent.value = before?.content || null; ready.value = true
  } catch (cause) { if (alive && token === generation) error.value = readerError(cause) }
  finally { if (alive && token === generation) busy.value = false }
}
watch(() => [props.revision, props.parent], () => void load(), { immediate: true })
const stop = appSDK.lifecycle.subscribe(event => {
  if (event === 'hidden') { hidden = true; clear() }
  if (event === 'active') { hidden = false; void load() }
  if (event === 'dispose') { alive = false; clear() }
})
onBeforeUnmount(() => { alive = false; clear(); stop() })
function fileLink(revision: string, file: string) { return { path: route.path, query: { ...route.query, revision, tab: 'files', fixed: undefined, file } } }
const contentLink = computed(() => ({ path: route.path, query: { ...route.query, revision: props.revision.revision_id, fixed: undefined, tab: undefined } }))
const time = computed(() => props.revision.created_at && Number.isFinite(Date.parse(props.revision.created_at)) ? new Date(props.revision.created_at).toLocaleString('zh-CN') : '时间未知')
const compactDiffRows = computed(() => {
  if (!diff.value) return []
  const rows: { oldLn: string; newLn: string; sign: string; kind: 'add' | 'del' | 'ctx'; text: string }[] = []
  let oldIdx = 1, newIdx = 1
  if (diff.value.name) {
    if (diff.value.name.before) rows.push({ oldLn: String(oldIdx++), newLn: '', sign: '-', kind: 'del', text: `项目名称: ${diff.value.name.before}` })
    rows.push({ oldLn: '', newLn: String(newIdx++), sign: '+', kind: 'add', text: `项目名称: ${diff.value.name.after}` })
  }
  for (const c of diff.value.files) {
    if (c.before) rows.push({ oldLn: String(oldIdx++), newLn: '', sign: '-', kind: 'del', text: `${c.before.path} (${c.before.content_ref.version_id})` })
    if (c.after) rows.push({ oldLn: '', newLn: String(newIdx++), sign: '+', kind: 'add', text: `${c.after.path} (${c.after.content_ref.version_id})` })
  }
  for (const group of relationshipGroups.value) {
    for (const c of group.entries) {
      if (c.before) rows.push({ oldLn: String(oldIdx++), newLn: '', sign: '-', kind: 'del', text: `${group.label}: ${JSON.stringify(c.before)}` })
      if (c.after) rows.push({ oldLn: '', newLn: String(newIdx++), sign: '+', kind: 'add', text: `${group.label}: ${JSON.stringify(c.after)}` })
    }
  }
  if (!rows.length) rows.push({ oldLn: '1', newLn: '1', sign: ' ', kind: 'ctx', text: `${props.revision.manifest.name} (${props.revision.revision_id})` })
  return rows
})
</script>

<template>
  <section class="panel"><h2>{{ revision.manifest.name }}</h2><p class="tabular-nums">{{ time }}</p><dl><dt>固定修订</dt><dd class="tabular-nums">{{ revision.revision_id }}</dd><dt>父修订</dt><dd class="tabular-nums">{{ revision.parent_revision_id || '初始修订' }}</dd></dl><p class="muted">当前接口未登记提交说明与操作者署名。</p><RouterLink class="btn" :to="contentLink">查看此固定修订的项目内容</RouterLink><RouterLink v-if="revision.manifest.entry_document_id" class="btn" :to="fileLink(revision.revision_id, revision.manifest.entry_document_id)">阅读该修订入口文档</RouterLink></section>
  <p v-if="!comparable" class="muted" :role="historyBusy ? 'status' : undefined">{{ historyBusy ? '正在读取真实父修订…' : '父修订暂不可读，未将此修订当作初始版本计算变化。' }}</p>
  <template v-if="diff">
    <section class="panel">
      <h2>文件变化</h2>
      <p v-if="!diff.files.length" class="muted">与父修订相比没有文件变化。</p>
      <div class="diff-viewer" role="table" aria-label="四列紧凑修订对照">
        <div v-for="(row, idx) in compactDiffRows" :key="idx" class="diff-row" role="row">
          <span class="diff-ln tabular-nums" role="cell">{{ row.oldLn }}</span>
          <span class="diff-ln tabular-nums" role="cell">{{ row.newLn }}</span>
          <span class="diff-sign" :class="row.kind" role="cell">{{ row.sign }}</span>
          <div class="diff-cell" :class="row.kind" role="cell">{{ row.text }}</div>
        </div>
      </div>
      <article v-for="change in diff.files" :key="change.key" class="change"><strong>{{ !change.before ? '新增' : !change.after ? '移除引用' : '变更' }} · {{ change.after?.path || change.before?.path }}</strong><p v-if="change.before && change.after && change.before.path !== change.after.path" class="muted">{{ change.before.path }} → {{ change.after.path }}</p><div class="actions"><RouterLink v-if="change.before && parent" class="btn" :to="fileLink(parent.revision_id, change.before.file_id)">查看变更前</RouterLink><RouterLink v-if="change.after" class="btn" :to="fileLink(revision.revision_id, change.after.file_id)">查看{{ change.before ? '变更后' : '该版文件' }}</RouterLink></div></article><p v-if="diff.files.some(c => !c.after)" class="muted">移除项目引用不代表不可变源文件已删除。</p>
    </section>
    <section class="panel"><h2>业务与固定关系变化</h2><p v-if="diff.name">项目名称：{{ diff.name.before || '未建立' }} → {{ diff.name.after }}</p><p v-if="diff.entry">入口文档：{{ diff.entry.before || '未设置' }} → {{ diff.entry.after || '未设置' }}</p><template v-for="group in relationshipGroups" :key="group.label"><article v-for="change in group.entries" :key="change.key" class="change"><strong>{{ group.label }} · {{ !change.before ? '新增' : !change.after ? '移除' : '变更' }}</strong><pre v-if="change.before">变更前：{{ JSON.stringify(change.before, null, 2) }}</pre><pre v-if="change.after">变更后：{{ JSON.stringify(change.after, null, 2) }}</pre></article></template>
      <p v-if="busy" class="muted" role="status">正在读取两版业务内容…</p><div v-else-if="error" role="alert"><p>{{ error }}</p><button class="btn" @click="load">重试业务对照</button></div><template v-else-if="ready"><article v-for="change in business" :key="change.key" class="change"><strong>{{ change.after?.label || change.before?.label }}</strong><pre v-if="change.before">变更前：{{ JSON.stringify(change.before.value, null, 2) }}</pre><pre v-if="change.after">变更后：{{ JSON.stringify(change.after.value, null, 2) }}</pre></article><p v-if="!business.length" class="muted">没有已登记的选用或直接来源关系变化。其他业务文字与媒体变化可从上方文件变化查看固定文档。</p></template>
    </section>
  </template>
</template>

<style scoped>
.panel { padding:20px; border:0.5px solid var(--vf-hairline-outer, var(--line)); border-radius:12px; background:var(--shell-panel); box-shadow:var(--vf-shadow-card); min-width:0; }
h2 { font-size:18px; margin-bottom:12px; }
dl { display:grid; grid-template-columns:80px minmax(0,1fr); gap:12px; margin:16px 0; }
.muted,dt { color:var(--muted); }
.tabular-nums, .diff-ln { font-variant-numeric: tabular-nums; }
.diff-viewer { border:0.5px solid var(--vf-hairline-outer, var(--line)); border-radius:8px; overflow:hidden; background:var(--subtle); margin:12px 0; font:12px/1.65 ui-monospace, "Cascadia Code", "Consolas", monospace; }
.diff-row { display:grid; grid-template-columns:44px 44px 24px minmax(0,1fr); border-bottom:0.5px solid var(--vf-hairline-outer, var(--line)); }
.diff-row:last-child { border-bottom:0; }
.diff-ln { user-select:none; text-align:right; padding:3px 8px; color:var(--muted); border-right:0.5px solid var(--vf-hairline-outer, var(--line)); }
.diff-sign { user-select:none; text-align:center; padding:3px 0; font-weight:600; }
.diff-sign.add { color:var(--accent); }
.diff-sign.del { color:var(--danger); }
.diff-cell { padding:3px 10px; white-space:pre-wrap; overflow-wrap:anywhere; }
.diff-cell.add { background:rgba(8, 126, 96, 0.08); }
.diff-cell.del { background:rgba(201, 87, 67, 0.08); }
.change { border-top:0.5px solid var(--vf-hairline-outer, var(--line)); padding:12px 0; }
.actions { display:flex; gap:8px; margin-top:8px; flex-wrap:wrap; }
pre { white-space:pre-wrap; overflow-wrap:anywhere; font:12px/1.6 monospace; margin-top:8px; color:var(--muted); }
[role=alert] { color:var(--danger); }
</style>
