<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ProjectReader, projectContentView, type Cast } from '../../platform/project-reader'
import ProjectMedia from '../ProjectMedia.vue'
import ProjectPrompt from '../ProjectPrompt.vue'
import ProjectEditDialog from '../ProjectEditDialog.vue'
import { useRouter } from 'vue-router'

const props = defineProps<{ reader: ProjectReader; tab: string }>()
const route = useRoute(), router = useRouter()
const editing = ref<'cast' | 'remove-cast'>()
const canEdit = computed(() => project.value.can_write === true && project.value.revision_id === project.value.head_revision_id)
function saved(revision: string) { editing.value = undefined; void router.replace({ path: route.path, query: { ...route.query, revision, item: undefined } }) }
const project = computed(() => props.reader.project!)
const content = computed(() => projectContentView(project.value, props.reader.content)!)
const video = computed(() => content.value.video!)
const item = computed(() => typeof route.query.item === 'string' ? route.query.item : '')
const castDetail = computed(() => video.value.cast.find(entry => entry.key === item.value))
const board = computed(() => video.value.storyboards.find(entry => entry.key === item.value))
const roles = [{ key: 'lead', label: '主角' }, { key: 'support', label: '配角' }] as const
const duration = (seconds?: number | null) => seconds == null ? '时长未填写' : `${seconds} 秒`
const value = (text?: string | null) => text?.trim() ? text : '未填写'
function boardLink(key: string) {
  return { path: route.path, query: { ...route.query, tab: 'storyboards', item: key } }
}
function castLink(cast: Cast) {
  return { path: route.path, query: {
    ...route.query, category: 'character', project: cast.project_id!, revision: cast.revision_id!, tab: 'setting', item: undefined,
    from: project.value.project_id, fromRevision: project.value.revision_id, fromName: project.value.manifest.name, fromItem: cast.key,
  } }
}
</script>

<template>
  <div class="short-video-page"><ProjectEditDialog v-if="editing" :reader="reader" :mode="editing" :item-key="item" @close="editing = undefined" @saved="saved" />
    <template v-if="item && tab === 'cast'">
      <p v-if="!castDetail" class="empty">当前项目修订中没有此片中角色。</p>
      <section v-else class="panel"><h2>{{ castDetail.name }}</h2><p class="cast-role">本片角色：{{ castDetail.role === 'lead' ? '主角' : '配角' }}</p><ProjectMedia :reader="reader" :file-id="castDetail.portrait_file_id" kind="image" :label="castDetail.name" /><RouterLink v-if="castDetail.project_id" class="btn" :to="castLink(castDetail)">打开来源角色固定版本</RouterLink><p v-else class="muted">尚未关联来源角色项目。</p><button v-if="canEdit" class="btn" @click="editing = 'remove-cast'">移出本片</button></section>
    </template>
    <template v-else-if="item">
      <p v-if="!board" class="empty">当前项目修订中没有这项故事板。</p>
      <article v-else class="short-video-detail">
        <section class="panel">
          <h2>概述</h2><p class="overview">{{ value(board.overview) }}</p>
          <dl class="fields"><dt>时长</dt><dd>{{ duration(board.duration_seconds) }}</dd><template v-for="(field, i) in board.fields" :key="i"><dt>{{ field.label }}</dt><dd>{{ value(field.value) }}</dd></template></dl>
        </section>
        <section class="panel">
          <h2>参考图</h2>
          <div v-if="board.reference_file_ids.length" class="short-video-references" tabindex="0" role="region" aria-label="参考图，可横向滚动">
            <ProjectMedia v-for="(id, i) in board.reference_file_ids" :key="`${id}:${i}`" :reader="reader" :file-id="id" kind="image" :label="`参考图 ${i + 1}`" />
          </div>
          <p v-else class="muted">未添加图片</p>
        </section>
        <section class="panel prompt-panel"><ProjectPrompt :key="board.key" :text="board.prompt" /></section>
        <section class="panel">
          <h2>结果视频</h2>
          <div class="outputs"><ProjectMedia v-for="(id, i) in board.result_file_ids" :key="`${id}:${i}`" :reader="reader" :file-id="id" kind="video" :label="`${board.title} ${i + 1}`" /><ProjectMedia v-if="!board.result_file_ids.length" :reader="reader" kind="video" :label="board.title" /></div>
        </section>
        <section v-if="board.file_ids.length" class="panel"><h2>文件</h2><ProjectMedia v-for="id in board.file_ids" :key="id" :reader="reader" :file-id="id" kind="file" :label="reader.file(id).path" /></section>
      </article>
    </template>
    <template v-else-if="tab === 'cast'">
      <header class="section-heading"><h2>角色创作</h2><button v-if="canEdit" class="btn" @click="editing = 'cast'">引用角色</button><span class="muted">{{ video.cast.length }} 位角色</span></header>
      <section v-for="role in roles" :key="role.key" class="panel">
        <h2>{{ role.label }}</h2>
        <div v-if="video.cast.some(cast => cast.role === role.key)" class="short-video-cast-grid">
          <article v-for="cast in video.cast.filter(cast => cast.role === role.key)" :key="cast.key" class="short-video-cast-card">
            <RouterLink :to="{ path: route.path, query: { ...route.query, tab: 'cast', item: cast.key } }" class="short-video-cast-link">
              <ProjectMedia :reader="reader" :file-id="cast.portrait_file_id" kind="image" :label="cast.name" thumbnail /><strong>{{ cast.name }}</strong>
            </RouterLink>
            <p class="cast-role">本片角色：{{ role.label }}</p>
          </article>
        </div>
        <p v-else class="muted">未添加{{ role.label }}</p>
      </section>
    </template>
    <template v-else>
      <section class="panel video-preview">
        <h2>成片预览</h2><p class="overview">{{ value(content.overview) }}</p>
        <ProjectMedia :reader="reader" :file-id="video.output_file_id" kind="video" :label="project.manifest.name" />
      </section>
      <section class="short-video-boards">
        <header class="section-heading"><h2>故事板</h2><span class="muted">{{ video.storyboards.length }} 个故事板 · {{ duration(video.duration_seconds) }}<template v-if="video.aspect"> · {{ video.aspect }}</template></span></header>
        <ol v-if="video.storyboards.length" class="short-video-board-grid">
          <li v-for="(entry, index) in video.storyboards" :key="entry.key">
            <RouterLink class="short-video-board" :to="boardLink(entry.key)">
              <ProjectMedia :reader="reader" :file-id="entry.image_file_id" kind="image" :label="entry.title" thumbnail />
              <div class="short-video-board-caption"><small>{{ String(index + 1).padStart(2, '0') }}</small><h3>{{ entry.title }}</h3><small>{{ duration(entry.duration_seconds) }}</small></div>
            </RouterLink>
          </li>
        </ol>
        <p v-else class="empty">未添加故事板</p>
      </section>
    </template>
  </div>
