<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter, type RouteLocationRaw } from 'vue-router'
import { useHost } from '../../platform/host'
import { reviewPath } from '../../platform/set-review'
import { spaceReviewTarget } from '../../platform/drama-set-review'
import { rememberReviewContext, reviewContextKey } from '../../platform/set-review-memory'
import { ProjectReader, projectContentView, type DramaGlobal, type DramaScene, type DramaShot } from '../../platform/project-reader'
import { dramaText, materialPreview, shotPreview, shotSummary, type ShotPreview } from '../../platform/drama-presentation'
import ProjectMedia, { type MediaGallery } from '../ProjectMedia.vue'

const props = defineProps<{
  reader: ProjectReader
  q: (key: string) => string
  link: (change: Record<string, string | undefined>) => RouteLocationRaw
  materialLink: (g: DramaGlobal, version?: string, state?: string) => RouteLocationRaw
  shotLink: (sceneKey: string, shotKey: string, tab?: string) => RouteLocationRaw
}>()

const router = useRouter(), route = useRoute(), { appSDK, dashboardHost } = useHost()
const reviewVersions = ref<Record<string, string>>({})
const reviewTarget = (g: DramaGlobal) => spaceReviewTarget(project.value.project_id, project.value.revision_id, g, reviewVersions.value[g.key])
function openSetReview(g: DramaGlobal) {
  const target = reviewTarget(g)
  if (!target) return
  rememberReviewContext(reviewContextKey(appSDK.context.get(), project.value.project_id), route.fullPath, dashboardHost.getScroll(), undefined, undefined, project.value.manifest.name, g.name)
  void router.push(reviewPath(target))
}
const project = computed(() => props.reader.project!)
const drama = computed(() => projectContentView(project.value, props.reader.content)!.ai_short_drama!)
const episode = computed(() => props.q('episode') ? drama.value.episodes.find(e => e.key === props.q('episode')) : drama.value.episodes[0])
const episodeShots = computed(() => episode.value?.scenes.flatMap(s => s.shots) || [])
const activeScene = computed(() => props.q('scene') ? episode.value?.scenes.find(s => s.key === props.q('scene')) : undefined)
const scopedScenes = computed(() => props.q('scene') ? (activeScene.value ? [activeScene.value] : []) : (episode.value?.scenes || []))
const scopedShots = computed(() => scopedScenes.value.flatMap(s => s.shots))
const matches = (text: string) => !props.q('find') || text.toLocaleLowerCase().includes(props.q('find').toLocaleLowerCase())
const locations = computed(() => drama.value.globals.filter(g => g.kind === 'location' && matches(g.name + ' ' + (g.description || ''))))
const human = (text?: string | null) => dramaText(text, drama.value).replace(/\*\*/g, '').replace(/^\s*\||\|\s*$/g, '').replace(/\s*\|\s*/g, ' · ').trim()

function filePath(fileId: string): string | undefined {
  try { return props.reader.file(fileId).path } catch { return project.value.manifest.files.find(f => f.file_id === fileId)?.path }
}

function previewOf(sh: DramaShot): ShotPreview {
  return shotPreview(sh, filePath)
}

const searchDraft = ref(props.q('find'))
const isImeComposing = ref(false)
const activeCardIndex = ref(0)
watch(() => props.q('find'), value => { searchDraft.value = value })

function search() {
  if (isImeComposing.value) return
  void router.replace(props.link({ find: searchDraft.value.trim() || undefined }))
}
function onSearchCompositionEnd() {
  isImeComposing.value = false
}
function onSearchKeydown(e: KeyboardEvent) {
  if (e.isComposing || e.keyCode === 229) return
}

function shotMatches(sh: DramaShot) {
  const preview = previewOf(sh)
  const status = props.q('status') || 'all'
  const textMatch = matches(`${sh.key} ${sh.title} ${human(sh.overview)} ${shotSummary(sh.overview, drama.value)}`)
  if (!textMatch) return false
  if (status === 'ready') return preview.tier === 'ready' || sh.versions.length > 0
  if (status === 'previs') return preview.tier === 'previs'
  if (status === 'missing') return preview.tier === 'missing' && !sh.versions.some(v => v.purpose === 'whitebox')
  return true
}

