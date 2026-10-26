<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useVisible } from '../platform/visible'
import { useHost } from '../platform/host'
import { ProjectReader, readerError } from '../platform/project-reader'
import { previewImage, closePreview } from '../platform/media-preview'
import {
  buildProjectViewerOpenTarget,
  classifyViewerOpenError,
  findSetPackageForFile,
  type ViewerLaunchOutcome,
} from '../platform/set-viewer-entry'
export interface MediaGallery { id: string; items: { fileId: string; label: string }[]; index: number }
const props = withDefaults(defineProps<{ reader: ProjectReader; fileId?: string | null; kind: 'image' | 'audio' | 'video' | 'file'; label: string; thumbnail?: boolean; contain?: boolean; edge?: 320 | 640; objectPosition?: { x: number; y: number }; gallery?: MediaGallery }>(), { thumbnail: false, contain: false, edge: 320 })
const root = ref<HTMLElement | null>(null), visible = useVisible(root)
const posterSource = ref(''), posterBusy = ref(false), posterFailed = ref(false)
const source = ref(''), lightboxSource = ref(''), lightboxLabel = ref(''), error = ref(''), busy = ref(false), downloadPath = ref('')
const viewerState = ref<ViewerLaunchOutcome | 'idle'>('idle'), viewerBusy = ref(false)
const matchedSetPackage = computed(() => findSetPackageForFile(props.reader.project?.manifest.files, props.fileId))

async function openInSetViewer() {
  const pkg = matchedSetPackage.value
  const proj = props.reader.project
  if (!pkg || !proj || viewerBusy.value) return
  viewerBusy.value = true
  viewerState.value = 'idle'
  try {
    const { appSDK } = useHost()
    const target = buildProjectViewerOpenTarget({
      projectId: proj.project_id,
      revisionId: proj.revision_id,
      setId: pkg.setId,
      cameraId: 'C1',
    })
    await appSDK.capabilities.call('navigation.openApp', target)
    viewerState.value = 'opened'
  } catch (cause) {
    viewerState.value = classifyViewerOpenError(cause)
  } finally {
    viewerBusy.value = false
  }
}

async function openViewerMarketplace() {
  try {
    const { appSDK } = useHost()
    await appSDK.navigation.request({ target: 'appcenter', path: '/?app=viewer' })
  } catch {
    // Host navigation error ignored
  }
}
const oversizedThumb = ref(false), nonImageFallback = ref(false), activeGalleryIndex = ref(0)
const viewer = ref<HTMLDialogElement | null>(null)
const videoViewer = ref<HTMLDialogElement | null>(null)
const videoOpen = ref(false)
const zoomScale = ref(1), zoomTx = ref(0), zoomTy = ref(0), isDragging = ref(false)
let dragStartX = 0, dragStartY = 0
let generation = 0
let videoRequest = 0, imageRequest = 0
let lastTrigger: HTMLElement | null = null
const previewOwner = {}, previewError = ref('')

const hasGallery = computed(() => (props.gallery?.items.length || 0) > 1)

