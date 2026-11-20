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
import { ProjectReader, projectContentView, type DramaGlobal, type DramaLocationSetup, type DramaUsage } from '../../platform/project-reader'
import { materialPreview, dramaText } from '../../platform/drama-presentation'
import { previewImage, type PreviewGallery } from '../../platform/media-preview'
import { projectSources } from '../../platform/project-source'
import VisibleThumbnail from '../VisibleThumbnail.vue'
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
const setDownloadBusy = ref(false)
const setPackageDownloadUrl = ref('')
const availableSetPackages = computed(() => detectProjectSetPackages(project.value?.manifest.files))
const lightboxOwner = {}

async function openPosterLightbox(fileId: string | null | undefined, label: string, gallery?: PreviewGallery) {
  if (!fileId) return
  try {
    const preview = await props.reader.media(fileId, 'image')
    await previewImage(lightboxOwner, preview, label, gallery)
  } catch {
    // Ignore preview error
  }
}

async function openWorldEmptySet(assetId?: string | null, versionId?: string | null, cameraId?: string) {
  if (!assetId || !versionId) return
  try {
    await appSDK.capabilities.call('navigation.openApp', {
      appId: 'world',
      path: `/scene/${assetId}?version=${versionId}${cameraId ? `&cam=${cameraId}` : ''}`,
    })
  } catch {
    // Ignore navigation error
  }
}

async function openProjectSetViewer(setId?: string, cameraId = 'C1') {
  const pkg = (setId ? availableSetPackages.value.find(p => p.setId === setId) : availableSetPackages.value[0]) || availableSetPackages.value[0]
  if (!pkg && material.value?.asset_id && material.value?.asset_version_id) {
    if (setViewerBusy.value) return
    setViewerBusy.value = true
    setViewerState.value = 'idle'
    try {
      const q = new URLSearchParams({
        asset: material.value.asset_id,
        version: material.value.asset_version_id,
        cam: cameraId || 'C1',
      })
      await appSDK.capabilities.call('navigation.openApp', { appId: 'viewer', path: `/?${q.toString()}` })
      setViewerState.value = 'opened'
    } catch (cause) {
      setViewerState.value = classifyViewerOpenError(cause)
    } finally {
      setViewerBusy.value = false
    }
    return
  }
  if (!pkg || setViewerBusy.value) return
  setViewerBusy.value = true
  setViewerState.value = 'idle'
  try {
    const target = buildProjectViewerOpenTarget({
      projectId: project.value.project_id,
      revisionId: project.value.revision_id,
      setId: pkg.setId,
      cameraId: cameraId || 'C1',
    })
    await appSDK.capabilities.call('navigation.openApp', target)
    setViewerState.value = 'opened'
  } catch (cause) {
    setViewerState.value = classifyViewerOpenError(cause)
  } finally {
    setViewerBusy.value = false
  }
}

async function downloadSetPackage(setId?: string) {
  if (setDownloadBusy.value) return
  const pkg = (setId ? availableSetPackages.value.find(p => p.setId === setId) : undefined) || availableSetPackages.value[0]
  if (!pkg) return
  setDownloadBusy.value = true
  try {
    const dl = await props.reader.download(pkg.sceneGlbFileId)
    setPackageDownloadUrl.value = dl.content_path
    if (typeof document !== 'undefined') {
      const anchor = document.createElement('a')
      anchor.href = dl.content_path
      anchor.download = `${pkg.setId || 'scene'}.glb`
      anchor.rel = 'noopener'
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
    }
  } catch {
    // Handled by reader
  } finally {
    setDownloadBusy.value = false
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
  const setup = owner.setups?.find(s => s.key === props.q('setup'))
  if (setup) return owner.states.find(s => s.key === setup.state_key)
  const latest = [...owner.versions].reverse().find(v => !props.q('state') || v.state_key === props.q('state'))
  const version = props.q('version') ? owner.versions.find(v => v.key === props.q('version')) : latest
  return owner.states.find(s => s.key === (props.q('state') || version?.state_key)) || owner.states[0]
})

const stateVersions = computed(() => {
  if (!material.value) return []
  return material.value.versions.filter(v => !state.value || v.state_key === state.value.key)
})

const selectedStateVersion = computed(() => {
  if (!material.value) return undefined
  const sel = material.value.selected_versions.find(s => !state.value || s.state_key === state.value.key)
  return sel ? material.value.versions.find(v => v.key === sel.version_key) : undefined
})

const activeLocationVersion = computed(() => {
  if (!material.value) return undefined
  if (props.q('version')) return stateVersions.value.find(v => v.key === props.q('version'))
  return selectedStateVersion.value || stateVersions.value.at(-1)
})

const activeSetup = computed<DramaLocationSetup | undefined>(() => {
  if (!material.value || material.value.kind !== 'location') return undefined
  const key = props.q('setup')
  if (!key) return undefined
  return material.value.setups.find(s => s.key === key && (!state.value || s.state_key === state.value.key)) || material.value.setups.find(s => s.key === key)
})

const setupSetPackage = computed(() => {
  if (!activeSetup.value) return undefined
  if (activeSetup.value.set_package_id) {
    return availableSetPackages.value.find(p => p.setId === activeSetup.value?.set_package_id) || availableSetPackages.value[0]
  }
  return availableSetPackages.value[0]
})

const stateSetups = computed<DramaLocationSetup[]>(() => {
  if (!material.value || material.value.kind !== 'location') return []
  return material.value.setups.filter(s => !state.value || s.state_key === state.value.key)
})

const emptySetCoverFileId = computed<string | null>(() => {
  if (!material.value) return null
  const ver = activeLocationVersion.value
  const ref = ver?.file_refs.find(f => ['image', 'preview', 'lighting', 'whitebox'].includes(f.purpose)) || ver?.file_refs[0]
  return ref?.file_id || materialPreview(material.value).fileId || material.value.file_ids[0] || null
})

function fileNameOf(fileId: string): string {
  try {
    return props.reader.file(fileId).path.split('/').pop() || ''
  } catch {
    return project.value.manifest.files.find(f => f.file_id === fileId)?.path.split('/').pop() || ''
  }
}

function isPreviewableImage(fileId: string, purpose?: string): boolean {
  const name = fileNameOf(fileId).toLowerCase()
  const ext = name.includes('.') ? name.split('.').pop() || '' : ''
  if (ext) return ['png', 'jpg', 'jpeg', 'webp', 'gif'].includes(ext)
  return purpose === 'image' || purpose === 'preview'
}

function inferCameraKey(name: string, index: number): string {
  const match = /(?:^|[_-])(c\d+|cam[_-]?[a-z0-9_]+)(?:[_.-]|$)/i.exec(name)
  return match ? match[1].toUpperCase() : `C${index + 1}`
}

const selectedEmptyPreviewIdx = ref(0)
watch(() => [material.value?.key, state.value?.key, props.q('version')], () => {
  selectedEmptyPreviewIdx.value = 0
})

