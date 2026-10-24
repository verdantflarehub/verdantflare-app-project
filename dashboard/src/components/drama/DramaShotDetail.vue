<script setup lang="ts">
import { computed, ref } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import { useHost } from '../../platform/host'
import {
  buildProjectViewerOpenTarget,
  classifyViewerOpenError,
  detectProjectSetPackages,
  type ViewerLaunchOutcome,
} from '../../platform/set-viewer-entry'
import { ProjectReader, projectContentView, type DramaGlobal, type DramaLocationSetup, type DramaUsage } from '../../platform/project-reader'
import DramaProse from './DramaProse.vue'
import DramaDocument from './DramaDocument.vue'
import DramaFile from './DramaFile.vue'
import DramaVersions from './DramaVersions.vue'
import DramaTips from './DramaTips.vue'

const props = defineProps<{
  reader: ProjectReader
  q: (key: string) => string
  link: (change: Record<string, string | undefined>) => RouteLocationRaw
  materialLink: (g: DramaGlobal, version?: string, state?: string) => RouteLocationRaw
  target: string
}>()

const { appSDK } = useHost()
const project = computed(() => props.reader.project!)
const drama = computed(() => projectContentView(project.value, props.reader.content)!.ai_short_drama!)
const episode = computed(() => props.q('episode') ? drama.value.episodes.find(e => e.key === props.q('episode')) : drama.value.episodes[0])
const scene = computed(() => episode.value?.scenes.find(s => s.key === props.q('scene')))
const shot = computed(() => scene.value?.shots.find(s => s.key === props.q('shot')))
const usages = computed(() => drama.value.usages.filter(u => shot.value ? u.target.episode_key === episode.value?.key && (!u.target.scene_key || u.target.scene_key === scene.value?.key) && (!u.target.shot_key || u.target.shot_key === shot.value.key) : false))
const availableSetPackages = computed(() => detectProjectSetPackages(project.value?.manifest.files))
const viewerOutcome = ref<ViewerLaunchOutcome | 'idle'>('idle')
const viewerBusy = ref(false)

function setupOf(u: DramaUsage): DramaLocationSetup | undefined {
  if (!u.setup_key) return undefined
  const g = drama.value.globals.find(gl => gl.key === u.global_key)
  return g?.setups?.find(s => s.key === u.setup_key)
}

async function openShotSetupInViewer(u: DramaUsage) {
  if (viewerBusy.value) return
  const setup = setupOf(u)
  const setId = setup?.set_package_id || availableSetPackages.value[0]?.setId
  const cameraId = u.camera_key || setup?.primary_camera_key || setup?.camera_keys[0] || 'C1'
  viewerBusy.value = true
  viewerOutcome.value = 'idle'
  try {
    const target = buildProjectViewerOpenTarget({
      projectId: project.value.project_id,
      revisionId: project.value.revision_id,
      setId,
      cameraId,
    })
    await appSDK.capabilities.call('navigation.openApp', target)
    viewerOutcome.value = 'opened'
  } catch (cause) {
    viewerOutcome.value = classifyViewerOpenError(cause)
  } finally {
    viewerBusy.value = false
  }
}

function usageMaterial(u: DramaUsage) {
  const g = drama.value.globals.find(gl => gl.key === u.global_key)!
  const setup = setupOf(u)
  return [
    g.name,
    g.states.find(s => s.key === u.state_key)?.name || '状态未整理',
    setup ? setup.label : (g.versions.find(v => v.key === u.version_key)?.label || '制作版本未选定'),
  ].join(' · ')
}
</script>