function clampZoom(next: number) { return Math.max(0.5, Math.min(5, Math.round(next * 100) / 100)) }
function resetZoom() { zoomScale.value = 1; zoomTx.value = 0; zoomTy.value = 0; isDragging.value = false }
function stepZoom(delta: number) { zoomScale.value = clampZoom(zoomScale.value + delta) }
async function openLightbox(event?: Event) {
  const current = generation, request = ++imageRequest
  if (event?.currentTarget instanceof HTMLElement) lastTrigger = event.currentTarget
  previewError.value = ''
  activeGalleryIndex.value = props.gallery?.index ?? 0
  lightboxLabel.value = props.label
  try {
    const preview = props.fileId ? await props.reader.media(props.fileId, 'image') : source.value
    if (current !== generation || request !== imageRequest || !visible.value) return
    const galleryMeta = props.gallery && props.gallery.items.length > 1 ? { id: props.gallery.id, index: activeGalleryIndex.value, count: props.gallery.items.length } : undefined
    if (await previewImage(previewOwner, preview, props.label, galleryMeta)) return
    lightboxSource.value = preview
    resetZoom(); viewer.value?.showModal()
  }
  catch { previewError.value = '无法打开图片预览，请刷新后重试。' }
}
async function stepGallery(delta: -1 | 1) {
  if (!props.gallery || props.gallery.items.length <= 1) return
  const count = props.gallery.items.length
  const nextIndex = (activeGalleryIndex.value + delta + count) % count
  const target = props.gallery.items[nextIndex]
  if (!target) return
  try {
    const preview = await props.reader.media(target.fileId, 'image')
    activeGalleryIndex.value = nextIndex
    lightboxLabel.value = target.label
    lightboxSource.value = preview
    resetZoom()
  } catch {
    previewError.value = '无法读取该张原图，请重试。'
  }
}
async function openVideo(event?: Event) {
  if (busy.value) return
  if (event?.currentTarget instanceof HTMLElement) lastTrigger = event.currentTarget
  previewError.value = ''
  const current = ++videoRequest
  if (!source.value && props.fileId) await load()
  if (current !== videoRequest || !source.value || !videoViewer.value) return
  videoViewer.value.showModal()
  videoOpen.value = true
}
function closeVideo() {
  videoRequest++
  videoViewer.value?.querySelector('video')?.pause()
  videoOpen.value = false
  videoViewer.value?.close()
  lastTrigger?.focus?.()
}
function closeLightbox() { imageRequest++; closePreview(previewOwner); resetZoom(); viewer.value?.close(); closeVideo(); lastTrigger?.focus?.() }
function onWheelZoom(e: WheelEvent) {
  const stage = e.currentTarget as HTMLElement | null
  const factor = e.deltaY < 0 ? 1.15 : 1 / 1.15
  const next = clampZoom(zoomScale.value * factor)
  if (stage) {
    const rect = stage.getBoundingClientRect()
    const cx = e.clientX - rect.left - rect.width / 2
    const cy = e.clientY - rect.top - rect.height / 2
    const ratio = next / zoomScale.value
    zoomTx.value = Math.round((cx - (cx - zoomTx.value) * ratio) * 10) / 10
    zoomTy.value = Math.round((cy - (cy - zoomTy.value) * ratio) * 10) / 10
  }
  zoomScale.value = next
}
function onPointerDown(e: PointerEvent) {
  isDragging.value = true
  dragStartX = e.clientX - zoomTx.value
  dragStartY = e.clientY - zoomTy.value
  ;(e.currentTarget as HTMLElement | null)?.setPointerCapture?.(e.pointerId)
}
function onPointerMove(e: PointerEvent) {
  if (!isDragging.value) return
  zoomTx.value = Math.round(e.clientX - dragStartX)
  zoomTy.value = Math.round(e.clientY - dragStartY)
}
function onPointerUp() { isDragging.value = false }