const emptyPreviewItems = computed(() => {
  if (!material.value || material.value.kind !== 'location') return []
  const seen = new Set<string>()
  const items: { fileId: string; label: string; cameraKey: string }[] = []
  const ver = activeLocationVersion.value
  for (const ref of ver?.file_refs || []) {
    if (seen.has(ref.file_id) || !isPreviewableImage(ref.file_id, ref.purpose)) continue
    seen.add(ref.file_id)
    const name = fileNameOf(ref.file_id).replace(/\.[^.]+$/, '')
    const idx = items.length
    items.push({
      fileId: ref.file_id,
      label: name || `${ver?.label || '空景预览'} · ${idx + 1}`,
      cameraKey: inferCameraKey(name, idx),
    })
  }
  if (!items.length && emptySetCoverFileId.value && isPreviewableImage(emptySetCoverFileId.value, 'preview')) {
    const name = fileNameOf(emptySetCoverFileId.value).replace(/\.[^.]+$/, '')
    items.push({
      fileId: emptySetCoverFileId.value,
      label: name || `${state.value?.name || material.value.name} · 主空景`,
      cameraKey: 'C1',
    })
  }
  return items
})

const activeEmptyPreview = computed(() => emptyPreviewItems.value[selectedEmptyPreviewIdx.value] || emptyPreviewItems.value[0])
const activeEmptyPreviewFileId = computed(() => activeEmptyPreview.value?.fileId || emptySetCoverFileId.value)
const activeEmptyCameraKey = computed(() => activeEmptyPreview.value?.cameraKey || 'C1')

const emptySetPackageId = computed<string | undefined>(() => {
  if (!material.value || material.value.kind !== 'location') return undefined
  const stateCandidate = state.value ? `${material.value.key}-${state.value.key}` : undefined
  if (stateCandidate && availableSetPackages.value.some(p => p.setId === stateCandidate)) return stateCandidate
  const matCandidate = material.value.key
  if (availableSetPackages.value.some(p => p.setId === matCandidate)) return matCandidate
  const usedBySetups = new Set(material.value.setups.map(s => s.set_package_id).filter(Boolean))
  const nonSetupPkg = availableSetPackages.value.find(p => !usedBySetups.has(p.setId))
  return nonSetupPkg?.setId || availableSetPackages.value[0]?.setId
})

function reviewStateLabel(st?: string): string {
  return ({ unreviewed: '未审核', in_review: '审核中', approved: '已通过', rejected: '需修改' } as Record<string, string>)[st || 'unreviewed'] || '未审核'
}

function setupSceneLabel(setup: DramaLocationSetup): string {
  if (!setup.scene_refs.length) return ''
  return setup.scene_refs
    .map(sr => {
      const ep = drama.value.episodes.find(e => e.key === sr.episode_key)
      const sc = ep?.scenes.find(s => s.key === sr.scene_key)
      return sc?.title || sr.scene_key.toUpperCase()
    })
    .join(' · ')
}

function setupShotCount(setup: DramaLocationSetup): number {
  return usages.value.filter(u => u.setup_key === setup.key && u.target.shot_key).length
}

const setupUsages = computed<DramaUsage[]>(() => {
  if (!activeSetup.value) return []
  return usages.value.filter(u => u.setup_key === activeSetup.value?.key)
})

function characterOf(charKey: string): DramaGlobal | undefined {
  return drama.value.globals.find(g => g.kind === 'character' && g.key === charKey)
}

function wardrobeOf(wKey?: string | null): DramaGlobal | undefined {
  if (!wKey) return undefined
  return drama.value.globals.find(g => g.kind === 'wardrobe' && g.key === wKey)
}

