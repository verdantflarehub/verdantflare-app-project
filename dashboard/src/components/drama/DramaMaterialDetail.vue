<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter, type RouteLocationRaw } from 'vue-router'
import { useHost } from '../../platform/host'
import { reviewPath } from '../../platform/set-review'
import { rememberReviewContext, reviewContextKey } from '../../platform/set-review-memory'
import {
  buildProjectViewerOpenTarget,
  classifyViewerOpenError,
  detectProjectSetPackages,
  type ViewerLaunchOutcome,
} from '../../platform/set-viewer-entry'
import { ProjectReader, projectContentView, type DramaGlobal, type DramaUsage } from '../../platform/project-reader'
import { materialPreview, dramaText } from '../../platform/drama-presentation'
import { projectSources } from '../../platform/project-source'
import ProjectMedia, { type MediaGallery } from '../ProjectMedia.vue'
import DramaProse from './DramaProse.vue'
import DramaDocument from './DramaDocument.vue'
import DramaFile from './DramaFile.vue'
import DramaVersions from './DramaVersions.vue'
import DramaTips from './DramaTips.vue'
import DramaPublication from './DramaPublication.vue'
import ProjectEditDialog from '../ProjectEditDialog.vue'

const props = defineProps<{
  reader: ProjectReader
  tab: string
  q: (key: string) => string
  link: (change: Record<string, string | undefined>) => RouteLocationRaw
  materialLink: (g: DramaGlobal, version?: string, state?: string) => RouteLocationRaw
  target: string
}>()

const router = useRouter(), route = useRoute()
const { appSDK, dashboardHost } = useHost()
const setViewerState = ref<ViewerLaunchOutcome | 'idle'>('idle')
const setViewerBusy = ref(false)
const setPackageDownloadUrl = ref('')
const availableSetPackages = computed(() => detectProjectSetPackages(project.value?.manifest.files))

async function openProjectSetViewer(setId?: string) {
  const pkg = (setId ? availableSetPackages.value.find(p => p.setId === setId) : availableSetPackages.value[0]) || availableSetPackages.value[0]
  if (!pkg || setViewerBusy.value) return
  setViewerBusy.value = true
  setViewerState.value = 'idle'
  try {
    const target = buildProjectViewerOpenTarget({
      projectId: project.value.project_id,
      revisionId: project.value.revision_id,
      setId: pkg.setId,
      cameraId: 'C1',
    })
    await appSDK.capabilities.call('navigation.openApp', target)
    setViewerState.value = 'opened'
  } catch (cause) {
    setViewerState.value = classifyViewerOpenError(cause)
  } finally {
    setViewerBusy.value = false
  }
}

async function prepareSetPackageDownload() {
  const pkg = availableSetPackages.value[0]
  if (!pkg) return
  try {
    const dl = await props.reader.download(pkg.sceneGlbFileId)
    setPackageDownloadUrl.value = dl.content_path
  } catch {
    // Handled by reader
  }
}

async function openSetViewerAppCenter() {
  try {
    await appSDK.navigation.request({ target: 'appcenter', path: '/?app=viewer' })
  } catch {
    // Ignore
  }
}

function openSetReview() {
  if (!material.value || material.value.kind !== 'location') return
  const version = props.q('version') ? material.value.versions.find(v => v.key === props.q('version')) : [...material.value.versions].reverse().find(v => !state.value || v.state_key === state.value.key)
  rememberReviewContext(reviewContextKey(appSDK.context.get(), project.value.project_id), route.fullPath, dashboardHost.getScroll(), undefined, undefined, project.value.manifest.name, material.value.name)
  void router.push(reviewPath({ kind: 'project', project_id: project.value.project_id, content_revision_id: project.value.revision_id, material_key: material.value.key, state_key: version?.state_key || '', version_key: version?.key || '' }))
}
const project = computed(() => props.reader.project!)
const drama = computed(() => projectContentView(project.value, props.reader.content)!.ai_short_drama!)
const material = computed(() => drama.value.globals.find(g => g.key === props.q('material')))
const editingMusicSource = ref(false)
const musicSource = computed(() => projectSources(props.reader.content).find(s => s.category === 'music' && s.item === material.value?.key))
const canEditMusic = computed(() => project.value.can_write === true && project.value.revision_id === project.value.head_revision_id)
watch(() => [props.reader, material.value?.key], () => { editingMusicSource.value = false })
function musicSaved(revision: string) { editingMusicSource.value = false; void router.replace(props.link({ revision })) }