async function load(automatic = false) {
  const current = ++generation
  source.value = ''; lightboxSource.value = ''; error.value = ''; downloadPath.value = ''; oversizedThumb.value = false; nonImageFallback.value = false; busy.value = true
  try {
    if (!props.fileId) return
    if (props.kind === 'file') {
      const read = await props.reader.download(props.fileId)
      if (current === generation) downloadPath.value = read.content_path
    } else {
      if (props.kind === 'image' && typeof props.reader.thumbnail === 'function') {
        try {
          const thumb = await props.reader.thumbnail(props.fileId, props.edge === 640 || !props.thumbnail ? 640 : 320, automatic ? () => visible.value : undefined)
          if (current === generation) { source.value = thumb; return }
        } catch (cause) {
          if (current !== generation || (cause as Error)?.name === 'AbortError') return
          const status = (cause as { status?: number })?.status
          if (status === 415) { nonImageFallback.value = true; return }
          if (status === 422 || status === 413) { oversizedThumb.value = true; return }
          throw cause
        }
      }
      const result = await props.reader.media(props.fileId, props.kind)
      if (current === generation) source.value = result
    }
  } catch (cause) { if (current === generation) error.value = readerError(cause) }
  finally { if (current === generation) busy.value = false }
}
async function prepareDownload() {
  if (!props.fileId) return
  const current = generation
  try { const read = await props.reader.download(props.fileId); if (current === generation) downloadPath.value = read.content_path }
  catch (cause) { if (current === generation) error.value = readerError(cause) }
}
watch(() => [props.reader, props.fileId, props.kind, props.thumbnail, props.edge], () => {
  generation++; posterSource.value = ''; posterBusy.value = false; posterFailed.value = false; source.value = ''; lightboxSource.value = ''; error.value = ''; downloadPath.value = ''; oversizedThumb.value = false; nonImageFallback.value = false; busy.value = false
  closeLightbox()
}, { immediate: true })
watch(() => [visible.value, props.reader, props.fileId, props.kind, props.thumbnail, props.edge, busy.value, posterBusy.value], () => {
  if (visible.value && props.kind === 'image' && props.fileId && !source.value && !busy.value && !error.value && !oversizedThumb.value && !nonImageFallback.value) void load(true)
  if (visible.value && props.kind === 'video' && props.thumbnail && props.fileId && !posterSource.value && !posterBusy.value && !posterFailed.value && typeof props.reader.poster === 'function') {
    const current = generation
    posterBusy.value = true
    void props.reader.poster(props.fileId, props.edge, () => visible.value && current === generation)
      .then(url => { if (current === generation) posterSource.value = url })
      .catch(cause => { if (current === generation && cause?.name !== 'AbortError') posterFailed.value = true })
      .finally(() => { if (current === generation) posterBusy.value = false })
  }
}, { flush: 'post' })
watch(visible, value => { if (!value) closeLightbox() })
onBeforeUnmount(() => { generation++; closeLightbox() })
</script>

