<script setup lang="ts">
import { computed, onBeforeUnmount, onServerPrefetch, ref, watch } from 'vue'
import { ProjectReader, readerError } from '../platform/project-reader'
import { previewImage, closePreview } from '../platform/media-preview'
export interface MediaGallery { id: string; items: { fileId: string; label: string }[]; index: number }
const props = withDefaults(defineProps<{ reader: ProjectReader; fileId?: string | null; kind: 'image' | 'audio' | 'video' | 'file'; label: string; thumbnail?: boolean; contain?: boolean; edge?: 320 | 640; objectPosition?: { x: number; y: number }; gallery?: MediaGallery }>(), { thumbnail: false, contain: false, edge: 320 })
const source = ref(''), lightboxSource = ref(''), lightboxLabel = ref(''), error = ref(''), busy = ref(false), downloadPath = ref('')
const oversizedThumb = ref(false), nonImageFallback = ref(false), activeGalleryIndex = ref(0)
const viewer = ref<HTMLDialogElement | null>(null)
const videoViewer = ref<HTMLDialogElement | null>(null)
const zoomScale = ref(1), zoomTx = ref(0), zoomTy = ref(0), isDragging = ref(false)
let dragStartX = 0, dragStartY = 0
let generation = 0
let lastTrigger: HTMLElement | null = null
const previewOwner = {}, previewError = ref('')

const waveformBars = computed(() => {
  const seed = `${props.fileId || ''}:${props.label || 'audio'}`
  let hash = 2166136261
  for (let i = 0; i < seed.length; i++) hash = Math.imul(hash ^ seed.charCodeAt(i), 16777619)
  const bars: number[] = []
  for (let i = 0; i < 48; i++) {
    hash = Math.imul(hash ^ (i + 17), 16777619)
    const norm = 0.15 + ((Math.abs(hash) % 85) / 100)
    bars.push(Math.max(4, Math.round(Math.pow(norm, 0.7) * 24)))
  }
  return bars
})

const hasGallery = computed(() => (props.gallery?.items.length || 0) > 1)