</template>

<style scoped>
.short-video-page,.short-video-detail,.outputs { display:grid; gap:16px; min-width:0; }
.panel { margin:0; padding:20px; border:1px solid var(--line); border-radius:12px; background:var(--shell-panel); min-width:0; }
h2 { font-size:18px; font-weight:600; line-height:1.6; margin:0 0 16px; }h3 { font-size:17px; font-weight:600; line-height:1.6; }
.overview { white-space:pre-wrap; line-height:1.8; margin-bottom:16px; }.muted,small,dt,.cast-role { color:var(--muted); }
.section-heading { display:flex; align-items:baseline; justify-content:space-between; gap:16px; flex-wrap:wrap; }.section-heading h2 { margin:0; }.section-heading .muted { font-size:12px; }
.short-video-board-grid { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); list-style:none; gap:20px; padding:0; margin:20px 0 0; }
.short-video-board-grid>li { min-width:0; }.short-video-board { display:block; border:1px solid var(--line); border-radius:12px; background:var(--shell-panel); overflow:hidden; }
.short-video-board:hover,.short-video-cast-card:has(a:hover) { border-color:var(--accent); }
.short-video-board :deep(.thumbnail) { height:300px; aspect-ratio:auto; border-radius:0; }.short-video-board :deep(.thumbnail img) { object-position:left top; }
.short-video-board-caption { display:grid; grid-template-columns:24px minmax(0,1fr); gap:8px 10px; padding:18px; align-items:baseline; }.short-video-board-caption small:last-child { grid-column:2; }
.short-video-references { display:flex; flex-wrap:nowrap; gap:18px; overflow-x:auto; padding-bottom:12px; min-width:0; }.short-video-references>:deep(.project-media) { flex:0 0 auto; max-width:100%; }.short-video-references :deep(.image-open img) { width:auto; height:clamp(180px,24vw,320px); max-width:100%; object-fit:contain; object-position:left top; }
.prompt-panel :deep(.project-prompt) { margin:0; border:0; padding:0; }.prompt-panel :deep(summary) { font-size:15px; }
.fields { display:grid; grid-template-columns:100px minmax(0,1fr); gap:10px 16px; }.fields dd { white-space:pre-wrap; }
.short-video-cast-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:20px; }.short-video-cast-card { border:1px solid var(--line); border-radius:10px; padding:18px; min-width:0; }
.short-video-cast-link { display:grid; grid-template-columns:100px minmax(0,1fr); align-items:start; gap:20px; min-width:0; }.short-video-cast-link>:deep(.project-media) { width:100px; height:136px; aspect-ratio:auto; border-radius:8px; }.short-video-cast-link strong { font-size:20px; }.cast-role { margin-top:16px; font-size:13px; }
.short-video-cast-link :deep(.media-state) { min-height:0; height:100%; padding:8px; font-size:12px; }
.empty { padding:24px; border:1px dashed var(--line); border-radius:8px; color:var(--muted); }
.video-preview :deep(video),.outputs :deep(video) { width:auto; max-width:100%; max-height:640px; }.video-preview :deep(.project-media),.outputs :deep(.project-media) { background:var(--subtle); border-radius:8px; }
.video-preview :deep(.media-state),.outputs :deep(.media-state) { min-height:110px; padding:24px; }
@media(max-width:1100px) { .short-video-board-grid { grid-template-columns:repeat(2,minmax(0,1fr)); }.short-video-cast-grid { grid-template-columns:minmax(0,1fr); } }
@media(max-width:650px) { .short-video-board-grid { grid-template-columns:minmax(0,1fr); }.panel { padding:18px; }.fields { grid-template-columns:70px minmax(0,1fr); }.short-video-cast-card { padding:12px; }.short-video-cast-link { grid-template-columns:80px minmax(0,1fr); gap:12px; }.short-video-cast-link>:deep(.project-media) { width:80px; height:110px; } }
</style>
