<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import type { ProjectReader, DramaWorldCandidate } from '../platform/project-reader'
import DramaPublication from './drama/DramaPublication.vue'
import DramaDocument from './drama/DramaDocument.vue'
import DramaFile from './drama/DramaFile.vue'
const props = defineProps<{ reader: ProjectReader; candidate: DramaWorldCandidate }>()
const route = useRoute()
const project = computed(() => props.reader.project!)
const sourceProject = computed(() => props.candidate.registration_request?.source_project_id || project.value.project_id)
const sourceRevision = computed(() => props.candidate.registration_request?.source_revision_id || project.value.revision_id)
const localSource = computed(() => sourceProject.value === project.value.project_id && sourceRevision.value === project.value.revision_id)
const owner = computed(() => {
  if (!localSource.value) return
  const drama = props.reader.content?.ai_short_drama, source = props.candidate.source
  return source.kind === 'global' ? drama?.globals.find(g => g.key === source.key) : drama?.deliverables.find(d => d.key === source.key)
})
const sourceName = computed(() => owner.value && ('name' in owner.value ? owner.value.name : owner.value.title))
const sourceLink = computed(() => ({ path: '/projects', query: { category: 'ai_short_drama', project: sourceProject.value, revision: sourceRevision.value, ...(owner.value ? props.candidate.source.kind === 'global' ? { tab: 'kind' in owner.value ? owner.value.kind : undefined, material: props.candidate.source.key } : { tab: 'delivery', deliverable: props.candidate.source.key } : {}) } }))
const files = computed(() => {
  if (!localSource.value) return []
  const ids = props.candidate.registration_request?.file_ids
  return ids ? project.value.manifest.files.filter(f => ids.includes(f.file_id)) : []
})
const state = computed(() => {
  const status = props.candidate.registration_status
  return !status ? '尚未入库' : status.state === 'committed' ? '已有固定入库结果' : status.state === 'preparing' ? '原入库请求结果待确认' : status.state === 'conflict' ? '入库版本发生冲突' : '原入库请求失败'
})
const review = computed(() => ({ unreviewed: '未审核', in_review: '审核中', approved: '已有审核记录', rejected: '需修改' })[props.candidate.review.state])
</script>

<template>
  <div class="candidate-detail">
    <section class="panel"><h2>{{ candidate.name }}</h2><p>{{ candidate.description || '尚未登记候选说明。' }}</p><dl><dt>入库状态</dt><dd>{{ state }}</dd><dt>创作审核</dt><dd>{{ review }}</dd><dt>来源项目</dt><dd>{{ sourceProject }}</dd><dt>固定修订</dt><dd>{{ sourceRevision }}</dd><dt>业务对象</dt><dd>{{ sourceName || candidate.source.key }}</dd></dl><RouterLink class="btn" :to="sourceLink">查看来源固定成果</RouterLink><p v-if="candidate.registration_status?.state === 'failed' || candidate.registration_status?.state === 'conflict'" class="muted">原请求结果：{{ candidate.registration_status.error_code }}。候选与原请求保留。</p></section>
    <section class="panel"><h2>可复用内容与输入依据</h2><p v-if="!localSource" class="muted">候选使用另一固定修订；请打开来源查看预览和文件，不以当前修订内容替代。</p><template v-else><p v-if="!owner" class="muted">当前修订没有该来源业务对象。</p><p v-if="!candidate.registration_request" class="muted">尚未固定本次入库文件范围；下列来源制作版本不表示已选用或已入库。</p><div v-if="files.length" class="outputs"><DramaFile v-for="file in files" :key="file.file_id" :reader="reader" :file-id="file.file_id" :label="file.path" /></div><template v-if="owner && !candidate.registration_request"><article v-for="version in owner.versions" :key="version.key"><h3>{{ version.label }}</h3><p class="muted">制作依据：{{ version.basis_revision_id }} · 父版本：{{ version.parent_version_key || '未登记' }}</p><p>{{ version.change_note }}</p><div class="outputs"><DramaFile v-for="file in version.file_refs" :key="file.file_id" :reader="reader" :file-id="file.file_id" :label="file.purpose" /></div><p v-if="!version.file_refs.length" class="muted">此版尚未登记成果文件。</p></article><p v-if="!owner.versions.length" class="muted">尚未登记制作版本。</p></template><DramaDocument v-if="candidate.publication_record_file_id" :reader="reader" :file-id="candidate.publication_record_file_id" title="入库范围与适配说明" /><p v-else class="muted">尚未登记入库说明、适配限制与许可声明，请先核对。</p><DramaDocument v-if="candidate.review.record_file_id" :reader="reader" :file-id="candidate.review.record_file_id" title="候选审核记录" /></template><p class="muted">许可与可见范围在数字世界确认页核对；本页没有记录时不推定可公开使用。</p></section>
    <DramaPublication :reader="reader" :candidates="[candidate]" />
    <RouterLink class="btn defer" :to="{ path: route.path, query: { ...route.query, candidate: undefined } }">暂不存储，返回资产引用</RouterLink>
  </div>
</template>

<style scoped>
.candidate-detail,.outputs { display:grid; gap:18px; min-width:0; }.panel { padding:24px; border:1px solid var(--line); border-radius:12px; background:var(--shell-panel); overflow-wrap:anywhere; min-width:0; }h2 { font-size:18px; margin-bottom:12px; }h3 { font-size:16px; margin:20px 0 12px; }p { line-height:1.8; }.muted,dt { color:var(--muted); }dl { display:grid; grid-template-columns:90px minmax(0,1fr); gap:12px; margin:20px 0; }.outputs { margin:16px 0; }.defer { justify-self:start; }
</style>