const visibleScenes = computed(() => scopedScenes.value.filter(s => s.shots.some(shotMatches)))
const flatVisibleShotKeys = computed(() => visibleScenes.value.flatMap(s => s.shots.filter(shotMatches).map(sh => `${s.key}:${sh.key}`)))

function shotFlatIndex(sceneKey: string, shotKey: string): number {
  const idx = flatVisibleShotKeys.value.indexOf(`${sceneKey}:${shotKey}`)
  return idx >= 0 ? idx : 0
}

function shotHudCode(sh: DramaShot): string {
  const m = /^(S\d+|SH\d+)\s+([^\s·]+)/i.exec(sh.title.trim())
  if (m) return `${m[1].toUpperCase()} · ${m[2]}`
  if (/^(s|sh|shot)[_-]?\d+$/i.test(sh.key)) return sh.key.toUpperCase()
  return sh.key.slice(0, 10).toUpperCase()
}

function sceneStats(s: DramaScene): string {
  const totalSeconds = s.shots.reduce((sum, sh) => sum + (sh.duration_seconds || 0), 0)
  const visualCount = s.shots.filter(sh => previewOf(sh).tier !== 'missing').length
  return `${s.shots.length} 镜 · ${totalSeconds} 秒预算 · ${visualCount}/${s.shots.length} 已有画面`
}

function sceneGallery(s: DramaScene, fileId: string): MediaGallery | undefined {
  const items = s.shots
    .map(sh => ({ sh, preview: previewOf(sh) }))
    .filter(entry => entry.preview.kind === 'image' && entry.preview.fileId)
    .map(entry => ({ fileId: entry.preview.fileId!, label: `${s.title} · ${entry.sh.title}` }))
  const index = items.findIndex(item => item.fileId === fileId)
  return index >= 0 && items.length > 1 ? { id: `scene-${s.key}`, items, index } : undefined
}

function onCardHover(event: MouseEvent, playing: boolean) {
  const card = event.currentTarget as HTMLElement | null
  const video = card?.querySelector<HTMLVideoElement>('video.thumb-video')
  if (!video) return
  if (playing) {
    void video.play().catch(() => {})
  } else {
    video.pause()
    try { video.currentTime = 0.001 } catch { /* Ignore unready media */ }
  }
}

function onGridKeydown(e: KeyboardEvent) {
  if (e.isComposing || e.keyCode === 229) return
  if (!['ArrowRight', 'ArrowLeft', 'ArrowDown', 'ArrowUp', 'Home', 'End'].includes(e.key)) return
  const grid = e.currentTarget as HTMLElement | null
  if (!grid) return
  const cards = Array.from(grid.querySelectorAll<HTMLElement>('.roving-item'))
  const current = (e.target as HTMLElement | null)?.closest<HTMLElement>('.roving-item')
  const idx = current ? cards.indexOf(current) : activeCardIndex.value
  if (idx < 0 || !cards.length) return
  e.preventDefault()
  const firstRect = cards[0]?.getBoundingClientRect()
  let cols = 1
  if (firstRect && firstRect.width > 0) {
    cols = Math.max(1, cards.filter(c => Math.abs(c.getBoundingClientRect().top - firstRect.top) < 8).length)
  } else {
    cols = Math.max(1, getComputedStyle(grid).gridTemplateColumns.split(' ').filter(Boolean).length || 3)
  }
  let next = idx
  if (e.key === 'ArrowRight') next = Math.min(cards.length - 1, idx + 1)
  else if (e.key === 'ArrowLeft') next = Math.max(0, idx - 1)
  else if (e.key === 'ArrowDown') next = Math.min(cards.length - 1, idx + cols)
  else if (e.key === 'ArrowUp') next = Math.max(0, idx - cols)
  else if (e.key === 'Home') next = 0
  else if (e.key === 'End') next = cards.length - 1
  activeCardIndex.value = next
  cards[next]?.focus()
}
</script>

