<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter, type RouteLocationRaw } from 'vue-router'
import { ProjectReader, projectContentView, type DramaGlobal } from '../../platform/project-reader'
import { dramaText } from '../../platform/drama-presentation'

const props = defineProps<{
  reader: ProjectReader
  q: (key: string) => string
  link: (change: Record<string, string | undefined>) => RouteLocationRaw
  materialLink: (g: DramaGlobal, version?: string, state?: string) => RouteLocationRaw
  shotLink: (sceneKey: string, shotKey: string, tab?: string) => RouteLocationRaw
}>()

const router = useRouter()
const project = computed(() => props.reader.project!)
const drama = computed(() => projectContentView(project.value, props.reader.content)!.ai_short_drama!)
const episode = computed(() => props.q('episode') ? drama.value.episodes.find(e => e.key === props.q('episode')) : drama.value.episodes[0])
const episodeShots = computed(() => episode.value?.scenes.flatMap(s => s.shots) || [])
const matches = (text: string) => !props.q('find') || text.toLocaleLowerCase().includes(props.q('find').toLocaleLowerCase())
const locations = computed(() => drama.value.globals.filter(g => g.kind === 'location' && matches(g.name + ' ' + (g.description || ''))))
const human = (text?: string | null) => dramaText(text, drama.value).replace(/\*\*/g, '').replace(/^\s*\||\|\s*$/g, '').replace(/\s*\|\s*/g, ' · ').trim()

const searchDraft = ref(props.q('find'))
watch(() => props.q('find'), value => { searchDraft.value = value })
function search() { void router.replace(props.link({ find: searchDraft.value || undefined })) }

function shotMatches(sh: { title: string; overview?: string | null; versions: { purpose?: string | null }[] }) {
  return matches(sh.title + ' ' + human(sh.overview))
    && (props.q('status') !== 'ready' || sh.versions.length > 0)
    && (props.q('status') !== 'missing' || !sh.versions.some(v => v.purpose === 'whitebox'))
}
</script>

<template>
  <header class="section-head">
    <div>
      <span class="muted">集 → 场 → 镜头</span>
      <h2>{{ q('view') === 'spaces' ? '场景白模与灯光' : '镜头制作' }}</h2>
    </div>
    <RouterLink v-if="q('view') !== 'spaces'" class="related-link" :to="link({ view: 'spaces', find: undefined })">场景白模与灯光</RouterLink>
  </header>

  <template v-if="q('view') === 'spaces'">
    <p class="muted">场景状态、灯光与白模按固定版本关联；未登记的工程和预览保留待制作。</p>
    <div class="production-grid">
      <RouterLink v-for="g in locations" :key="g.key" class="production-card" :to="materialLink(g)">
        <span class="muted">场景空间</span>
        <h3>{{ g.name }}</h3>
        <span class="badge">{{ g.versions.some(v => v.purpose === 'whitebox') ? '已有白模版本' : '白模待补充' }}</span>
        <p>{{ g.states.length }} 种场景状态 · {{ g.states.map(s => s.name).join('、') }}</p>
        <span class="read-more">查看场景状态与灯光 →</span>
      </RouterLink>
    </div>
  </template>

  <template v-else>
    <div class="material-toolbar production-toolbar">
      <label>
        分集
        <select :value="episode?.key" @change="router.replace(link({ episode: ($event.target as HTMLSelectElement).value, scene: undefined, shot: undefined }))">
          <option v-for="e in drama.episodes" :key="e.key" :value="e.key">{{ e.title }}</option>
        </select>
      </label>
      <label>
        制作筛选
        <select :value="q('status') || 'all'" @change="router.replace(link({ status: ($event.target as HTMLSelectElement).value }))">
          <option value="all">全部镜头</option>
          <option value="missing">白模待补充</option>
          <option value="ready">已有制作成果</option>
        </select>
      </label>
      <form class="search" @submit.prevent="search">
        <input v-model="searchDraft" type="search" aria-label="搜索镜头" placeholder="搜索镜头或动作">
        <button class="btn" type="submit">搜索</button>
      </form>
    </div>

    <p v-if="episode" class="muted">{{ episode.title }} · {{ episodeShots.length }} 镜。场景、镜头及复杂道具白模须核对空间与灯光。</p>

    <template v-for="s in episode?.scenes" :key="s.key">
      <template v-if="s.shots.some(shotMatches)">
        <h3 class="scene-title">{{ s.title }}</h3>
        <div class="production-grid">
          <RouterLink v-for="sh in s.shots.filter(shotMatches)" :key="sh.key" class="production-card" :to="shotLink(s.key, sh.key)">
            <span class="muted">{{ sh.duration_seconds == null ? '时长待定' : sh.duration_seconds + ' 秒预算' }}</span>
            <h3>{{ sh.title }}</h3>
            <span class="badge">{{ sh.versions.length ? sh.versions.length + ' 个制作版本' : '制作待开始' }}</span>
            <p>{{ human(sh.overview?.match(/\*\*时空：\*\*([^\n]+)/)?.[1]) }}</p>
            <span class="read-more">查看制作依据与缺项 →</span>
          </RouterLink>
        </div>
      </template>
    </template>

    <p v-if="!episodeShots.some(shotMatches)" class="empty">暂无匹配镜头，请调整分集或筛选条件。</p>
  </template>
</template>

<style scoped>
h2 { font-size: 18px; line-height: 1.35; margin: 0 0 10px; }
h3 { font-size: 15px; margin: 8px 0; }
.muted { color: var(--muted); font-size: 12px; line-height: 1.85; }
.empty { color: var(--muted); padding: 0; border: 0; font-size: 13px; line-height: 1.8; }
.section-head { margin: 0 0 16px; display: flex; align-items: start; justify-content: space-between; gap: 20px; }
.section-head h2 { margin: 3px 0 0; }
.related-link { display: block; color: var(--accent); font-size: 13px; line-height: 1.7; padding: 6px 0; }
.read-more { display: block; margin-top: 12px; color: var(--accent); font-size: 12px; }
.material-toolbar { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; margin: 18px 0; }
.material-toolbar label { display: grid; gap: 6px; font-size: 12px; color: var(--muted); }
.production-toolbar > label, .production-toolbar > .search { flex: 1; min-width: 180px; }
.search { display: flex; gap: 8px; flex: 1; min-width: 180px; }
.search input { flex: 1; min-width: 0; }
.search .btn { white-space: nowrap; }
.btn { justify-self: start; font-size: 14px; border-radius: 8px; padding: 8px 12px; min-height: 35px; }
input, select { border: 1px solid var(--line); background: var(--panel); color: var(--ink); padding: 9px 11px; border-radius: 6px; max-width: 100%; font: inherit; font-size: 13px; }
.production-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; margin: 16px 0 22px; }
.production-card { display: block; min-width: 0; border: 1px solid var(--line); border-radius: 8px; background: var(--shell-panel); padding: 20px; }
.production-card:hover { border-color: var(--accent); }
.production-card h3 { font-size: 15px; margin: 10px 0; }
.production-card p { font-size: 13px; color: var(--muted); line-height: 1.85; margin: 14px 0; }
.scene-title { font-size: 15px; margin: 18px 0 10px; }
.badge { display: inline-block; border: 1px solid var(--line); border-radius: 4px; padding: 2px 7px; color: var(--muted); font-size: 11px; }
@media (max-width: 800px) {
  .production-grid { grid-template-columns: minmax(0, 1fr); }
  .section-head { flex-wrap: wrap; }
  .material-toolbar { align-items: stretch; }
  .material-toolbar label, .material-toolbar select { width: 100%; }
}
</style>