const state = computed(() => {
  const owner = material.value
  if (!owner) return undefined
  if (props.q('lighting')) return owner.states.find(s => s.key === props.q('lighting'))
  if (props.q('state')) return owner.states.find(s => s.key === props.q('state'))
  const latest = [...owner.versions].reverse().find(v => !props.q('state') || v.state_key === props.q('state'))
  const version = props.q('version') ? owner.versions.find(v => v.key === props.q('version')) : latest
  return owner.states.find(s => s.key === (props.q('state') || version?.state_key)) || owner.states[0]
})

const lightingVersions = computed(() => material.value?.versions.filter(v => v.state_key === state.value?.key && (!props.q('version') || v.key === props.q('version'))) || [])
const human = (text?: string | null) => dramaText(text, drama.value).replace(/\*\*/g, '').replace(/^\s*\||\|\s*$/g, '').replace(/\s*\|\s*/g, ' · ').trim()
const related = computed(() => drama.value.globals.filter(g => material.value && (material.value.input_global_keys.includes(g.key) || g.input_global_keys.includes(material.value.key))))
const relatedLooks = computed(() => material.value?.kind === 'character' ? related.value.filter(g => g.kind === 'wardrobe') : related.value.filter(g => g.kind === material.value?.kind && g.input_global_keys.includes(material.value!.key)))
const candidates = computed(() => drama.value.world_candidates.filter(c => material.value ? c.source.kind === 'global' && c.source.key === material.value.key : true))
const usages = computed(() => drama.value.usages.filter(u => material.value ? u.global_key === material.value.key : false))

function usageLabel(u: DramaUsage) {
  const e = drama.value.episodes.find(ep => ep.key === u.target.episode_key)
  const s = e?.scenes.find(sc => sc.key === u.target.scene_key)
  const sh = s?.shots.find(shot => shot.key === u.target.shot_key)
  return [e?.title, s?.title, sh?.title, u.segment?.label].filter(Boolean).join(' · ')
}

function usageMaterial(u: DramaUsage) {
  const g = drama.value.globals.find(gl => gl.key === u.global_key)!
  return [g.name, g.states.find(s => s.key === u.state_key)?.name || '状态未整理', g.versions.find(v => v.key === u.version_key)?.label || '制作版本未选定'].join(' · ')
}

function fileGallery(groupId: string, fileIds: string[], index: number, prefix: string): MediaGallery {
  return {
    id: groupId,
    items: fileIds.map((fileId, i) => ({ fileId, label: `${prefix} ${i + 1}` })),
    index,
  }
}
</script>