<template>
  <template v-if="q('view') === 'spaces'">
    <div class="production-toolbar" role="toolbar" aria-label="场景白模与灯光工具栏">
      <form class="search toolbar-search" @submit.prevent="search">
        <input
          v-model="searchDraft"
          type="search"
          aria-label="搜索场景空间"
          placeholder="搜索场景名称或状态"
          @compositionstart="isImeComposing = true"
          @compositionend="onSearchCompositionEnd"
          @keydown="onSearchKeydown"
        >
        <button class="btn toolbar-btn" type="submit">搜索</button>
      </form>
      <RouterLink class="btn toolbar-btn spaces-btn" :to="link({ view: undefined, find: undefined })">镜头制作分镜墙</RouterLink>
    </div>

    <div class="production-grid shot-wall-grid" data-roving-grid="true" @keydown="onGridKeydown">
      <article v-for="(g, idx) in locations" :key="g.key" class="production-card shot-card">
        <div class="shot-stage">
          <ProjectMedia v-if="materialPreview(g).fileId" :reader="reader" :file-id="materialPreview(g).fileId" kind="image" :label="g.name" thumbnail :edge="640" />
          <RouterLink v-else class="shot-slate" :to="materialLink(g)" tabindex="-1" :aria-label="g.name">
            <span class="slate-code">SPACE · {{ g.key.toUpperCase() }}</span>
            <span class="slate-hint">场景空间白模与灯光设定</span>
          </RouterLink>
          <span class="hud-pill hud-tl">场景空间</span>
          <span class="hud-pill hud-tr tabular-nums" :class="g.versions.some(v => v.purpose === 'whitebox') ? 'tier-ready' : 'tier-previs'">
            <i class="hud-dot" aria-hidden="true" />{{ g.versions.some(v => v.purpose === 'whitebox') ? '已有白模版本' : '白模待补充' }}
          </span>
        </div>
        <RouterLink class="shot-body roving-item" :tabindex="idx === activeCardIndex ? 0 : -1" :to="materialLink(g)" @focus="activeCardIndex = idx">
          <div class="shot-head">
            <h3>{{ g.name }}</h3>
            <span class="shot-segment-count tabular-nums">{{ g.states.length }} 状态</span>
          </div>
          <p class="shot-summary">{{ g.states.length ? `${g.states.length} 种场景状态 · ${g.states.map(s => s.name).join('、')}` : human(g.description) || '查看场景空间、白模工程与灯光状态设定。' }}</p>
        </RouterLink>
        <select v-if="g.versions.length > 1" v-model="reviewVersions[g.key]" :aria-label="g.name + ' 的审阅版本'">
          <option :value="undefined">{{ materialPreview(g).version ? '当前预览版本' : '选择审阅版本' }}</option>
          <option v-for="v in g.versions" :key="v.key" :value="v.key">{{ g.states.find(s => s.key === v.state_key)?.name || '场景状态' }} · {{ v.label }}</option>
        </select>
        <button class="btn" :disabled="!reviewTarget(g)" @click="openSetReview(g)">片场审阅 →</button>
      </article>
    </div>
    <p v-if="!locations.length" class="empty">暂无匹配的场景空间。</p>
  </template>

  <template v-else>
    <div class="production-toolbar" role="toolbar" aria-label="镜头制作筛选工具栏">
      <select
        aria-label="分集"
        class="toolbar-select"
        :value="episode?.key"
        @change="router.replace(link({ episode: ($event.target as HTMLSelectElement).value, scene: undefined, shot: undefined }))"
      >
        <option v-for="e in drama.episodes" :key="e.key" :value="e.key">{{ e.title }}</option>
      </select>

      <select
        aria-label="场次筛选"
        class="toolbar-select"
        :value="q('scene') || ''"
        @change="router.replace(link({ episode: episode?.key, scene: ($event.target as HTMLSelectElement).value || undefined, shot: undefined }))"
      >
        <option value="">全部场次 ({{ episodeShots.length }})</option>
        <option v-for="s in episode?.scenes" :key="s.key" :value="s.key">{{ s.title }} ({{ s.shots.length }})</option>
      </select>

      <select
        aria-label="制作筛选"
        class="toolbar-select"
        :value="q('status') || 'all'"
        @change="router.replace(link({ status: ($event.target as HTMLSelectElement).value === 'all' ? undefined : ($event.target as HTMLSelectElement).value }))"
      >
        <option value="all">全部镜头 ({{ scopedShots.length }})</option>
        <option value="ready">已有制作成果</option>
        <option value="previs">机位预演与白模</option>
        <option value="missing">白模待补充</option>
      </select>

      <form class="search toolbar-search" @submit.prevent="search">
        <input
          v-model="searchDraft"
          type="search"
          aria-label="搜索镜头"
          placeholder="搜索镜号、机位或画面动作"
          @compositionstart="isImeComposing = true"
          @compositionend="onSearchCompositionEnd"
          @keydown="onSearchKeydown"
        >
        <button class="btn toolbar-btn" type="submit">搜索</button>
      </form>

      <RouterLink class="btn toolbar-btn spaces-btn" :to="link({ view: 'spaces', find: undefined })">场景白模与灯光</RouterLink>
    </div>

    <p v-if="q('scene') && !activeScene" class="empty">当前修订没有此场次，请切换全部场次查看。</p>

    <div v-else-if="visibleScenes.length" class="shot-wall" data-roving-grid="true" @keydown="onGridKeydown">
      <section v-for="s in visibleScenes" :key="s.key" class="scene-section">
        <header class="scene-group-head">
          <h3 class="scene-title">{{ s.title }}</h3>
          <span class="scene-stats tabular-nums">{{ sceneStats(s) }}</span>
        </header>

        <div class="production-grid shot-wall-grid">
          <article
            v-for="sh in s.shots.filter(shotMatches)"
            :key="sh.key"
            class="production-card shot-card"
            @mouseenter="onCardHover($event, true)"
            @mouseleave="onCardHover($event, false)"
          >
            <div class="shot-stage">
              <ProjectMedia
                v-if="previewOf(sh).fileId && previewOf(sh).kind"
                :reader="reader"
                :file-id="previewOf(sh).fileId"
                :kind="previewOf(sh).kind!"
                :label="sh.title"
                thumbnail
                contain
                :edge="640"
                :gallery="previewOf(sh).kind === 'image' ? sceneGallery(s, previewOf(sh).fileId!) : undefined"
              />
              <RouterLink v-else class="shot-slate" :to="shotLink(s.key, sh.key)" tabindex="-1" :aria-label="sh.title">
                <svg class="slate-reticle" viewBox="0 0 160 90" aria-hidden="true">
                  <rect x="14" y="12" width="132" height="66" rx="4" fill="none" stroke="currentColor" stroke-width="0.75" stroke-dasharray="4 3" opacity="0.45" />
                  <line x1="80" y1="28" x2="80" y2="62" stroke="currentColor" stroke-width="0.75" opacity="0.4" />
                  <line x1="60" y1="45" x2="100" y2="45" stroke="currentColor" stroke-width="0.75" opacity="0.4" />
                  <circle cx="80" cy="45" r="9" fill="none" stroke="currentColor" stroke-width="0.9" opacity="0.55" />
                </svg>
                <span class="slate-code tabular-nums">{{ shotHudCode(sh) }}</span>
                <span class="slate-hint">{{ previewOf(sh).tier === 'previs' ? '白模工程已登记 · 待渲染画面' : '待生成镜头画面 · 点击查看依据' }}</span>
              </RouterLink>

              <span class="hud-pill hud-tl tabular-nums">{{ shotHudCode(sh) }}</span>
              <span class="hud-pill hud-tr tabular-nums" :class="`tier-${previewOf(sh).tier}`">
                <i class="hud-dot" aria-hidden="true" />{{ previewOf(sh).badge }}
              </span>
              <span class="hud-pill hud-br tabular-nums">{{ sh.duration_seconds == null ? '时长待定' : `${sh.duration_seconds}s` }}</span>
            </div>

            <RouterLink
              class="shot-body roving-item"
              :tabindex="shotFlatIndex(s.key, sh.key) === activeCardIndex ? 0 : -1"
              :to="shotLink(s.key, sh.key)"
              @focus="activeCardIndex = shotFlatIndex(s.key, sh.key)"
            >
              <div class="shot-head">
                <h3>{{ sh.title }}</h3>
                <span v-if="sh.segments.length" class="shot-segment-count tabular-nums">{{ sh.segments.length }} 段</span>
              </div>
              <p class="shot-summary">{{ shotSummary(sh.overview, drama) }}</p>
            </RouterLink>
          </article>
        </div>
      </section>
    </div>

    <p v-else class="empty">暂无匹配镜头，请调整分集、场次或筛选条件。</p>
  </template>