function wardrobeStateName(wKey?: string | null, wsKey?: string | null): string {
  const w = wardrobeOf(wKey)
  if (!w) return wsKey || '默认造型'
  const st = wsKey ? w.states.find(s => s.key === wsKey) : w.states[0]
  return st ? `${w.name} · ${st.name}` : w.name
}

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
      <div class="section-head">
        <div>
          <h2>灯光与空间基线 · {{ state.name }}</h2>
          <p class="muted">固定场景状态；未登记灯光方案不视为已完成。</p>
        </div>
        <button type="button" class="btn" @click="openSetReview">片场审阅 →</button>
      </div>
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

  <template v-else-if="q('material') && q('setup')">
    <p v-if="!material || material.kind !== 'location' || !activeSetup" class="empty" data-testid="setup-missing">当前修订没有此戏份布景。</p>
    <template v-else>
      <header class="location-page-head" data-testid="setup-detail-head">
        <div>
          <h1>{{ activeSetup.label }}</h1>
          <div class="location-meta-row">
            <span class="meta-chip accent"><i class="chip-dot" />戏份布景 · {{ state?.name || activeSetup.state_key }}</span>
            <span v-if="setupSceneLabel(activeSetup)" class="meta-chip">场次：{{ setupSceneLabel(activeSetup) }}</span>
            <span class="meta-chip tabular-nums">固定修订 · {{ project.revision_id.slice(0, 8) }}</span>
            <span class="meta-chip tabular-nums">片场包 · {{ activeSetup.set_package_id || `${material.key}-${activeSetup.state_key}-${activeSetup.key}` }}</span>
            <span class="review-pill" :class="`st-${activeSetup.review.state}`">{{ reviewStateLabel(activeSetup.review.state) }}</span>
          </div>
        </div>
        <div class="location-head-actions">
          <button type="button" class="btn" @click="openSetReview">片场审阅 →</button>
          <button
            v-if="setupSetPackage"
            type="button"
            class="btn"
            :disabled="setDownloadBusy"
            data-testid="setup-download-package"
            @click="downloadSetPackage(setupSetPackage.setId)"
          >
            {{ setDownloadBusy ? '正在获取…' : '下载片场包' }}
          </button>
          <button
            type="button"
            class="btn primary"
            :disabled="setViewerBusy"
            data-testid="setup-open-viewer"
            @click="openProjectSetViewer(activeSetup.set_package_id || undefined, activeSetup.primary_camera_key || activeSetup.camera_keys[0] || 'C1')"
          >
            在片场查看器中打开
          </button>
        </div>
      </header>

      <div v-if="setViewerState === 'not_installed'" class="panel" style="margin-bottom:16px">
        <strong>当前工作站尚未安装「片场查看器 (viewer)」</strong>
        <p class="muted" style="margin:6px 0">安装后可在浏览器内三维环绕、第一人称漫游与核对 Blender 实拍机位；您仍可直接下载片场包。</p>
        <button type="button" class="btn" @click="openSetViewerAppCenter">前往应用中心安装</button>
      </div>

      <div class="material-detail location-workspace">
        <div class="stack">
          <section class="panel" data-testid="setup-poster-section">
            <div class="setup-hero-frame">
              <button
                type="button"
                class="setup-shot-trigger hero"
                :title="`点击在片场查看器中打开 ${activeSetup.label}（机位 ${activeSetup.primary_camera_key || activeSetup.camera_keys[0] || 'C1'}）`"
                data-testid="setup-hero-open-viewer"
                @click="openProjectSetViewer(activeSetup.set_package_id || undefined, activeSetup.primary_camera_key || activeSetup.camera_keys[0] || 'C1')"
              >
                <VisibleThumbnail v-if="activeSetup.poster_file_id" :cache-key="`${reader.project?.revision_id}:${activeSetup.poster_file_id}`" :load="eligible => reader.thumbnail(activeSetup!.poster_file_id!, 640, eligible)" :label="activeSetup.label" />
                <div v-else class="shot-placeholder">
                  <span>{{ activeSetup.label }} · 主机位 {{ activeSetup.primary_camera_key || activeSetup.camera_keys[0] || 'C1' }}</span>
                </div>
                <span class="shot-corner-chip">主机位 · {{ activeSetup.primary_camera_key || activeSetup.camera_keys[0] || 'C1' }} · 点击进入三维片场</span>
              </button>
              <button
                v-if="activeSetup.poster_file_id"
                type="button"
                class="shot-zoom-btn"
                title="在灯箱中放大查看原图"
                aria-label="放大查看原图"
                data-testid="setup-hero-zoom"
                @click.stop="openPosterLightbox(activeSetup.poster_file_id, activeSetup.label)"
              >
                ⊕ 原图
              </button>
            </div>

            <div class="camera-strip-section" data-testid="setup-camera-strip">
              <h3>机位截图条（{{ activeSetup.camera_keys.length }}）</h3>
              <div class="camera-strip-grid">
                <button
                  v-for="camKey in activeSetup.camera_keys"
                  :key="camKey"
                  type="button"
                  class="camera-strip-card"
                  :class="{ primary: camKey === (activeSetup.primary_camera_key || activeSetup.camera_keys[0]) }"
                  :data-testid="`setup-cam-${camKey}`"
                  @click="openProjectSetViewer(activeSetup.set_package_id || undefined, camKey)"
                >
                  <div class="cam-card-thumb">
                    <VisibleThumbnail v-if="activeSetup.poster_file_id" :cache-key="`${reader.project?.revision_id}:${activeSetup.poster_file_id}`" :load="eligible => reader.thumbnail(activeSetup!.poster_file_id!, 640, eligible)" :label="camKey" />
                    <span class="cam-id-pill">{{ camKey }}</span>
                    <span v-if="camKey === (activeSetup.primary_camera_key || activeSetup.camera_keys[0])" class="cam-primary-pill">主机位</span>
                  </div>
                  <span class="cam-card-action">在查看器锁定此机位 →</span>
                </button>
              </div>
            </div>
          </section>

          <section class="panel" data-testid="setup-cast-section">
            <h2>登场人物（{{ activeSetup.cast.length }}）</h2>
            <p v-if="!activeSetup.cast.length" class="muted">本戏份未登记登场人物。</p>
            <table v-else class="setup-cast-table">
              <thead>
                <tr>
                  <th>角色</th>
                  <th>造型状态</th>
                  <th>替身／模型</th>
                  <th>站位说明</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="c in activeSetup.cast" :key="c.character_key + ':' + (c.stand_in || '')">
                  <td>
                    <RouterLink v-if="characterOf(c.character_key)" class="related-link" :to="materialLink(characterOf(c.character_key)!)">
                      {{ characterOf(c.character_key)!.name }}
                    </RouterLink>
                    <span v-else>{{ c.character_key }}</span>
                  </td>
                  <td>
                    <RouterLink
                      v-if="wardrobeOf(c.wardrobe_key)"
                      class="related-link"
                      :to="materialLink(wardrobeOf(c.wardrobe_key)!, undefined, c.wardrobe_state_key || undefined)"
                    >
                      {{ wardrobeStateName(c.wardrobe_key, c.wardrobe_state_key) }}
                    </RouterLink>
                    <span v-else class="muted">{{ wardrobeStateName(c.wardrobe_key, c.wardrobe_state_key) }}</span>
                  </td>
                  <td><code class="mono-pill">{{ c.stand_in || '未绑定节点' }}</code></td>
                  <td class="muted">{{ c.mark_note || '—' }}</td>
                </tr>
              </tbody>
            </table>
          </section>

          <section class="panel" data-testid="setup-shots-section">
            <h2>使用镜头（{{ setupUsages.length }}）</h2>
            <p v-if="!setupUsages.length" class="muted">当前尚无分镜直接绑定此戏份布景。</p>
            <div v-else class="setup-shot-grid">
              <RouterLink
                v-for="u in setupUsages"
                :key="u.key"
                class="setup-shot-card"
                :to="link({ tab: 'production', material: undefined, state: undefined, setup: undefined, episode: u.target.episode_key, scene: u.target.scene_key, shot: u.target.shot_key })"
              >
                <div class="setup-shot-head">
                  <strong>{{ usageLabel(u) }}</strong>
                  <code v-if="u.camera_key" class="mono-pill">{{ u.camera_key }}</code>
                </div>
                <p class="muted">{{ u.description || '点击进入镜头详情核对制作版本与机位' }}</p>
                <span class="related-link">查看镜头详情 →</span>
              </RouterLink>
            </div>
          </section>
        </div>

        <aside class="stack material-aside">
          <section class="panel" data-testid="setup-lineage-panel">
            <h2>空景来源链</h2>
            <ol class="lineage-list">
              <li>
                <span class="lineage-step-label">1. 世界空景</span>
                <button
                  v-if="material.asset_id && material.asset_version_id"
                  type="button"
                  class="lineage-link-btn"
                  @click="openWorldEmptySet(material.asset_id, material.asset_version_id)"
                >
                  世界空景 · {{ material.asset_version_id.slice(0, 8) }} →
                </button>
                <span v-else class="muted">项目自建空景（未引用世界资产）</span>
              </li>
              <li>
                <span class="lineage-step-label">2. 项目场景状态</span>
                <RouterLink class="related-link" :to="link({ setup: undefined })">
                  {{ material.name }} · {{ state?.name || activeSetup.state_key }} →
                </RouterLink>
              </li>
              <li>
                <span class="lineage-step-label">3. 当前戏份布景</span>
                <strong>{{ activeSetup.label }}</strong>
              </li>
            </ol>
          </section>

          <section class="panel">
            <h2>片场审阅与版本</h2>
            <p class="muted">戏份布景与空景状态的审阅结论互不继承；人物站位或机位变化生成新修订。</p>
            <button type="button" class="btn" @click="openSetReview">进入片场审阅 →</button>
          </section>

          <DramaTips :project-id="project.project_id" :revision-id="project.revision_id" :target="target" />
        </aside>
      </div>
    </template>
  </template>

  <template v-else-if="q('material')">
    <p v-if="!material" class="empty">当前修订没有这项资料。</p>
    <p v-else-if="q('state') && !state" class="empty">当前修订没有这个状态。</p>
    <template v-else>
      <header v-if="material.kind === 'location' && tab === 'materials'" class="location-page-head" data-testid="location-detail-head">
        <div>
          <h1>{{ material.name }}</h1>
          <div class="location-meta-row">
            <span v-if="state" class="meta-chip accent"><i class="chip-dot" />{{ state.name }}</span>
            <button
              v-if="material.asset_id && material.asset_version_id"
              type="button"
              class="meta-chip link-chip"
              data-testid="location-world-source-link"
              @click="openWorldEmptySet(material.asset_id, material.asset_version_id)"
            >
              引用世界空景 · {{ material.asset_version_id.slice(0, 8) }} →
            </button>
            <span v-else class="meta-chip" data-testid="location-world-source-self">项目自建空景</span>
            <span class="meta-chip tabular-nums">{{ stateVersions.length }} 个制作版本</span>
            <span class="review-pill" :class="`st-${material.review.state}`">{{ reviewStateLabel(material.review.state) }}</span>
            <span class="meta-chip" :class="{ amber: !material.selected_versions.length }">{{ material.selected_versions.length ? '已登记选用' : '未登记选用' }}</span>
          </div>
        </div>
        <div class="location-head-actions">
          <button type="button" class="btn" @click="openSetReview">片场审阅 →</button>
          <button
            v-if="availableSetPackages.length"
            type="button"
            class="btn"
            :disabled="setDownloadBusy"
            data-testid="location-download-package"
            @click="downloadSetPackage(emptySetPackageId)"
          >
            {{ setDownloadBusy ? '正在获取…' : '下载片场包' }}
          </button>
          <button
            v-if="availableSetPackages.length || (material.asset_id && material.asset_version_id)"
            type="button"
            class="btn primary"
            :disabled="setViewerBusy"
            data-testid="location-open-viewer"
            @click="openProjectSetViewer(emptySetPackageId, activeEmptyCameraKey)"
          >
            在片场查看器中打开
          </button>
        </div>
      </header>

      <div v-if="material.kind === 'location' && setViewerState === 'not_installed'" class="panel" style="margin-bottom:16px">
        <strong>当前工作站尚未安装「片场查看器 (viewer)」</strong>
        <p class="muted" style="margin:6px 0">安装后可在浏览器内三维环绕、第一人称漫游与核对 Blender 实拍机位；您仍可直接下载片场包。</p>
        <button type="button" class="btn" @click="openSetViewerAppCenter">前往应用中心安装</button>
      </div>
      <p v-else-if="material.kind === 'location' && setViewerState === 'forbidden'" class="muted" role="alert">当前会话无权打开片场查看器或读取此固定修订。</p>

      <nav
        v-if="material.kind === 'location' && tab === 'materials' && material.states.length > 1"
        class="state-links location-state-bar"
        aria-label="场景剧情状态"
        data-testid="location-state-tabs"
      >
        <RouterLink
          v-for="s in material.states"
          :key="s.key"
          :class="{ active: state?.key === s.key }"
          :to="link({ state: s.key, setup: undefined, version: undefined, compare: undefined })"
        >
          {{ s.name }}
        </RouterLink>
      </nav>

      <p v-if="tab === 'materials' && material.kind !== 'location' && !material.definition_refs.length" class="material-note material-description">{{ human(material.description) }}</p>

      <div :class="[tab === 'materials' ? 'material-detail' : 'reader-layout', { 'location-workspace': material.kind === 'location' && tab === 'materials' }]">
        <div class="stack">
          <section v-if="material.kind === 'location' && tab === 'materials'" class="location-layer-section" data-testid="location-set-layer-grid">
            <div class="sec-head">
              <h2>空景与戏份布景</h2>
              <span class="muted tabular-nums">1 空景 · {{ stateSetups.length }} 戏份布景</span>
            </div>

            <article v-if="!stateSetups.length" class="empty-showcase" data-testid="empty-set-showcase">
              <div class="hero-stage" data-testid="empty-set-card">
                <button
                  type="button"
                  class="setup-shot-trigger hero-trigger"
                  :title="`点击在片场查看器中打开 ${material.name} 空景`"
                  data-testid="empty-set-open-viewer"
                  @click="openProjectSetViewer(emptySetPackageId, activeEmptyCameraKey)"
                >
                  <template v-if="activeEmptyPreviewFileId">
                    <VisibleThumbnail
                      class="hero-backdrop-thumb"
                      :cache-key="`${reader.project?.revision_id}:${activeEmptyPreviewFileId}`"
                      :load="eligible => reader.thumbnail(activeEmptyPreviewFileId!, 640, eligible)"
                      label=""
                      aria-hidden="true"
                    />
                    <VisibleThumbnail
                      class="hero-fg-thumb"
                      :cache-key="`${reader.project?.revision_id}:${activeEmptyPreviewFileId}`"
                      :load="eligible => reader.thumbnail(activeEmptyPreviewFileId!, 640, eligible)"
                      :label="`${material.name} 空景`"
                    />
                  </template>
                  <div v-else class="shot-placeholder">
                    <span>{{ state?.name || '默认状态' }} · 空景基线</span>
                  </div>
                  <div class="hud-tl">
                    <span class="glass-pill"><i class="dot" />空景 · 无人物</span>
                    <span v-if="activeLocationVersion" class="glass-pill">{{ activeLocationVersion.label }}</span>
                  </div>
                  <span class="glass-pill hover-open-pill">点击进入三维片场查看器 →</span>
                </button>
                <button
                  v-if="activeEmptyPreviewFileId"
                  type="button"
                  class="shot-zoom-btn hero-zoom"
                  title="在灯箱中放大查看空景原图"
                  aria-label="放大查看空景原图"
                  data-testid="empty-set-zoom"
                  @click.stop="openPosterLightbox(activeEmptyPreviewFileId, `${material.name} · ${state?.name || ''} 空景`, emptyPreviewItems.length > 1 ? { id: `empty-${material.key}`, index: selectedEmptyPreviewIdx, count: emptyPreviewItems.length } : undefined)"
                >
                  ⊕ 原图
                </button>
              </div>

              <div v-if="emptyPreviewItems.length > 1" class="cam-strip-wrap" data-testid="empty-preview-strip">
                <div class="cam-strip-bar">
                  <span>空景多视角快览（{{ emptyPreviewItems.length }} 张预览图）</span>
                  <button
                    v-if="availableSetPackages.length || (material.asset_id && material.asset_version_id)"
                    type="button"
                    class="lineage-link-btn"
                    @click="openProjectSetViewer(emptySetPackageId, activeEmptyCameraKey)"
                  >
                    在查看器中漫游全部机位 →
                  </button>
                </div>
                <div class="cam-strip">
                  <button
                    v-for="(item, idx) in emptyPreviewItems.slice(0, 6)"
                    :key="item.fileId"
                    type="button"
                    class="cam-thumb"
                    :class="{ active: idx === selectedEmptyPreviewIdx }"
                    @click="selectedEmptyPreviewIdx = idx"
                  >
                    <VisibleThumbnail :cache-key="`${reader.project?.revision_id}:${item.fileId}`" :load="eligible => reader.thumbnail(item.fileId, 320, eligible)" :label="item.label" />
                    <span>{{ item.label }}</span>
                  </button>
                </div>
              </div>

              <div class="setup-empty-bar" data-testid="empty-setups-hint">
                <span>当前状态尚无人物登场戏份布景 · 场次演员站位与实拍机位定稿后将并列于此</span>
              </div>
            </article>

            <div v-else class="set-layer-grid">
              <article class="set-layer-card empty-set-card" data-testid="empty-set-card">
                <div class="setup-shot-frame">
                  <button
                    type="button"
                    class="setup-shot-trigger"
                    :title="`点击在片场查看器中打开 ${material.name} 空景`"
                    data-testid="empty-set-open-viewer"
                    @click="openProjectSetViewer(emptySetPackageId, 'C1')"
                  >
                    <VisibleThumbnail v-if="emptySetCoverFileId" :cache-key="`${reader.project?.revision_id}:${emptySetCoverFileId}`" :load="eligible => reader.thumbnail(emptySetCoverFileId!, 640, eligible)" :label="`${material.name} 空景`" />
                    <div v-else class="shot-placeholder">
                      <span>{{ state?.name || '默认状态' }} · 空景基线</span>
                    </div>
                    <span class="shot-corner-chip empty-chip">空景 · 无人物</span>
                  </button>
                  <button
                    v-if="emptySetCoverFileId"
                    type="button"
                    class="shot-zoom-btn"
                    title="在灯箱中放大查看空景原图"
                    aria-label="放大查看空景原图"
                    data-testid="empty-set-zoom"
                    @click.stop="openPosterLightbox(emptySetCoverFileId, `${material.name} · ${state?.name || ''} 空景`)"
                  >
                    ⊕
                  </button>
                </div>
                <div class="set-layer-body">
                  <div class="set-layer-title-row">
                    <strong>{{ state?.name || '默认状态' }} · 空景基线</strong>
                    <span class="review-pill" :class="`st-${material.review.state}`">{{ reviewStateLabel(material.review.state) }}</span>
                  </div>
                  <p class="muted">
                    {{ material.asset_id ? `引用世界空景 · ${material.asset_version_id?.slice(0, 8)}` : '项目自建空景' }} · 不含任何人物替身
                  </p>
                </div>
              </article>

              <article
                v-for="setup in stateSetups"
                :key="setup.key"
                class="set-layer-card setup-card"
                :data-testid="`setup-card-${setup.key}`"
              >
                <div class="setup-shot-frame">
                  <button
                    type="button"
                    class="setup-shot-trigger"
                    :title="`点击在片场查看器中打开 ${setup.label}（机位 ${setup.primary_camera_key || setup.camera_keys[0] || 'C1'}）`"
                    :data-testid="`setup-shot-open-${setup.key}`"
                    @click="openProjectSetViewer(setup.set_package_id || undefined, setup.primary_camera_key || setup.camera_keys[0] || 'C1')"
                  >
                    <VisibleThumbnail v-if="setup.poster_file_id" :cache-key="`${reader.project?.revision_id}:${setup.poster_file_id}`" :load="eligible => reader.thumbnail(setup.poster_file_id!, 640, eligible)" :label="setup.label" />
                    <div v-else class="shot-placeholder">
                      <span>{{ setup.label }}</span>
                    </div>
                    <span class="shot-corner-chip">{{ setup.primary_camera_key || setup.camera_keys[0] || 'C1' }} · 点击看片场</span>
                  </button>
                  <button
                    v-if="setup.poster_file_id"
                    type="button"
                    class="shot-zoom-btn"
                    title="在灯箱中放大查看原图"
                    aria-label="放大查看原图"
                    :data-testid="`setup-shot-zoom-${setup.key}`"
                    @click.stop="openPosterLightbox(setup.poster_file_id, setup.label)"
                  >
                    ⊕
                  </button>
                </div>
                <div class="set-layer-body">
                  <div class="set-layer-title-row">
                    <RouterLink
                      class="setup-title-link"
                      :data-testid="`setup-title-link-${setup.key}`"
                      :to="link({ state: setup.state_key, setup: setup.key })"
                    >
                      {{ setupSceneLabel(setup) ? `${setupSceneLabel(setup)} · ${setup.label}` : setup.label }}
                    </RouterLink>
                    <span class="review-pill" :class="`st-${setup.review.state}`">{{ reviewStateLabel(setup.review.state) }}</span>
                  </div>
                  <div class="setup-cast-pills">
                    <span v-for="c in setup.cast.slice(0, 4)" :key="c.character_key" class="cast-pill">
                      {{ characterOf(c.character_key)?.name || c.character_key }}
                    </span>
                    <span v-if="setup.cast.length > 4" class="cast-pill more">+{{ setup.cast.length - 4 }}</span>
                  </div>
                  <div class="setup-card-meta muted">
                    <span>{{ setup.camera_keys.length }} 机位</span>
                    <span>·</span>
                    <span>{{ setupShotCount(setup) }} 镜头</span>
                  </div>
                </div>
              </article>
            </div>
          </section>

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

          <template v-if="tab === 'materials' && material.kind === 'location'">
            <section class="panel" data-testid="location-spatial-basis-panel">
              <h2>空间设定与环境依据</h2>
              <p v-if="material.description" class="summary-lead">{{ human(material.description) }}</p>
              <p v-else-if="!state?.marks.length && !state?.definition_refs.length && !material.definition_refs.length" class="muted">空间说明与环境依据尚未填写。</p>
              <div v-for="mark in state?.marks || []" :key="mark.key" class="material-mark">
                <h3>{{ mark.part === '按原文' && mark.side === '按原文' ? '空间与环境要求' : mark.part + ' · ' + mark.side }}</h3>
                <DramaProse :text="mark.description" :drama="drama" />
                <DramaDocument v-for="(r, i) in mark.reference_refs" :key="`${r.file_id}:${i}`" :reader="reader" :file-id="r.file_id" :anchor="r.anchor" :title="`细节参考 ${i + 1}`" />
              </div>
              <div v-if="(state?.definition_refs.length || 0) + material.definition_refs.length > 0" class="doc-drawer-list" data-testid="location-doc-drawers">
                <DramaDocument
                  v-for="(r, i) in state?.definition_refs || []"
                  :key="`state:${r.file_id}:${i}`"
                  :reader="reader"
                  :file-id="r.file_id"
                  :anchor="r.anchor"
                  :title="r.anchor ? `场景状态与空间规范 · ${r.anchor}` : `场景状态依据 ${i + 1}`"
                />
                <DramaDocument
                  v-for="(r, i) in material.definition_refs"
                  :key="`mat:${r.file_id}:${i}`"
                  :reader="reader"
                  :file-id="r.file_id"
                  :anchor="r.anchor"
                  :title="r.anchor ? `完整设定资料 · ${r.anchor}` : `完整设定资料 ${i + 1}`"
                />
              </div>
            </section>

            <details v-if="material.versions.length || !material.file_ids.length" class="panel versions-fold" data-testid="location-versions-fold">
              <summary>
                <span>历史制作版本（{{ stateVersions.length }}）</span>
                <span class="fold-hint">展开查看</span>
              </summary>
              <DramaVersions
                material-mode
                :reader="reader"
                :versions="stateVersions"
                :selections="material.selected_versions"
              />
            </details>
          </template>

          <template v-else-if="tab === 'materials'">
            <DramaVersions
              v-if="material.versions.length || !material.file_ids.length"
              material-mode
              :reader="reader"
              :versions="stateVersions"
              :selections="material.selected_versions"
            >
              <p v-if="!material.states.length" class="muted">状态尚未整理，不默认视为干净、完好或日景。</p>
              <nav v-else-if="material.states.length > 1" class="state-links" aria-label="查看状态">
                <RouterLink v-for="s in material.states" :key="s.key" :class="{ active: state?.key === s.key }" :to="link({ state: s.key, version: undefined, compare: undefined })">{{ s.name }}</RouterLink>
              </nav>
            </DramaVersions>
            <template v-else>
              <nav v-if="material.states.length > 1" class="state-links" aria-label="查看状态">
                <RouterLink v-for="s in material.states" :key="s.key" :class="{ active: state?.key === s.key }" :to="link({ state: s.key, version: undefined, compare: undefined })">{{ s.name }}</RouterLink>
              </nav>
              <p v-else-if="!material.states.length" class="line-note muted">状态尚未整理，不默认视为干净、完好或日景 · 尚未登记制作版本，已有文件保留原记录。</p>
            </template>

            <section v-if="state" class="panel">
              <h2>连续性细节</h2>
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

          <section v-if="tab === 'script' || (material.kind !== 'location' && material.definition_refs.length)" class="panel manuscript">
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
                  <ProjectMedia v-if="materialPreview(look).fileId" :reader="reader" :file-id="materialPreview(look).fileId" kind="image" :label="look.name" thumbnail :edge="640" />
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
          <section v-if="material.kind === 'location' && tab === 'materials'" class="panel" data-testid="location-spec-panel">
            <h2>片场规格与来源</h2>
            <dl class="rail-kv">
              <dt>空间状态</dt>
              <dd>{{ state?.name || '未整理' }}</dd>
              <dt>资产来源</dt>
              <dd>
                <button
                  v-if="material.asset_id && material.asset_version_id"
                  type="button"
                  class="lineage-link-btn"
                  @click="openWorldEmptySet(material.asset_id, material.asset_version_id)"
                >
                  数字世界 · {{ material.asset_version_id.slice(0, 8) }} →
                </button>
                <span v-else>项目自建空景</span>
              </dd>
              <dt>片场包</dt>
              <dd class="tabular-nums">{{ emptySetPackageId || '尚未导出' }}</dd>
              <dt>选用版本</dt>
              <dd>{{ selectedStateVersion?.label || '尚未登记正式选用' }}</dd>
            </dl>
          </section>

          <section class="panel">
            <h2>{{ tab === 'materials' ? '设定与关联' : '关联内容' }}</h2>
            <RouterLink v-for="g in related" :key="g.key" class="related-link" :to="materialLink(g)">{{ g.name }} →</RouterLink>
            <p v-if="!related.length" class="muted">与本项目的剧本及配套设定一同审阅。</p>
          </section>

          <section v-if="material.kind === 'music'" class="panel">
            <h2>音乐来源</h2>
            <RouterLink v-if="musicSource" class="btn" :to="link({ tab: 'references', material: undefined, state: undefined, version: undefined, compare: undefined, reference: musicSource.key })">查看来源固定版本</RouterLink>
            <p v-else class="muted">此配乐未登记直接音乐项目来源；其他资料沿现有记录查看。</p>
            <button v-if="canEditMusic" type="button" class="btn" @click="editingMusicSource = true">关联配乐来源</button>
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
.panel { background: var(--panel, var(--shell-panel)); border: 0.5px solid var(--vf-hairline-outer, var(--line)); border-radius: 12px; box-shadow: var(--vf-shadow-card); padding: 18px 20px; min-width: 0; }
h2 { font-size: 15px; line-height: 1.35; margin: 0 0 10px; }
h3 { font-size: 15px; margin: 8px 0; }
.muted { color: var(--muted); font-size: 12px; line-height: 1.85; }
.line-note { margin: 0; padding: 4px 2px; }
.empty { color: var(--muted); padding: 0; border: 0; font-size: 13px; line-height: 1.8; }
.section-head { margin: 0 0 16px; display: flex; align-items: start; justify-content: space-between; gap: 20px; }
.sec-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; margin-bottom: 10px; }
.sec-head h2 { margin: 0; font-size: 15.5px; font-weight: 620; }
.btn { justify-self: start; font-size: 13px; border-radius: 8px; padding: 0 14px; height: 36px; min-height: 36px; }
.material-note { color: var(--muted); font-size: 13px; line-height: 1.85; overflow-wrap: anywhere; }
.material-description { margin: 0 0 18px; }
.material-detail { display: grid; grid-template-columns: minmax(0, 1fr) 260px; gap: 20px; align-items: start; }
.material-detail.location-workspace { grid-template-columns: minmax(0, 1fr) 300px; gap: 22px; }
.reader-layout { display: grid; grid-template-columns: minmax(0, 1fr) 235px; gap: 22px; align-items: start; }
.stack, .material-aside { display: grid; gap: 18px; min-width: 0; }
.location-workspace .material-aside { gap: 14px; }
.material-aside { position: sticky; top: 16px; align-self: start; }
.material-aside .usage-panel { order: 2; }
.material-aside :deep(.review) { order: 3; }
.state-links { display: flex; gap: 6px; flex-wrap: wrap; }
.state-links a { border: 0.5px solid var(--vf-hairline-outer, var(--line)); padding: 6px 13px; border-radius: 8px; font-size: 12.5px; color: var(--muted); background: var(--panel); }
.state-links a.active { border-color: var(--accent); color: var(--ink); font-weight: 600; background: var(--subtle); box-shadow: var(--vf-shadow-card); }
.material-mark { border-left: 2px solid var(--accent); padding: 0 0 0 14px; margin: 14px 0; }
.material-mark h3 { margin-bottom: 6px; font-size: 14px; }
.manuscript { padding: 32px clamp(20px, 3vw, 44px); border: 0.5px solid var(--vf-hairline-outer, var(--line)); border-radius: 12px; background: var(--panel); min-width: 0; }
.manuscript h2 { font-size: 20px; }
.eyebrow { color: var(--accent); font-size: 11px; letter-spacing: .12em; }
.character-looks .material-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 16px; margin-top: 16px; }
.look-card { border: 0.5px solid var(--vf-hairline-outer, var(--line)); border-radius: 8px; padding: 14px; min-width: 0; }
.look-card h3 { margin: 12px 0; font-size: 15px; }
.look-card :deep(img) { height: 150px; object-fit: contain; }
.look-card .state-links { font-size: 12px; }
.related-link { display: block; color: var(--accent); font-size: 13px; line-height: 1.7; padding: 5px 0; }
.row { border-top: 0.5px solid var(--vf-hairline-outer, var(--line)); margin-top: 14px; padding-top: 14px; }
.row > a { font-size: 13px; line-height: 1.75; color: var(--accent); }
summary { font-size: 12.5px; color: var(--muted); cursor: pointer; margin-top: 4px; }