<template>
  <template v-if="q('lighting')">
    <section v-if="material?.kind === 'location' && state" class="panel">
      <h2>{{ material.name }} · {{ state.name }} · 环境与灯光</h2>
      <button class="btn" @click="openSetReview">片场审阅 →</button>
      <p class="muted">固定场景状态；未登记灯光方案不视为已完成。</p>
      <DramaDocument v-for="r in state.definition_refs" :key="r.file_id + ':' + r.anchor" :reader="reader" :file-id="r.file_id" :anchor="r.anchor" title="场景状态依据" />
      <template v-for="v in lightingVersions" :key="v.key">
        <h3>{{ v.label }}</h3>
        <DramaDocument v-if="v.lighting_record_file_id" :reader="reader" :file-id="v.lighting_record_file_id" title="环境与灯光方案" initially-expanded />
        <p v-else class="muted">此版尚未登记灯光方案。</p>
        <DramaDocument v-if="v.spatial_record_file_id" :reader="reader" :file-id="v.spatial_record_file_id" title="空间与相机记录" />
      </template>
      <p v-if="!lightingVersions.length" class="empty">{{ q('version') ? '此状态没有指定的制作版本。' : '此状态尚未登记制作版本与灯光方案。' }}</p>
    </section>
    <p v-else class="empty">当前修订没有此场景状态。</p>
  </template>

  <template v-else-if="q('material')">
    <p v-if="!material" class="empty">当前修订没有这项资料。</p>
    <p v-else-if="q('state') && !state" class="empty">当前修订没有这个状态。</p>
    <template v-else>
      <div v-if="material.kind === 'location'" class="set-review-entry">
        <button class="btn" @click="openSetReview">片场审阅 →</button>
        <template v-if="availableSetPackages.length">
          <button class="btn primary" :disabled="setViewerBusy" @click="openProjectSetViewer()">在片场查看器中打开</button>
          <button v-if="!setPackageDownloadUrl" class="btn" @click="prepareSetPackageDownload">准备下载片场包</button>
          <a v-else class="btn" :href="setPackageDownloadUrl" download="scene.glb">下载片场包 (scene.glb)</a>
        </template>
        <span class="muted">固定证据、机位与范围结论</span>
        <div v-if="setViewerState === 'not_installed'" class="panel" style="margin-top:10px;width:100%">
          <strong>当前工作站尚未安装「片场查看器 (viewer)」</strong>
          <p class="muted" style="margin:6px 0">安装后可在浏览器内三维环绕、第一人称漫游与核对 Blender 实拍机位；您仍可直接下载片场包。</p>
          <button class="btn" @click="openSetViewerAppCenter">前往应用中心安装</button>
        </div>
        <p v-else-if="setViewerState === 'forbidden'" class="muted" role="alert">当前会话无权打开片场查看器或读取此固定修订。</p>
      </div>
      <p v-if="tab === 'materials'" class="material-note material-description">{{ human(material.description) }}</p>
      <div :class="tab === 'materials' ? 'material-detail' : 'reader-layout'">
        <div class="stack">
          <section v-if="!material.versions.length && material.file_ids.length" class="panel">
            <h2>已有成果</h2>
            <div class="thumb-grid">
              <DramaFile
                v-for="(id, i) in material.file_ids"
                :key="id"
                thumbnail
                hide-label
                :reader="reader"
                :file-id="id"
                :label="`成果 ${i + 1}`"
                :gallery="fileGallery(`material-files-${material.key}`, material.file_ids, i, '成果')"
              />
            </div>
          </section>

          <template v-if="tab === 'materials'">
            <DramaVersions
              v-if="material.versions.length || !material.file_ids.length"
              material-mode
              :reader="reader"
              :versions="material.versions.filter(v => !state || v.state_key === state?.key)"
              :selections="material.selected_versions"
            >
              <p v-if="!material.states.length" class="muted">状态尚未整理，不默认视为干净、完好或日景。</p>
              <nav v-else class="state-links" aria-label="查看状态">
                <RouterLink v-for="s in material.states" :key="s.key" :class="{ active: state?.key === s.key }" :to="link({ state: s.key, version: undefined, compare: undefined })">{{ s.name }}</RouterLink>
              </nav>
            </DramaVersions>
            <template v-else>
              <nav v-if="material.states.length" class="state-links" aria-label="查看状态">
                <RouterLink v-for="s in material.states" :key="s.key" :class="{ active: state?.key === s.key }" :to="link({ state: s.key, version: undefined, compare: undefined })">{{ s.name }}</RouterLink>
              </nav>
              <p v-else class="line-note muted">状态尚未整理，不默认视为干净、完好或日景 · 尚未登记制作版本，已有文件保留原记录。</p>
            </template>

            <section v-if="state" class="panel">
              <h2>{{ material.kind === 'location' ? '场景状态：时段、天气与光源' : '连续性细节' }}</h2>
              <div v-for="mark in state.marks" :key="mark.key" class="material-mark">
                <h3>{{ mark.part === '按原文' && mark.side === '按原文' ? '连续性要求' : mark.part + ' · ' + mark.side }}</h3>
                <DramaProse :text="mark.description" :drama="drama" />
                <DramaDocument v-for="(r, i) in mark.reference_refs" :key="`${r.file_id}:${i}`" :reader="reader" :file-id="r.file_id" :anchor="r.anchor" :title="`细节参考 ${i + 1}`" />
              </div>
              <p v-if="!state.marks.length" class="muted">本状态没有额外登记污损或破损标记；环境以状态依据为准。</p>
              <DramaDocument v-for="(r, i) in state.definition_refs" :key="`${r.file_id}:${i}`" :reader="reader" :file-id="r.file_id" :anchor="r.anchor" :initially-expanded="!!r.anchor" :title="`状态依据 ${i + 1}`" />
            </section>
            <p v-else class="line-note muted">状态依据尚未整理。</p>
          </template>

          <section v-if="tab === 'script' || material.definition_refs.length" class="panel manuscript">
            <span class="eyebrow">文字设定与生成依据</span>
            <h2>设定说明</h2>
            <DramaProse :text="material.description || '说明尚未填写。'" :drama="drama" />
            <DramaDocument v-for="(r, i) in material.definition_refs" :key="`${r.file_id}:${i}`" :reader="reader" :file-id="r.file_id" :anchor="r.anchor" :title="`完整设定资料 ${i + 1}`" />
          </section>

          <section v-if="relatedLooks.length && tab === 'materials'" class="panel character-looks">
            <h2>{{ material.kind === 'character' ? '剧情造型与状态' : '细分素材与状态' }}</h2>
            <p v-if="material.kind === 'character'" class="muted">人物身份基线用于核对同一张脸与体型；下列服装造型分别保存各阶段的衣着、剧情状态和制作版本。</p>
            <div class="material-grid">
              <article v-for="look in relatedLooks" :key="look.key" class="look-card">
                <RouterLink :to="materialLink(look, undefined, materialPreview(look).version?.state_key)">
                  <ProjectMedia v-if="materialPreview(look).fileId" :reader="reader" :file-id="materialPreview(look).fileId" kind="image" :label="look.name" thumbnail />
                  <h3>{{ look.name }}</h3>
                </RouterLink>
                <nav class="state-links" :aria-label="`${look.name}的状态`">
                  <RouterLink v-for="s in look.states" :key="s.key" :to="materialLink(look, undefined, s.key)">{{ s.name }} · {{ look.versions.filter(v => v.state_key === s.key).length }} 版</RouterLink>
                </nav>
              </article>
            </div>
          </section>

          <section v-if="material.reference_file_ids.length" class="panel">
            <h2>本素材参考资料</h2>
            <div class="thumb-grid">
              <DramaFile
                v-for="(id, i) in material.reference_file_ids"
                :key="id"
                thumbnail
                hide-label
                :reader="reader"
                :file-id="id"
                :label="`参考 ${i + 1}`"
                :gallery="fileGallery(`material-refs-${material.key}`, material.reference_file_ids, i, '参考')"
              />
            </div>
          </section>
        </div>

        <aside class="stack material-aside">
          <section class="panel">
            <h2>{{ tab === 'materials' ? '设定与关联' : '关联内容' }}</h2>
            <RouterLink v-for="g in related" :key="g.key" class="related-link" :to="materialLink(g)">{{ g.name }} →</RouterLink>
            <p v-if="!related.length" class="muted">与本项目的剧本及配套设定一同审阅。</p>
          </section>

          <section v-if="material.kind === 'music'" class="panel">
            <h2>音乐来源</h2>
            <RouterLink v-if="musicSource" class="btn" :to="link({ tab: 'references', material: undefined, state: undefined, version: undefined, compare: undefined, reference: musicSource.key })">查看来源固定版本</RouterLink>
            <p v-else class="muted">此配乐未登记直接音乐项目来源；其他资料沿现有记录查看。</p>
            <button v-if="canEditMusic" class="btn" @click="editingMusicSource = true">关联配乐来源</button>
            <ProjectEditDialog v-if="editingMusicSource" :reader="reader" mode="music-source" :item-key="material.key" @close="editingMusicSource = false" @saved="musicSaved" />
          </section>

          <DramaPublication v-if="tab === 'materials'" :reader="reader" :candidates="candidates" />

          <section class="panel usage-panel">
            <details>
              <summary>查看使用位置 · {{ usages.length }} 处</summary>
              <p v-if="!usages.length" class="muted">尚未关联使用位置。</p>
              <div v-for="u in usages" :key="u.key" class="row">
                <RouterLink :to="link({ tab: 'production', returnMaterial: undefined, continuity: undefined, deliverable: undefined, material: undefined, state: undefined, version: undefined, find: undefined, kind: undefined, compare: undefined, episode: u.target.episode_key, scene: u.target.scene_key, shot: u.target.shot_key })">{{ usageLabel(u) }} →</RouterLink>
                <p class="muted">{{ usageMaterial(u) }}</p>
                <details v-if="u.description">
                  <summary>使用说明</summary>
                  <DramaProse :text="u.description" :drama="drama" />
                </details>
              </div>
            </details>
          </section>

          <DramaTips :project-id="project.project_id" :revision-id="project.revision_id" :target="target" />
        </aside>
      </div>
    </template>
  </template>