</template>

<style scoped>
.tabular-nums { font-variant-numeric: tabular-nums; }
.empty { color: var(--muted); padding: 24px; border: 0.5px dashed var(--vf-hairline-outer, var(--line)); border-radius: 10px; font-size: 13px; line-height: 1.8; }

.production-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin: 0 0 18px;
}

.toolbar-select,
.toolbar-search input,
.toolbar-btn {
  height: var(--vf-row-comfort, 36px);
  min-height: 36px;
  box-sizing: border-box;
  border: 0.5px solid var(--vf-hairline-outer, var(--line));
  border-radius: 8px;
  background: var(--panel);
  color: var(--ink);
  font: inherit;
  font-size: 13px;
  line-height: 1.2;
  box-shadow: var(--vf-shadow-card);
  transition: border-color 150ms var(--vf-ease-out, cubic-bezier(0.16, 1, 0.3, 1)), background-color 150ms var(--vf-ease-out, cubic-bezier(0.16, 1, 0.3, 1));
}

.toolbar-select {
  flex: 0 0 auto;
  padding: 0 12px;
  max-width: 100%;
  font-variant-numeric: tabular-nums;
}

.toolbar-search {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1 1 220px;
  min-width: 200px;
  margin: 0;
}

.toolbar-search input {
  flex: 1 1 auto;
  min-width: 0;
  width: 100%;
  padding: 0 12px;
}