function clampZoom(next: number) { return Math.max(0.5, Math.min(5, Math.round(next * 100) / 100)) }
function resetZoom() { zoomScale.value = 1; zoomTx.value = 0; zoomTy.value = 0; isDragging.value = false }
function stepZoom(delta: number) { zoomScale.value = clampZoom(zoomScale.value + delta) }
async function openLightbox(event?: Event) {
  if (event?.currentTarget instanceof HTMLElement) lastTrigger = event.currentTarget
  previewError.value = ''
  activeGalleryIndex.value = props.gallery?.index ?? 0
  lightboxLabel.value = props.label
  try {
    const preview = (props.thumbnail || oversizedThumb.value) && props.fileId ? await props.reader.media(props.fileId, 'image') : source.value
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
  if (event?.currentTarget instanceof HTMLElement) lastTrigger = event.currentTarget
  previewError.value = ''
  if (!source.value && props.fileId) await load()
  if (source.value) videoViewer.value?.showModal()
}
function closeVideo() {
  videoViewer.value?.querySelector('video')?.pause()
  videoViewer.value?.close()
  lastTrigger?.focus?.()
}
function closeLightbox() { closePreview(previewOwner); resetZoom(); viewer.value?.close(); closeVideo(); lastTrigger?.focus?.() }
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

async function load() {
  const current = ++generation
  source.value = ''; lightboxSource.value = ''; error.value = ''; downloadPath.value = ''; oversizedThumb.value = false; nonImageFallback.value = false; busy.value = true
  try {
    if (!props.fileId) return
    if (props.kind === 'file') {
      const read = await props.reader.download(props.fileId)
      if (current === generation) downloadPath.value = read.content_path
    } else {
      if (props.thumbnail && props.kind === 'image' && typeof props.reader.thumbnail === 'function') {
        try {
          const thumb = await props.reader.thumbnail(props.fileId, props.edge === 640 ? 640 : 320)
          if (current === generation) { source.value = thumb; return }
        } catch (thumbErr) {
          if (current !== generation) return
          const status = (thumbErr as { status?: number } | null)?.status
          if (status === 415) { nonImageFallback.value = true; return }
          if (status === 422) { oversizedThumb.value = true; return }
          const read = await props.reader.download(props.fileId)
          if (current !== generation) return
          if (!read.version.mime.toLowerCase().startsWith('image/')) { nonImageFallback.value = true; return }
          if (read.version.size > 8 * 1024 * 1024) { oversizedThumb.value = true; return }
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
let activeLoad: Promise<void> | null = null
watch(() => [props.reader, props.fileId, props.kind, props.thumbnail], () => {
  generation++; source.value = ''; lightboxSource.value = ''; error.value = ''; downloadPath.value = ''; oversizedThumb.value = false; nonImageFallback.value = false; busy.value = false
  closeLightbox()
  if ((props.kind === 'image' || (props.kind === 'video' && props.thumbnail)) && props.fileId) activeLoad = load()
  else activeLoad = null
}, { immediate: true })
onServerPrefetch(async () => { if (typeof props.reader.thumbnail === 'function' && activeLoad) await activeLoad })
onBeforeUnmount(() => { generation++; closeLightbox() })
</script>

<template>
  <div class="project-media" :class="[kind, { thumbnail, contain }]" :aria-label="label">
    <p v-if="previewError" role="alert">{{ previewError }}</p>
    <div v-if="kind === 'audio' && (source || fileId)" class="waveform-strip" data-gamma="0.7" aria-hidden="true">
      <span v-for="(barHeight, idx) in waveformBars" :key="idx" class="waveform-bar" :style="{ height: `${barHeight}px` }" />
    </div>
    <button v-if="kind === 'video' && thumbnail && fileId" type="button" class="video-open" :class="{ loaded: !!source }" :aria-label="`放大${label}`" :disabled="busy" @click="openVideo($event)">
      <video v-if="source" class="thumb-video" :src="source" muted playsinline preload="auto" :aria-label="label" @loadedmetadata="($event.target as HTMLVideoElement).currentTime = 0.001" @error="error = '浏览器无法播放此视频。'; source = ''" />
      <span v-else-if="busy" role="status">正在读取…</span>
      <span v-else-if="error" role="alert">{{ error }}</span>
      <span class="video-open-badge">放大查看</span>
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
      <template v-else-if="error"><span role="alert">{{ error }}</span><button class="btn" @click.stop.prevent="load">重试</button><button v-if="!thumbnail && !downloadPath" class="btn" @click="prepareDownload">下载查看</button></template>
      <button v-else-if="!downloadPath" class="btn" @click="load">{{ kind === 'file' ? `准备下载 ${label}` : kind === 'audio' ? '加载音频' : '加载视频' }}</button>
      <a v-if="downloadPath" class="btn" :href="downloadPath" download>{{ kind === 'file' ? `下载 ${label}` : '下载文件' }}</a>
    </div>
    <dialog v-if="kind === 'video' && thumbnail" ref="videoViewer" class="lightbox-dialog video-dialog" :aria-label="label" @click="($event.target === videoViewer) && closeVideo()">
      <div class="zoom-toolbar"><span>{{ label }}</span><button class="btn close" autofocus aria-label="关闭视频" @click="closeVideo()">关闭</button></div>
      <video v-if="source" :src="source" controls autoplay playsinline :aria-label="label" @error="error = '浏览器无法播放此视频。'; source = ''" />
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
.video-open .thumb-video { pointer-events: none; }
.video-open.loaded .video-open-badge { position: absolute; right: 8px; bottom: 8px; padding: 3px 8px; border-radius: 6px; background: rgba(15, 20, 18, 0.72); color: #fff; font-size: 12px; line-height: 1.4; pointer-events: none; }
.video-dialog video { width: min(92vw, 1280px); max-height: 80vh; object-fit: contain; background: #111; }
img { display: block; width: 100%; height: auto; }
.thumbnail { aspect-ratio: 16 / 10; }
.thumbnail img { width: 100%; height: 100%; object-fit: cover; object-position: center top; transition: transform 280ms var(--vf-ease-spring, cubic-bezier(0.22, 1, 0.36, 1)); }
.thumbnail.contain { aspect-ratio: 4 / 3; }
.thumbnail.contain img, .thumb-video { object-fit: contain; object-position: center; padding: 0; background: var(--subtle); }
.thumb-frame, .thumb-video { aspect-ratio: 4 / 3; width: 100%; height: 100%; min-height: 0; max-height: 100%; }
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