</template>

<style scoped>
.thumb-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(340px, 1fr)); gap: 16px; align-items: start; }
@media (max-width: 720px) { .thumb-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
.thumb-grid :deep(.drama-file) { min-width: 0; min-height: 0; }
.panel { background: var(--shell-panel); border: 1px solid var(--line); border-radius: 8px; padding: 20px; min-width: 0; }
h2 { font-size: 15px; line-height: 1.35; margin: 0 0 10px; }
h3 { font-size: 15px; margin: 8px 0; }
.muted { color: var(--muted); font-size: 12px; line-height: 1.85; }
.line-note { margin: 0; padding: 4px 2px; }
.empty { color: var(--muted); padding: 0; border: 0; font-size: 13px; line-height: 1.8; }
.section-head { margin: 0 0 16px; display: flex; align-items: start; justify-content: space-between; gap: 20px; }
.btn { justify-self: start; font-size: 14px; border-radius: 8px; padding: 8px 12px; min-height: 35px; }
.material-note { color: var(--muted); font-size: 13px; line-height: 1.85; overflow-wrap: anywhere; }
.material-description { margin: 0 0 18px; }
.material-detail { display: grid; grid-template-columns: minmax(0, 1fr) 250px; gap: 20px; align-items: start; }
.reader-layout { display: grid; grid-template-columns: minmax(0, 1fr) 235px; gap: 22px; align-items: start; }
.stack, .material-aside { display: grid; gap: 18px; min-width: 0; }
.material-aside { position: sticky; top: 16px; align-self: start; }
.material-aside .usage-panel { order: 2; }
.material-aside :deep(.review) { order: 3; }
.state-links { display: flex; gap: 8px; flex-wrap: wrap; }
.state-links a { border: 1px solid var(--line); padding: 8px 12px; border-radius: 6px; font-size: 12px; }
.state-links a.active { border-color: var(--accent); background: var(--shell-button); }
.material-mark { border-left: 2px solid var(--accent); padding: 0 0 0 14px; margin: 18px 0; }
.material-mark h3 { margin-bottom: 8px; }
.manuscript { padding: 32px clamp(20px, 3vw, 44px); border: 1px solid var(--line); border-radius: 7px; background: var(--shell-panel); min-width: 0; }
.manuscript h2 { font-size: 22px; }
.eyebrow { color: var(--accent); font-size: 11px; letter-spacing: .12em; }
.character-looks .material-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 16px; margin-top: 16px; }
.look-card { border: 1px solid var(--line); border-radius: 8px; padding: 14px; min-width: 0; }
.look-card h3 { margin: 12px 0; font-size: 15px; }
.look-card :deep(img) { height: 150px; object-fit: contain; }
.look-card .state-links { font-size: 12px; }
.related-link { display: block; color: var(--accent); font-size: 13px; line-height: 1.7; padding: 6px 0; }
.row { border-top: 1px solid var(--line); margin-top: 14px; padding-top: 14px; }
.row > a { font-size: 13px; line-height: 1.75; color: var(--accent); }
summary { font-size: 12px; color: var(--muted); cursor: pointer; margin-top: 8px; }
@media (max-width: 1250px) {
  .reader-layout { grid-template-columns: minmax(0, 1fr); }
}
@media (max-width: 1200px) {
  .material-detail { grid-template-columns: minmax(0, 1fr); }
  .material-aside { grid-template-columns: repeat(2, minmax(0, 1fr)); position: static; }
}
@media (max-width: 800px) {
  .material-aside { grid-template-columns: minmax(0, 1fr); }
  .panel { padding: 16px; }
  .manuscript { padding: 24px 18px; }
}
</style>
