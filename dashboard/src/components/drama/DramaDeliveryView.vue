<script setup lang="ts">
import { computed } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import { ProjectReader, projectContentView } from '../../platform/project-reader'
import DramaDocument from './DramaDocument.vue'
import DramaFile from './DramaFile.vue'
import DramaVersions from './DramaVersions.vue'
import DramaTips from './DramaTips.vue'
import DramaPublication from './DramaPublication.vue'

const props = defineProps<{
  reader: ProjectReader
  q: (key: string) => string
  link: (change: Record<string, string | undefined>) => RouteLocationRaw
  target: string
}>()

const project = computed(() => props.reader.project!)
const drama = computed(() => projectContentView(project.value, props.reader.content)!.ai_short_drama!)
const delivery = computed(() => drama.value.deliverables.find(d => d.key === props.q('deliverable')))
const candidates = computed(() => drama.value.world_candidates.filter(c => delivery.value ? c.source.kind === 'deliverable' && c.source.key === delivery.value.key : true))
</script>

<template>
  <template v-if="q('deliverable') && q('version') && !q('package')">
    <template v-if="delivery && delivery.versions.some(v => v.key === q('version'))">
      <h2>{{ delivery.title }} · 成果版本</h2>
      <DramaVersions fixed :reader="reader" :versions="delivery.versions" :selected-key="delivery.selected_version_key" />
      <section class="panel">
        <h2>实际交付回执</h2>
        <p class="muted">下列记录属于此成果项；仅有回执明确指向本版本时，才表示该版本已经交付。</p>
        <DramaDocument v-for="(id, i) in delivery.delivery_record_file_ids" :key="id" :reader="reader" :file-id="id" :title="'交付记录 ' + (i + 1)" />
        <p v-if="!delivery.delivery_record_file_ids.length" class="empty">尚未登记实际交付回执。</p>
      </section>
    </template>
    <p v-else class="empty">当前修订没有此成果版本。</p>
  </template>

  <template v-else-if="q('package')">
    <section v-if="delivery && q('package') === delivery.key" class="panel">
      <h2>{{ delivery.title }} · 交付包</h2>
      <p class="muted">固定成果版本与实际交付回执分别核对。</p>
      <DramaVersions :reader="reader" :versions="delivery.versions" :selected-key="delivery.selected_version_key" />
      <h3>实际交付记录</h3>
      <DramaDocument v-for="(id, i) in delivery.delivery_record_file_ids" :key="id" :reader="reader" :file-id="id" :title="'交付记录 ' + (i + 1)" />
      <p v-if="!delivery.delivery_record_file_ids.length" class="empty">尚未登记实际交付回执。</p>
    </section>
    <p v-else class="empty">当前修订没有此成果或交付包。</p>
  </template>

  <template v-else-if="q('deliverable')">
    <template v-if="delivery">
      <div class="section-head">
        <RouterLink class="btn" :to="link({ package: delivery.key })">查看交付包与回执</RouterLink>
      </div>
      <p class="prose">{{ delivery.description }}</p>
      <DramaVersions :reader="reader" :versions="delivery.versions" :selected-key="delivery.selected_version_key" />
      <section class="panel">
        <h2>实际交付记录</h2>
        <p v-if="!delivery.delivery_record_file_ids.length" class="muted">尚未登记实际交付。导出或下载不代表客户已收到。</p>
        <DramaDocument v-for="(id, i) in delivery.delivery_record_file_ids" :key="id" :reader="reader" :file-id="id" :title="`交付记录 ${i + 1}`" />
      </section>
      <section v-if="!delivery.versions.length" class="panel">
        <h2>已有文件</h2>
        <div class="thumb-grid">
          <DramaFile v-for="(id, i) in delivery.file_ids" :key="id" thumbnail :reader="reader" :file-id="id" :label="`成果文件 ${i + 1}`" />
        </div>
      </section>
      <DramaTips :project-id="project.project_id" :revision-id="project.revision_id" :target="target" />
    </template>
    <p v-else class="empty">当前修订没有这项成果。</p>
  </template>

  <template v-else>
    <h2>成果与交付</h2>
    <p class="muted">成片、工程与交付包各自固定版本；新候选不会改变历史成片。</p>
    <div class="material-grid">
      <RouterLink v-for="d in drama.deliverables" :key="d.key" class="production-card" :to="link({ deliverable: d.key, version: undefined, compare: undefined })">
        <h3>{{ d.title }}</h3>
        <p class="prose">{{ d.description }}</p>
        <p class="muted">{{ d.versions.find(v => v.key === d.selected_version_key)?.label || '当前选用未整理' }} · {{ d.delivery_record_file_ids.length ? '有交付记录' : '尚无交付记录' }}</p>
      </RouterLink>
    </div>
    <p v-if="!drama.deliverables.length" class="empty">尚未登记成果项。</p>
    <section v-for="e in drama.episodes.filter(e => e.output_file_id || e.edit_file_id)" :key="e.key" class="panel">
      <h3>{{ e.title }} · 已有分集文件</h3>
      <DramaFile v-if="e.output_file_id" :reader="reader" :file-id="e.output_file_id" label="成片" />
      <DramaFile v-if="e.edit_file_id" :reader="reader" :file-id="e.edit_file_id" label="编辑工程" />
    </section>
  </template>

  <DramaPublication v-if="!(q('deliverable') && q('version') && !q('package'))" :reader="reader" :candidates="candidates" />
</template>

<style scoped>
.thumb-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(340px, 1fr)); gap: 16px; align-items: start; }
@media (max-width: 720px) { .thumb-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
.thumb-grid :deep(.drama-file) { min-width: 0; min-height: 0; }
.panel { background: var(--shell-panel); border: 1px solid var(--line); border-radius: 8px; padding: 20px; min-width: 0; }
h2 { font-size: 18px; line-height: 1.35; margin: 0 0 10px; }
.panel h2 { font-size: 15px; }
h3 { font-size: 15px; margin: 8px 0; }
.muted { color: var(--muted); font-size: 12px; line-height: 1.85; }
.prose { white-space: pre-wrap; font-size: 14px; line-height: 1.85; }
.empty { color: var(--muted); padding: 0; border: 0; font-size: 13px; line-height: 1.8; }
.btn { justify-self: start; font-size: 14px; border-radius: 8px; padding: 8px 12px; min-height: 35px; }
.material-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 16px; align-items: stretch; margin: 16px 0; }
.production-card { display: block; min-width: 0; border: 1px solid var(--line); border-radius: 8px; background: var(--shell-panel); padding: 20px; }
.production-card:hover { border-color: var(--accent); }
.production-card h3 { font-size: 15px; margin: 10px 0; }
.production-card p { font-size: 13px; color: var(--muted); line-height: 1.85; margin: 14px 0; }
@media (max-width: 800px) {
  .material-grid { grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px; }
  .panel { padding: 16px; }
}
</style>