/* 唯一页头（Single Page Header Ownership） */
.location-page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 20px; flex-wrap: wrap; padding-bottom: 16px; margin-bottom: 18px; border-bottom: 0.5px solid var(--vf-hairline-outer, var(--line)); }
.location-page-head h1 { font-size: 26px; font-weight: 660; letter-spacing: -0.012em; margin: 0 0 8px; line-height: 1.24; color: var(--ink); }
.location-meta-row { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 8px; font-size: 12px; color: var(--muted); }
.meta-chip { display: inline-flex; align-items: center; gap: 5px; height: 24px; padding: 0 10px; border-radius: 999px; background: var(--panel); border: 0.5px solid var(--vf-hairline-outer, var(--line)); font-size: 12px; color: var(--ink); white-space: nowrap; }
.meta-chip.accent { background: var(--accent-glow); color: var(--accent); border-color: transparent; font-weight: 560; }
.meta-chip.amber { color: var(--amber); border-color: var(--amber); }
.meta-chip.link-chip { color: var(--accent); cursor: pointer; }
.meta-chip.link-chip:hover { border-color: var(--accent); text-decoration: underline; }
.chip-dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; display: inline-block; }
.location-head-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding-top: 2px; }
.location-head-actions .btn.primary { background: var(--accent); border-color: var(--accent); color: #ffffff; font-weight: 600; box-shadow: none; }
:global(body[data-theme="dark"]) .location-head-actions .btn.primary { color: #0d1914; }
.location-state-bar { display: inline-flex; padding: 3px; border-radius: 10px; background: var(--panel); box-shadow: var(--vf-shadow-card); margin-bottom: 18px; }
.location-state-bar a { border: 0; padding: 6px 14px; border-radius: 7px; background: transparent; }
.location-state-bar a.active { background: var(--subtle); box-shadow: 0 0 0 0.5px var(--vf-hairline-outer, var(--line)); }

/* 自适应空景大画幅展台（0 戏份时） */
.location-layer-section { min-width: 0; }
.empty-showcase { background: var(--panel); border: 0.5px solid var(--vf-hairline-outer, var(--line)); border-radius: 12px; box-shadow: var(--vf-shadow-card); overflow: hidden; }
.hero-stage { position: relative; width: 100%; height: clamp(320px, 34vw, 460px); background: radial-gradient(circle at 50% 45%, #2a312d 0%, #171c19 100%); overflow: hidden; }
.hero-trigger { position: relative; width: 100%; height: 100%; padding: 0; border: 0; background: transparent; cursor: pointer; display: block; }
.hero-stage :deep(.hero-backdrop-thumb) { position: absolute; inset: -16px; width: calc(100% + 32px); height: calc(100% + 32px); background: transparent; pointer-events: none; filter: blur(22px) brightness(0.46) saturate(0.85); }
.hero-stage :deep(.hero-backdrop-thumb img) { width: 100%; height: 100%; object-fit: cover; }
.hero-stage :deep(.hero-fg-thumb) { position: relative; z-index: 1; width: 100%; height: 100%; background: transparent; }
.hero-stage :deep(.hero-fg-thumb img) { width: 100%; height: 100%; object-fit: contain; transition: transform 240ms var(--vf-ease-out, ease); }
.hero-stage:hover :deep(.hero-fg-thumb img) { transform: scale(1.012); }
.hud-tl { position: absolute; left: 14px; top: 14px; z-index: 3; display: flex; gap: 6px; flex-wrap: wrap; pointer-events: none; }
.glass-pill { display: inline-flex; align-items: center; gap: 6px; height: 26px; padding: 0 10px; border-radius: 999px; background: rgba(18, 24, 21, .76); color: #f3f7f4; backdrop-filter: blur(10px); font-size: 12px; font-weight: 520; border: 0.5px solid rgba(255, 255, 255, .18); }
.glass-pill .dot { width: 6px; height: 6px; border-radius: 50%; background: #6ee2be; display: inline-block; }
.hover-open-pill { position: absolute; left: 50%; bottom: 14px; z-index: 3; transform: translateX(-50%) translateY(4px); opacity: 0; transition: opacity 180ms ease, transform 180ms ease; pointer-events: none; }
.hero-stage:hover .hover-open-pill { opacity: 1; transform: translateX(-50%) translateY(0); }
.cam-strip-wrap { padding: 12px 16px 14px; border-top: 0.5px solid var(--vf-hairline-outer, var(--line)); display: grid; gap: 10px; }
.cam-strip-bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; font-size: 12px; color: var(--muted); }
.cam-strip { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 8px; }
@media (max-width: 900px) { .cam-strip { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
.cam-thumb { position: relative; padding: 0; border: 0.5px solid var(--vf-hairline-outer, var(--line)); border-radius: 7px; overflow: hidden; aspect-ratio: 16 / 10; background: #1e2421; cursor: pointer; text-align: left; }
.cam-thumb :deep(img) { width: 100%; height: 100%; object-fit: cover; display: block; }
.cam-thumb span { position: absolute; inset: auto 4px 4px 4px; padding: 1px 5px; border-radius: 4px; background: rgba(14, 18, 16, .74); color: #eef3ef; font-size: 10.5px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.cam-thumb.active { box-shadow: 0 0 0 2px var(--accent); border-color: var(--accent); }
.setup-empty-bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; padding: 10px 16px; background: var(--subtle); border-top: 0.5px solid var(--vf-hairline-outer, var(--line)); font-size: 12.5px; color: var(--muted); }

/* 有戏份时的 16:9 卡片网格 */
.set-layer-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(290px, 1fr)); gap: 14px; align-items: stretch; }
.set-layer-card { display: flex; flex-direction: column; border: 0.5px solid var(--vf-hairline-outer, var(--line)); border-radius: 12px; overflow: hidden; background: var(--panel); box-shadow: var(--vf-shadow-card); min-width: 0; }
.set-layer-card:hover { border-color: var(--accent); box-shadow: var(--vf-shadow-card-hover); }
.setup-shot-frame, .setup-hero-frame { position: relative; width: 100%; aspect-ratio: 16 / 9; background: #1e2421; overflow: hidden; }
.setup-hero-frame { border-radius: 10px; border: 0.5px solid var(--vf-hairline-outer, var(--line)); }
.setup-shot-trigger { position: relative; width: 100%; height: 100%; padding: 0; border: 0; background: transparent; cursor: pointer; display: block; text-align: left; }
.setup-shot-trigger :deep(img) { width: 100%; height: 100%; object-fit: cover; display: block; }
.setup-hero-frame .setup-shot-trigger :deep(img) { object-fit: contain; }
.shot-placeholder { width: 100%; height: 100%; display: grid; place-items: center; padding: 16px; color: #c8d2cb; font-size: 13px; text-align: center; }
.shot-corner-chip { position: absolute; left: 10px; bottom: 10px; padding: 3px 9px; border-radius: 999px; font-size: 11px; background: rgba(18, 24, 21, .76); color: #f3f7f4; border: 0.5px solid rgba(255, 255, 255, .18); pointer-events: none; }
.shot-corner-chip.empty-chip { color: #6ee2be; }
.shot-zoom-btn { position: absolute; top: 12px; right: 12px; z-index: 3; height: 28px; min-width: 28px; padding: 0 10px; border-radius: 8px; border: 0.5px solid rgba(255, 255, 255, .22); background: rgba(18, 24, 21, .76); color: #f3f7f4; backdrop-filter: blur(8px); font-size: 12px; cursor: pointer; }
.shot-zoom-btn:hover { background: var(--accent); border-color: var(--accent); color: #ffffff; }
.set-layer-body { padding: 12px 14px 14px; display: flex; flex-direction: column; gap: 8px; flex: 1; }
.set-layer-title-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; font-size: 14px; }
.setup-title-link { font-weight: 620; color: var(--ink); text-decoration: none; }
.setup-title-link:hover { color: var(--accent); }
.setup-cast-pills { display: flex; flex-wrap: wrap; gap: 6px; }
.cast-pill { display: inline-flex; align-items: center; padding: 2px 8px; border-radius: 999px; font-size: 11px; background: var(--subtle); color: var(--ink); border: 0.5px solid var(--vf-hairline-outer, var(--line)); }
.cast-pill.more { color: var(--muted); }
.setup-card-meta { display: flex; align-items: center; gap: 6px; font-size: 12px; margin-top: auto; }
.review-pill { display: inline-flex; align-items: center; height: 24px; padding: 0 9px; border-radius: 999px; font-size: 11.5px; border: 0.5px solid var(--vf-hairline-outer, var(--line)); background: var(--panel); color: var(--muted); white-space: nowrap; }
.review-pill.st-approved { color: var(--accent); border-color: var(--accent); }
.lineage-link-btn { padding: 0; border: 0; background: transparent; color: var(--accent); font: inherit; font-size: 12.5px; cursor: pointer; text-align: left; }
.lineage-link-btn:hover { text-decoration: underline; }

/* 空间设定与折叠抽屉、右栏规格表 */
.summary-lead { margin: 0 0 12px; font-size: 13.5px; line-height: 1.75; color: var(--ink); }
.doc-drawer-list { display: grid; gap: 8px; margin-top: 12px; padding-top: 12px; border-top: 0.5px solid var(--vf-hairline-outer, var(--line)); }
.doc-drawer-list :deep(.drama-document) { margin: 0; }
.versions-fold summary { list-style: none; display: flex; align-items: center; justify-content: space-between; gap: 12px; font-size: 14px; font-weight: 600; color: var(--ink); margin: 0; }
.versions-fold summary::-webkit-details-marker { display: none; }
.versions-fold .fold-hint { font-size: 12px; color: var(--accent); font-weight: 500; }
.versions-fold[open] > summary { margin-bottom: 14px; }
.versions-fold[open] > summary .fold-hint { display: none; }
.rail-kv { display: grid; grid-template-columns: 68px minmax(0, 1fr); gap: 8px 10px; margin: 0; font-size: 12.5px; line-height: 1.5; }
.rail-kv dt { color: var(--muted); }
.rail-kv dd { margin: 0; color: var(--ink); }

.camera-strip-section { margin-top: 16px; }
.camera-strip-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 12px; margin-top: 10px; }
.camera-strip-card { display: flex; flex-direction: column; gap: 6px; padding: 8px; border-radius: 8px; border: 0.5px solid var(--vf-hairline-outer, var(--line)); background: var(--panel); cursor: pointer; text-align: left; }
.camera-strip-card.primary, .camera-strip-card:hover { border-color: var(--accent); }
.cam-card-thumb { position: relative; width: 100%; aspect-ratio: 16 / 9; border-radius: 6px; overflow: hidden; background: var(--subtle); }
.cam-card-thumb :deep(img) { width: 100%; height: 100%; object-fit: cover; display: block; }
.cam-id-pill { position: absolute; left: 6px; bottom: 6px; padding: 1px 6px; border-radius: 4px; font-size: 11px; font-family: monospace; background: var(--shell-panel); color: var(--ink); border: 0.5px solid var(--vf-hairline-outer, var(--line)); }
.cam-primary-pill { position: absolute; right: 6px; top: 6px; padding: 1px 6px; border-radius: 4px; font-size: 10px; background: var(--accent); color: #111310; font-weight: 600; }
.cam-card-action { font-size: 11px; color: var(--accent); }
.setup-cast-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.setup-cast-table th, .setup-cast-table td { padding: 10px 8px; border-bottom: 0.5px solid var(--vf-hairline-outer, var(--line)); text-align: left; vertical-align: middle; }
.setup-cast-table th { font-size: 12px; color: var(--muted); font-weight: 500; }
.mono-pill { display: inline-block; padding: 2px 6px; border-radius: 4px; font-family: monospace; font-size: 11px; background: var(--subtle); border: 0.5px solid var(--vf-hairline-outer, var(--line)); }
.setup-shot-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 12px; }
.setup-shot-card { display: flex; flex-direction: column; gap: 6px; padding: 12px; border-radius: 8px; border: 0.5px solid var(--vf-hairline-outer, var(--line)); background: var(--panel); text-decoration: none; color: inherit; }
.setup-shot-card:hover { border-color: var(--accent); }
.setup-shot-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; font-size: 13px; }
.lineage-list { list-style: none; margin: 0; padding: 0; display: grid; gap: 10px; font-size: 13px; }
.lineage-list li { display: flex; flex-direction: column; gap: 2px; padding-bottom: 8px; border-bottom: 0.5px solid var(--vf-hairline-outer, var(--line)); }
.lineage-list li:last-child { border-bottom: 0; padding-bottom: 0; }
.lineage-step-label { font-size: 11px; color: var(--muted); }
@media (max-width: 1250px) {
  .reader-layout { grid-template-columns: minmax(0, 1fr); }
}
@media (max-width: 1200px) {
  .material-detail, .material-detail.location-workspace { grid-template-columns: minmax(0, 1fr); }
  .material-aside { grid-template-columns: repeat(2, minmax(0, 1fr)); position: static; }
}
@media (max-width: 800px) {
  .material-aside { grid-template-columns: minmax(0, 1fr); }
  .panel { padding: 16px; }
  .manuscript { padding: 24px 18px; }
}
</style>