.toolbar-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0 14px;
  white-space: nowrap;
  cursor: pointer;
  text-decoration: none;
  flex-shrink: 0;
}

.spaces-btn {
  color: var(--accent);
  font-weight: 500;
}

.toolbar-select:hover,
.toolbar-search input:hover,
.toolbar-btn:hover {
  border-color: var(--accent);
}

.shot-wall {
  display: grid;
  gap: 22px;
}

.scene-section {
  min-width: 0;
}

.scene-group-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  padding-bottom: 8px;
  margin-bottom: 12px;
  border-bottom: 0.5px solid var(--vf-hairline-outer, var(--line));
}

.scene-title {
  font-size: 15px;
  font-weight: 600;
  margin: 0;
  color: var(--ink);
}

.scene-stats {
  font-size: 12px;
  color: var(--muted);
}

.shot-wall-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
  align-items: stretch;
}

.shot-card {
  display: flex;
  flex-direction: column;
  min-width: 0;
  border: 0.5px solid var(--vf-hairline-outer, var(--line));
  border-radius: 12px;
  background: var(--shell-panel, var(--panel));
  box-shadow: var(--vf-shadow-card);
  overflow: hidden;
  transition: transform 220ms var(--vf-ease-spring, cubic-bezier(0.22, 1, 0.36, 1)), box-shadow 220ms var(--vf-ease-out, cubic-bezier(0.16, 1, 0.3, 1)), border-color 160ms var(--vf-ease-out, cubic-bezier(0.16, 1, 0.3, 1));
}

.shot-card:hover,
.shot-card:focus-within {
  border-color: var(--accent);
  box-shadow: var(--vf-shadow-card-hover);
  transform: translateY(-2px);
}