<template>
  <div ref="root" class="project-media" :class="[kind, { thumbnail, contain }]" :aria-label="label">
    <p v-if="previewError" role="alert">{{ previewError }}</p>
    <button v-if="kind === 'video' && thumbnail && fileId" type="button" class="video-open" :aria-label="`播放${label}`" :disabled="busy" @click="openVideo($event)">
      <span v-if="busy" role="status">正在读取…</span>
      <span v-else-if="error" role="alert">{{ error }}</span>
      <template v-else><img v-if="posterSource" :src="posterSource" alt="" class="video-poster"><span class="video-open-badge">▶ 播放视频</span></template>
    </button>
    <button v-else-if="kind === 'image' && oversizedThumb && fileId" type="button" class="image-open oversized-thumb" :aria-label="`放大${label}`" @click="openLightbox($event)">
      <span>原图较大 · 点击查看</span>
    </button>
    <div v-else-if="kind === 'image' && nonImageFallback" class="media-state thumb-frame">
      <span>{{ label }}</span>
    </div>
    <template v-else-if="source">
      <button v-if="kind === 'image' && thumbnail" type="button" class="image-open" :aria-label="`放大${label}`" @click="openLightbox($event)"><img :src="source" :alt="label" loading="lazy" :style="objectPosition ? { objectPosition: `${objectPosition.x}% ${objectPosition.y}%` } : undefined" @error="error = '图片解码失败。'; source = ''"></button>
      <button v-else-if="kind === 'image'" type="button" class="image-open" :aria-label="`放大${label}`" @click="openLightbox($event)"><img :src="source" :alt="label" @error="error = '图片解码失败。'; source = ''"></button>
      <audio v-else-if="kind === 'audio'" :src="source" controls preload="metadata" :aria-label="label" @error="error = '浏览器无法播放此音频。'; source = ''" />
      <video v-else :src="source" controls playsinline preload="metadata" :aria-label="label" @error="error = '浏览器无法播放此视频。'; source = ''" />
    </template>
    <div v-else class="media-state" :class="{ 'thumb-frame': thumbnail && kind === 'file' }">
      <span v-if="!fileId">{{ kind === 'image' ? '未添加图片' : kind === 'audio' ? '未添加音频' : kind === 'video' ? '未添加视频' : '未添加文件' }}</span>
      <span v-else-if="busy" role="status">正在读取…</span>
      <template v-else-if="error"><span role="alert">{{ error }}</span><button class="btn" @click.stop.prevent="load()">重试</button><button v-if="!thumbnail && !downloadPath" class="btn" @click="prepareDownload">下载查看</button></template>
      <template v-else-if="kind === 'file' && matchedSetPackage">
        <button type="button" class="btn primary" :disabled="viewerBusy" @click="openInSetViewer">在片场查看器中打开</button>
        <button v-if="!downloadPath" type="button" class="btn" @click="load()">准备下载 {{ label }}</button>
        <a v-if="downloadPath" class="btn" :href="downloadPath" download>下载 {{ label }}</a>
        <div v-if="viewerState === 'not_installed'" class="set-viewer-fallback" role="status">
          <strong>当前工作站尚未安装「片场查看器 (viewer)」</strong>
          <span>安装后可在浏览器内直接环绕、第一人称漫游与核对 Blender 实拍机位；您仍可直接下载片场包文件。</span>
          <button type="button" class="btn" @click="openViewerMarketplace">前往应用中心安装</button>
        </div>
        <p v-else-if="viewerState === 'forbidden'" class="set-viewer-fallback" role="alert">当前会话无权打开片场查看器或读取此固定修订。</p>
      </template>
      <button v-else-if="!downloadPath" class="btn" @click="load()">{{ kind === 'file' ? `准备下载 ${label}` : kind === 'image' ? '加载预览' : kind === 'audio' ? '播放音频' : '播放视频' }}</button>
      <a v-if="downloadPath" class="btn" :href="downloadPath" download>{{ kind === 'file' ? `下载 ${label}` : '下载文件' }}</a>
    </div>
    <dialog v-if="kind === 'video' && thumbnail" ref="videoViewer" class="lightbox-dialog video-dialog" :aria-label="label" @click="($event.target === videoViewer) && closeVideo()" @cancel.prevent="closeVideo()" @close="videoOpen = false">
      <div class="zoom-toolbar"><span>{{ label }}</span><button class="btn close" autofocus aria-label="关闭视频" @click="closeVideo()">关闭</button></div>
      <video v-if="videoOpen && source" :src="source" controls autoplay playsinline :aria-label="label" @error="error = '浏览器无法播放此视频。'; source = ''" />
    </dialog>
    <dialog v-if="kind === 'image'" ref="viewer" class="lightbox-dialog" :aria-label="lightboxLabel || label" @click="($event.target === viewer) && closeLightbox()">
      <div class="zoom-toolbar">
        <div class="zoom-controls">
          <button type="button" class="btn zoom-btn" aria-label="缩小图片" @click="stepZoom(-0.25)">-</button>
          <span class="zoom-readout tabular-nums">{{ Math.round(zoomScale * 100) }}%</span>
          <button type="button" class="btn zoom-btn" aria-label="放大图片" @click="stepZoom(0.25)">+</button>
          <button type="button" class="btn zoom-btn" aria-label="1:1 原始比例" @click="resetZoom()">1:1</button>
          <template v-if="hasGallery">
            <button type="button" class="btn zoom-btn" aria-label="上一张" @click="stepGallery(-1)">上一张</button>
            <span class="zoom-readout tabular-nums">{{ activeGalleryIndex + 1 }} / {{ gallery!.items.length }}</span>
            <button type="button" class="btn zoom-btn" aria-label="下一张" @click="stepGallery(1)">下一张</button>
          </template>
        </div>
        <button class="btn close" autofocus aria-label="关闭图片" @click="closeLightbox()">关闭</button>
      </div>
      <div class="zoom-stage" :class="{ dragging: isDragging }" @wheel.prevent="onWheelZoom" @pointerdown="onPointerDown" @pointermove="onPointerMove" @pointerup="onPointerUp" @pointercancel="onPointerUp">
        <img :src="(lightboxSource || source) || undefined" :alt="lightboxLabel || label" :style="{ transform: `translate3d(${zoomTx}px, ${zoomTy}px, 0) scale(${zoomScale})` }">
      </div>
    </dialog>
  </div>