<template>
  <template v-if="q('shot') && q('version')">
    <template v-if="shot && shot.versions.some(v => v.key === q('version'))">
      <h2>{{ shot.title }} · 镜头制作版本</h2>
      <DramaVersions fixed :reader="reader" :versions="shot.versions" :selections="shot.selected_versions" />
      <section class="panel">
        <h2>固定制作依据</h2>
        <p class="muted">制作依据、修改记录和审核来自本版本。技术检查未登记时不推定通过。</p>
        <DramaDocument v-for="(r, i) in shot.definition_refs" :key="r.file_id + ':' + i" :reader="reader" :file-id="r.file_id" :anchor="r.anchor" :title="'镜头依据 ' + (i + 1)" />
        <p v-if="!shot.definition_refs.length" class="muted">未登记额外镜头依据。</p>
      </section>
    </template>
    <p v-else class="empty">当前修订没有此镜头制作版本。</p>
  </template>

  <template v-else-if="q('shot')">
    <p v-if="!shot" class="empty">当前修订没有这个镜头。</p>
    <div v-else class="production-cols">
      <div class="stack">
        <section v-if="!shot.versions.length" class="panel">
          <h2>制作准备</h2>
          <p class="empty">尚无制作候选，先核对素材选版、场景白模与灯光依据。</p>
          <div class="row">
            <h3>场景与镜头白模</h3>
            <p class="muted">本镜尚未登记可编辑工程与对应相机预览。</p>
            <RouterLink class="related-link" :to="link({ view: 'spaces', shot: undefined, scene: undefined })">查看场景白模与灯光 →</RouterLink>
          </div>
          <div class="row">
            <h3>环境与灯光</h3>
            <p class="muted">本镜段落固定相应状态；具体工程灯值与曝光以实际试光记录为准。</p>
          </div>
        </section>
        <DramaVersions v-else :reader="reader" :versions="shot.versions" :selections="shot.selected_versions" />

        <section class="panel">
          <h2>镜头说明与依据</h2>
          <DramaProse :text="shot.overview || '镜头说明尚未填写。'" :drama="drama" />
          <p class="muted">{{ shot.duration_seconds == null ? '剧本时长未填写' : `剧本预算 ${shot.duration_seconds} 秒` }}</p>
          <DramaDocument v-for="(r, i) in shot.definition_refs" :key="`${r.file_id}:${i}`" :reader="reader" :file-id="r.file_id" :anchor="r.anchor" :title="`剧本与连续性依据 ${i + 1}`" />
          <div v-for="s in [...shot.segments].sort((a, b) => a.order - b.order)" :key="s.key" class="row">{{ s.order }} · {{ s.label }}</div>
        </section>

        <section v-if="!shot.versions.length && shot.result_file_ids.length" class="panel">
          <h2>已有结果 · 版本关系未整理</h2>
          <div class="thumb-grid">
            <DramaFile v-for="(id, i) in shot.result_file_ids" :key="id" thumbnail :reader="reader" :file-id="id" :label="`镜头结果 ${i + 1}`" />
          </div>
        </section>
      </div>

      <aside class="stack">
        <section class="panel">
          <h2>固定素材与场景状态</h2>
          <p v-if="!usages.length" class="muted">素材、白模与灯光输入尚未关联。</p>
          <div v-for="u in usages" :key="u.key" class="row">
            <RouterLink
              v-if="u.setup_key"
              :to="link({ tab: 'location', kind: 'location', material: u.global_key, state: u.state_key || setupOf(u)?.state_key, setup: u.setup_key, episode: undefined, scene: undefined, shot: undefined })"
            >
              {{ usageMaterial(u) }} →
            </RouterLink>
            <RouterLink
              v-else
              :to="materialLink(drama.globals.find(g => g.key === u.global_key)!, u.version_key || undefined, u.state_key || undefined)"
            >
              {{ usageMaterial(u) }} →
            </RouterLink>
            <p class="muted">
              {{ u.segment?.label || '场次／整镜使用' }}
              <template v-if="u.setup_key"> · 机位 <code>{{ u.camera_key || setupOf(u)?.primary_camera_key || 'C1' }}</code></template>
            </p>
            <button
              v-if="u.setup_key || u.camera_key"
              type="button"
              class="shot-viewer-link"
              :disabled="viewerBusy"
              data-testid="shot-open-viewer"
              @click="openShotSetupInViewer(u)"
            >
              在查看器中核对本镜机位 →
            </button>
          </div>
        </section>
        <section v-if="shot.continuity_keys.length" class="panel">
          <h2>连续性核对</h2>
          <div v-for="c in drama.continuity.filter(c => shot?.continuity_keys.includes(c.key))" :key="c.key" class="row">
            <h3>{{ c.name }}</h3>
            <DramaProse :text="c.description" :drama="drama" />
            <DramaDocument v-for="(r, i) in c.definition_refs" :key="`${r.file_id}:${i}`" :reader="reader" :file-id="r.file_id" :anchor="r.anchor" :title="`连续性资料 ${i + 1}`" />
          </div>
        </section>
        <DramaTips :project-id="project.project_id" :revision-id="project.revision_id" :target="target" />
      </aside>
    </div>
  </template>
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
.empty { color: var(--muted); padding: 0; border: 0; font-size: 13px; line-height: 1.8; }
.production-cols { display: grid; grid-template-columns: minmax(0, 1fr) 260px; gap: 20px; align-items: start; }
.production-cols > *, .stack { min-width: 0; }
.stack { display: grid; gap: 18px; }
.production-cols > aside { position: sticky; top: 16px; align-self: start; }
.related-link { display: block; color: var(--accent); font-size: 13px; line-height: 1.7; padding: 6px 0; }
.shot-viewer-link { margin-top: 6px; padding: 0; border: 0; background: transparent; color: var(--accent); font: inherit; font-size: 12px; cursor: pointer; text-align: left; }
.shot-viewer-link:hover { text-decoration: underline; }
.row { border-top: 1px solid var(--line); margin-top: 14px; padding-top: 14px; }
.row > a { font-size: 13px; line-height: 1.75; color: var(--accent); }
@media (max-width: 1200px) {
  .production-cols { grid-template-columns: minmax(0, 1fr); }
  .production-cols > aside { grid-template-columns: repeat(2, minmax(0, 1fr)); position: static; }
}
@media (max-width: 800px) {
  .production-cols > aside { grid-template-columns: minmax(0, 1fr); }
  .panel { padding: 16px; }
}
</style>