.shot-stage {
  position: relative;
  width: 100%;
  aspect-ratio: 16 / 9;
  background: #0d1210;
  overflow: hidden;
  border-bottom: 0.5px solid var(--vf-hairline-inner, var(--line));
}

.shot-stage :deep(.project-media),
.shot-stage :deep(.image-open),
.shot-stage :deep(.video-open),
.shot-stage :deep(.thumb-frame),
.shot-stage :deep(.thumb-video) {
  width: 100%;
  height: 100%;
  aspect-ratio: 16 / 9;
  border-radius: 0;
  border: 0;
  background: #0d1210;
}

.shot-stage :deep(img),
.shot-stage :deep(video.thumb-video) {
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: center;
}

.shot-stage :deep(.media-state) {
  min-height: 0;
  height: 100%;
  padding: 12px;
  font-size: 12px;
  background: linear-gradient(105deg, rgba(255, 255, 255, 0.02) 30%, rgba(46, 169, 124, 0.08) 50%, rgba(255, 255, 255, 0.02) 70%);
}

.shot-slate {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  width: 100%;
  height: 100%;
  padding: 16px;
  box-sizing: border-box;
  text-decoration: none;
  color: rgba(226, 236, 231, 0.72);
  background:
    radial-gradient(circle at 50% 45%, rgba(46, 169, 124, 0.12), rgba(11, 16, 14, 0.94) 75%),
    repeating-linear-gradient(0deg, rgba(255, 255, 255, 0.025) 0 1px, transparent 1px 18px),
    repeating-linear-gradient(90deg, rgba(255, 255, 255, 0.025) 0 1px, transparent 1px 18px);
}

.slate-reticle {
  width: 84px;
  height: 48px;
  color: rgba(164, 198, 182, 0.65);
}

.slate-code {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.06em;
  color: rgba(235, 243, 239, 0.88);
}

.slate-hint {
  font-size: 11px;
  color: rgba(190, 206, 198, 0.62);
}

.hud-pill {
  position: absolute;
  z-index: 2;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 2px 8px;
  border-radius: 6px;
  font-size: 11px;
  line-height: 1.45;
  color: #f2f7f4;
  background: rgba(12, 17, 15, 0.72);
  border: 0.5px solid rgba(255, 255, 255, 0.14);
  backdrop-filter: blur(6px);
  pointer-events: none;
}

.hud-tl { top: 8px; left: 8px; font-weight: 600; }
.hud-tr { top: 8px; right: 8px; }
.hud-br { bottom: 8px; right: 8px; }

.shot-stage :deep(.video-open.loaded .video-open-badge) {
  right: auto;
  left: 8px;
  bottom: 8px;
  font-size: 11px;
  padding: 2px 7px;
}

.hud-dot {
  width: 6px;
  height: 6px;
  border-radius: 99px;
  background: rgba(200, 212, 206, 0.65);
}

.tier-ready .hud-dot {
  background: #2ea97c;
  box-shadow: 0 0 6px rgba(46, 169, 124, 0.75);
}

.tier-previs .hud-dot {
  background: #e2a336;
  box-shadow: 0 0 6px rgba(226, 163, 54, 0.65);
}

.shot-body {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex: 1 1 auto;
  padding: 12px 14px;
  text-decoration: none;
  color: inherit;
  min-width: 0;
}

.shot-body:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: -2px;
}

.shot-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
}

.shot-head h3 {
  font-size: 14px;
  font-weight: 600;
  line-height: 1.35;
  margin: 0;
  color: var(--ink);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.shot-segment-count {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--muted);
}

.shot-summary {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.55;
  color: var(--muted);
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  overflow: hidden;
}

@media (max-width: 800px) {
  .shot-wall-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .toolbar-select {
    flex: 1 1 calc(50% - 8px);
  }
  .toolbar-search {
    flex: 1 1 100%;
  }
  .spaces-btn {
    width: 100%;
  }
}
</style>