</template>

<style scoped>
.project-media { min-width: 0; }
.image, .video { border-radius: 12px; background: var(--subtle); overflow: hidden; border: 0.5px solid var(--vf-hairline-outer, var(--line)); }
.image-open, .video-open { position: relative; display: grid; place-items: center; width: 100%; height: 100%; min-height: 0; padding: 0; border: 0; background: transparent; color: inherit; cursor: zoom-in; }
.oversized-thumb { min-height: 120px; padding: 16px; color: var(--muted); font-size: 13px; }
.video-poster { width: 100%; max-height: 100%; object-fit: contain; }
.video-open { cursor: pointer; padding: 16px; color: var(--muted); }
.video-open-badge { font-size: 13px; }
.video-dialog video { width: min(92vw, 1280px); max-height: 80vh; object-fit: contain; background: #111; }
img { display: block; width: 100%; height: auto; }
.thumbnail { aspect-ratio: 16 / 10; }
.thumbnail img { width: 100%; height: 100%; object-fit: cover; object-position: center top; transition: transform 280ms var(--vf-ease-spring, cubic-bezier(0.22, 1, 0.36, 1)); }
.thumbnail.contain { aspect-ratio: 4 / 3; }
.thumbnail.contain img { object-fit: contain; object-position: center; padding: 0; background: var(--subtle); }
.thumb-frame { aspect-ratio: 4 / 3; width: 100%; height: 100%; min-height: 0; max-height: 100%; }
.video .thumb-frame { min-height: 0; height: 100%; }
.media-state { min-height: 72px; display: flex; align-items: center; justify-content: center; gap: 10px; flex-wrap: wrap; padding: 16px; color: var(--muted); overflow-wrap: anywhere; }
.image .media-state, .video .media-state { min-height: 180px; height: 100%; }
.waveform-strip { display: flex; align-items: center; gap: 2px; height: 28px; padding: 4px 10px; border-radius: 8px; background: var(--subtle); border: 0.5px solid var(--vf-hairline-outer, var(--line)); margin-bottom: 6px; }
.waveform-bar { flex: 1; min-width: 2px; border-radius: 99px; background: var(--accent); opacity: 0.72; transition: opacity 120ms ease; }
.waveform-strip:hover .waveform-bar { opacity: 0.95; }
audio, video { display: block; width: 100%; }
video { max-height: 70vh; }
dialog { margin: auto; border: 0.5px solid var(--vf-hairline-outer, var(--line)); background: var(--panel); color: var(--ink); border-radius: 12px; max-width: 95vw; max-height: 95vh; padding: 12px; box-shadow: var(--vf-shadow-card-hover); }
dialog::backdrop { background: var(--aside); }
.zoom-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-bottom: 10px; flex-wrap: wrap; }
.zoom-controls { display: flex; align-items: center; gap: 6px; }
.zoom-readout { font-variant-numeric: tabular-nums; font-size: 12px; min-width: 46px; text-align: center; color: var(--accent); font-weight: 600; }
.zoom-stage { position: relative; overflow: hidden; display: grid; place-items: center; min-width: min(72vw, 760px); max-height: 80vh; border-radius: 10px; background: var(--subtle); border: 0.5px solid var(--vf-hairline-outer, var(--line)); cursor: grab; user-select: none; }
.zoom-stage.dragging { cursor: grabbing; }
.zoom-stage img { width: auto; max-width: 90vw; max-height: 76vh; object-fit: contain; transform-origin: center center; will-change: transform; pointer-events: none; }
.close { margin-left: auto; }
</style>
