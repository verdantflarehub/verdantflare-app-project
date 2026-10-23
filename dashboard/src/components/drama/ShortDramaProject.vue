<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ProjectReader, projectContentView, type DramaGlobal } from '../../platform/project-reader'
import ProjectMedia from '../ProjectMedia.vue'
import DramaProse from './DramaProse.vue'
import { materialPreview, dramaText } from '../../platform/drama-presentation'
import DramaDocument from './DramaDocument.vue'
import DramaTips from './DramaTips.vue'
import DramaOverview from './DramaOverview.vue'
import DramaMaterialDetail from './DramaMaterialDetail.vue'
import DramaShotDetail from './DramaShotDetail.vue'
import DramaDeliveryView from './DramaDeliveryView.vue'
import DramaProductionView from './DramaProductionView.vue'
import { dramaBreadcrumb, dramaHref, parseDramaPath, rememberDramaListContext, type DramaView } from '../../platform/drama-route.ts'

const props = defineProps<{ reader: ProjectReader; tab: string; hideCrumb?: boolean }>()
const tab = computed(() => ['character', 'wardrobe', 'prop', 'location', 'sound', 'music'].includes(props.tab) ? 'materials' : props.tab === 'continuity' ? 'script' : props.tab)
const route = useRoute(), router = useRouter()
const dramaView = computed(() => parseDramaPath(route.fullPath))

function q(key: string) {
  const view = dramaView.value
  if (view) {
    if (key === 'find') return view.query.q || ''
    if (key === 'status') return view.query.status || ''
    if (key === 'version') return view.query.version || ''
    return String((view as unknown as Record<string, string | undefined>)[key] || '')
  }
  if (key === 'find') return typeof route.query.q === 'string' ? route.query.q : typeof route.query.find === 'string' ? route.query.find as string : ''
  return typeof route.query[key] === 'string' ? route.query[key] as string : ''
}

const searchDraft = ref(q('find'))
watch(() => q('find'), value => { searchDraft.value = value })
function search() { void router.replace(link({ find: searchDraft.value || undefined })) }

const project = computed(() => props.reader.project!)
const drama = computed(() => projectContentView(project.value, props.reader.content)!.ai_short_drama!)
const kinds = [
  { key: 'script', label: '剧本正文' },
  { key: 'character', label: '人物卡片' },
  { key: 'wardrobe', label: '服装造型' },
  { key: 'prop', label: '道具' },
  { key: 'location', label: '场景' },
  { key: 'sound', label: '声音' },
  { key: 'music', label: '配乐' },
]

const material = computed(() => drama.value.globals.find(g => g.key === q('material')))
const state = computed(() => {
  const owner = material.value
  if (!owner) return undefined
  if (q('lighting')) return owner.states.find(s => s.key === q('lighting'))
  if (q('state')) return owner.states.find(s => s.key === q('state'))
  const latest = [...owner.versions].reverse().find(v => !q('state') || v.state_key === q('state'))
  const version = q('version') ? owner.versions.find(v => v.key === q('version')) : latest
  return owner.states.find(s => s.key === (q('state') || version?.state_key)) || owner.states[0]
})

const episode = computed(() => q('episode') ? drama.value.episodes.find(e => e.key === q('episode')) : drama.value.episodes[0])
const scene = computed(() => episode.value?.scenes.find(s => s.key === q('scene')))
const shot = computed(() => scene.value?.shots.find(s => s.key === q('shot')))
const delivery = computed(() => drama.value.deliverables.find(d => d.key === q('deliverable')))

const scriptKinds = [
  { key: 'script', label: '剧本正文' },
  { key: 'character', label: '人物小传' },
  { key: 'location', label: '地点与场次' },
  { key: 'wardrobe', label: '服装造型' },
  { key: 'prop', label: '道具设定' },
  { key: 'continuity', label: '逐镜连续性' },
  { key: 'sound', label: '声音' },
  { key: 'music', label: '配乐' },
]

const activeKind = computed(() => material.value?.kind || (['character', 'wardrobe', 'prop', 'location', 'continuity', 'sound', 'music'].includes(props.tab) ? props.tab : q('kind')) || (tab.value === 'script' ? 'script' : 'character'))
const rail = computed(() => tab.value === 'script' ? scriptKinds : kinds.filter(k => k.key !== 'script'))
const railTitle = computed(() => tab.value === 'script' ? '剧本与设定' : '素材设定')
const showRail = computed(() => false)
const activeName = computed(() => rail.value.find(k => k.key === activeKind.value)?.label || '设定')
const matches = (text: string) => !q('find') || text.toLocaleLowerCase().includes(q('find').toLocaleLowerCase())
const filtered = computed(() => drama.value.globals.filter(g => g.kind === activeKind.value && matches(g.name + ' ' + (g.description || '')) && (q('status') !== 'ready' || g.versions.length || g.file_ids.length) && (q('status') !== 'missing' || (!g.versions.length && !g.file_ids.length))))
const continuityFiles = computed(() => new Set(drama.value.continuity.flatMap(c => [...c.reference_file_ids, ...c.definition_refs.map(r => r.file_id)])))
const continuityEntries = computed(() => drama.value.episodes.flatMap(e => e.scenes.flatMap(s => s.shots.map(sh => ({ key: sh.key, title: sh.title, episode: e, scene: s, refs: sh.definition_refs.filter(r => continuityFiles.value.has(r.file_id) && r.anchor) })).filter(x => x.refs.length))))
const continuityEntry = computed(() => continuityEntries.value.find(e => e.key === q('continuity')))
const episodeShots = computed(() => episode.value?.scenes.flatMap(s => s.shots) || [])
const human = (text?: string | null) => dramaText(text, drama.value).replace(/\*\*/g, '').replace(/^\s*\||\|\s*$/g, '').replace(/\s*\|\s*/g, ' · ').trim()

const episodeCharacters = computed(() => {
  const used = new Set(drama.value.usages.filter(u => u.target.episode_key === episode.value?.key).map(u => u.global_key))
  for (const g of drama.value.globals) if (used.has(g.key)) for (const key of g.input_global_keys) used.add(key)
  return drama.value.globals.filter(g => g.kind === 'character' && used.has(g.key))
})

function cardStatus(g: DramaGlobal) {
  const children = drama.value.globals.filter(child => child.kind === g.kind && child.input_global_keys.includes(g.key))
  if (!g.versions.length && !g.file_ids.length && children.length) return `素材组 · ${children.length} 项独立素材`
  return g.versions.length
    ? `${g.versions.length} 个制作版本 · ${g.selected_versions.length ? '已登记项目选用' : '尚未登记正式选用'}`
    : g.file_ids.length
      ? '已有成果 · 版本关系待整理'
      : (project.value.head_revision_id && project.value.head_revision_id !== project.value.revision_id ? '此历史版本尚未登记制作素材' : '已有文字依据 · 待制作')
}

function previewStatus(g: DramaGlobal) {
  const preview = materialPreview(g), v = preview.version
  return v ? `${g.states.find(s => s.key === v.state_key)?.name || '状态未整理'} · ${v.label} · ${{ unreviewed: '未审核', in_review: '审核中', approved: '此版已有审核记录', rejected: '需修改' }[v.review.state]} · ${preview.selected ? '项目选用' : '未选用'}` : ''
}

function crumbChange(target: DramaView): Record<string, string | undefined> {
  return {
    tab: target.tab,
    kind: target.kind,
    material: target.material,
    state: target.state,
    episode: target.episode,
    scene: target.scene,
    shot: target.shot,
    deliverable: target.deliverable,
    continuity: target.continuity,
    lighting: target.lighting,
    package: target.package,
    view: target.view,
    version: target.query.version,
    revision: target.query.revision,
    find: target.query.q,
    status: target.query.status,
  }
}

function categoryLink(kind: string) {
  return link({ tab: kind === 'continuity' ? 'continuity' : kind === 'script' ? 'script' : kind, kind, material: undefined, state: undefined, version: undefined, compare: undefined, scene: undefined, shot: undefined, find: undefined, status: undefined, continuity: undefined, returnMaterial: undefined })
}

function shotLink(sceneKey: string, shotKey: string, nextTab = props.tab) {
  return link({ tab: nextTab, episode: episode.value?.key, scene: sceneKey, shot: shotKey, material: undefined, state: undefined, version: undefined, compare: undefined })
}

function currentView(): DramaView {
  const parsed = dramaView.value
  if (parsed?.projectId) return parsed
  const projectId = project.value.project_id
  const kind = material.value?.kind || q('kind') || (['character', 'wardrobe', 'prop', 'location', 'sound', 'music', 'continuity', 'script'].includes(props.tab) ? props.tab : undefined)
  return {
    projectId,
    tab: props.tab || 'overview',
    kind,
    material: q('material') || undefined,
    state: q('state') || undefined,
    episode: q('episode') || undefined,
    scene: q('scene') || undefined,
    shot: q('shot') || undefined,
    deliverable: q('deliverable') || undefined,
    continuity: q('continuity') || undefined,
    lighting: q('lighting') || undefined,
    package: q('package') || undefined,
    view: q('view') || undefined,
    query: {
      revision: q('revision') || undefined,
      version: q('version') || undefined,
      q: q('find') || undefined,
      status: q('status') || undefined,
    },
  }
}

watch(
  () => [route.fullPath, props.tab],
  () => {
    rememberDramaListContext(currentView())
    if (dramaView.value?.projectId && tab.value === 'materials' && material.value && material.value.states.length > 0 && !q('state') && !q('lighting') && state.value?.key) {
      void router.replace(link({ state: state.value.key }))
    }
  },
  { immediate: true }
)

function link(change: Record<string, string | undefined>) {
  const view = { ...currentView(), query: { ...currentView().query } }
  const reset = 'tab' in change || ['material', 'shot', 'deliverable'].some(key => key in change)
  if (reset) { view.lighting = undefined; view.package = undefined }
  for (const [key, value] of Object.entries(change)) {
    if (key === 'find') view.query.q = value
    else if (key === 'revision' || key === 'version' || key === 'status' || key === 'q') view.query[key] = value
    else if (key === 'tab') view.tab = value || view.tab
    else (view as unknown as Record<string, string | undefined>)[key] = value
  }
  if (view.tab === 'materials') view.tab = view.kind || 'character'
  const href = dramaHref(view)
  const url = new URL(href, 'https://app.invalid')
  return { path: url.pathname, query: Object.fromEntries(url.searchParams) }
}

const crumbs = computed(() => dramaBreadcrumb(currentView(), {
  work: project.value.manifest.name,
  material: material.value?.name,
  episode: episode.value?.title,
  scene: scene.value?.title,
  shot: shot.value?.title,
  deliverable: delivery.value?.title,
  state: state.value?.name,
  version: q('version') || undefined,
}))

function materialLink(g: DramaGlobal, version?: string, stateKey?: string) {
  return link({ tab: g.kind === 'script' ? 'script' : g.kind, material: g.key, state: stateKey, version, find: undefined, status: undefined, kind: g.kind, compare: undefined, episode: undefined, scene: undefined, shot: undefined, deliverable: undefined, returnMaterial: undefined })
}

const target = computed(() => [props.tab, q('material'), q('state'), q('episode'), q('scene'), q('shot'), q('deliverable'), q('version'), q('continuity')].join('/'))
</script>

<template>
  <div class="short-drama" :class="{ 'with-rail': showRail }">
    <nav v-if="showRail" class="category-rail" :aria-label="railTitle">
      <h2>{{ railTitle }}</h2>
      <RouterLink v-for="k in rail" :key="k.key" :class="{ active: activeKind === k.key }" :to="categoryLink(k.key)">{{ k.label }}</RouterLink>
    </nav>

    <div class="workspace-body">
      <nav v-if="!hideCrumb" class="crumb" aria-label="面包屑">
        <template v-for="(item, index) in crumbs" :key="item.label + index">
          <RouterLink v-if="index < crumbs.length - 1" :to="link(crumbChange(item.to))">{{ item.label }}</RouterLink>
          <span v-else aria-current="page">{{ item.label }}</span>
          <span v-if="index < crumbs.length - 1">/</span>
        </template>
      </nav>

      <DramaMaterialDetail
        v-if="q('lighting') || q('material')"
        :reader="reader"
        :tab="tab"
        :q="q"
        :link="link"
        :material-link="materialLink"
        :target="target"
      />

      <DramaShotDetail
        v-else-if="q('shot') || (tab === 'delivery' && q('deliverable') && q('version') && !q('package') && false)"
        :reader="reader"
        :q="q"
        :link="link"
        :material-link="materialLink"
        :target="target"
      />

      <DramaDeliveryView
        v-else-if="tab === 'delivery'"
        :reader="reader"
        :q="q"
        :link="link"
        :target="target"
      />

      <DramaOverview
        v-else-if="tab === 'overview'"
        :reader="reader"
        :link="link"
      />

      <template v-else-if="tab === 'materials' || tab === 'script'">
        <header class="section-head">
          <div>
            <span class="muted">{{ tab === 'materials' ? '项目共用素材' : '剧本与配套设定' }}</span>
            <h2>{{ activeName }}</h2>
            <RouterLink v-if="activeKind === 'location'" class="related-link" :to="link({ tab: 'production', view: 'spaces', kind: undefined })">查看白模与灯光设计 →</RouterLink>
          </div>
          <span v-if="activeKind !== 'script' && activeKind !== 'continuity'" class="muted">{{ filtered.length }} 项</span>
        </header>

        <template v-if="tab === 'script' && activeKind === 'script'">
          <nav v-if="!q('scene')" class="episode-tabs" aria-label="分集">
            <RouterLink v-for="(e, i) in drama.episodes" :key="e.key" :class="{ active: e.key === episode?.key }" :to="link({ episode: e.key, scene: undefined, shot: undefined })">第 {{ i + 1 }} 集</RouterLink>
          </nav>
          <p v-if="q('scene') && !scene" class="empty">当前修订没有此剧本场次。</p>
          <div v-else-if="episode" class="reader-layout">
            <article class="manuscript">
              <div class="chapter-heading">
                <span class="eyebrow">第 {{ drama.episodes.indexOf(episode) + 1 }} 集</span>
                <h2>{{ episode.title }}</h2>
                <p class="muted">{{ episodeShots.length }} 镜 · {{ episodeShots.reduce((n, s) => n + (s.duration_seconds || 0), 0) }} 秒画面预算</p>
              </div>
              <DramaProse v-if="episode.overview" :text="episode.overview" :drama="drama" />
              <template v-for="s in episode.scenes.filter(s => !q('scene') || s.key === q('scene'))" :key="s.key">
                <h3><RouterLink :to="link({ scene: s.key })">{{ s.title }} · 查看场次 →</RouterLink></h3>
                <section v-for="sh in s.shots" :key="sh.key" class="script-scene">
                  <header class="scene-heading">
                    <h3>{{ sh.title }}</h3>
                    <span class="muted">{{ sh.duration_seconds == null ? '时长待定' : sh.duration_seconds + ' 秒' }}</span>
                  </header>
                  <DramaProse hide-production-notes :text="sh.overview" :drama="drama" />
                  <div class="scene-links">
                    <RouterLink :to="link({ kind: 'continuity', continuity: sh.key, find: undefined })">查看连续性 →</RouterLink>
                    <RouterLink :to="shotLink(s.key, sh.key, 'production')">查看镜头制作 →</RouterLink>
                  </div>
                </section>
              </template>
              <nav v-if="!q('scene')" class="chapter-nav" aria-label="前后分集">
                <RouterLink v-if="drama.episodes.indexOf(episode) > 0" :to="link({ episode: drama.episodes[drama.episodes.indexOf(episode) - 1].key })">← 上一集</RouterLink>
                <span v-else />
                <RouterLink v-if="drama.episodes.indexOf(episode) < drama.episodes.length - 1" :to="link({ episode: drama.episodes[drama.episodes.indexOf(episode) + 1].key })">下一集 →</RouterLink>
              </nav>
              <DramaDocument :reader="reader" :file-id="episode.script_file_id || drama.script_file_id" title="剧本完整原稿" />
            </article>
            <aside class="stack">
              <section class="panel">
                <h2>本集人物</h2>
                <RouterLink v-for="g in episodeCharacters" :key="g.key" class="related-link" :to="link({ material: g.key, kind: 'character' })">{{ g.name }}</RouterLink>
                <h3>配套设定</h3>
                <RouterLink v-for="k in scriptKinds.filter(k => ['location', 'wardrobe', 'prop'].includes(k.key))" :key="k.key" class="related-link" :to="categoryLink(k.key)">{{ k.label }}</RouterLink>
              </section>
              <DramaTips :project-id="project.project_id" :revision-id="project.revision_id" :target="target" />
            </aside>
          </div>
          <div v-else class="empty">
            尚未整理分集剧本。
            <DramaDocument :reader="reader" :file-id="drama.script_file_id" title="剧本正文" />
          </div>
        </template>

        <template v-else>
          <div class="material-toolbar">
            <form v-if="!q('continuity')" class="search" @submit.prevent="search">
              <input v-model="searchDraft" type="search" aria-label="搜索当前内容" placeholder="搜索名称或设定">
              <button class="btn" type="submit">搜索</button>
            </form>
            <select v-if="tab === 'materials'" aria-label="素材筛选" :value="q('status') || 'all'" @change="router.replace(link({ status: ($event.target as HTMLSelectElement).value }))">
              <option value="all">全部素材</option>
              <option value="ready">已有候选或成果</option>
              <option value="missing">待制作</option>
            </select>
          </div>

          <template v-if="activeKind === 'continuity'">
            <template v-if="q('continuity')">
              <div v-if="continuityEntry" class="reader-layout">
                <article class="manuscript">
                  <span class="eyebrow">{{ continuityEntry.episode.title }}</span>
                  <h2>{{ continuityEntry.title }}</h2>
                  <DramaDocument v-for="r in continuityEntry.refs" :key="`${r.file_id}:${r.anchor}`" :reader="reader" :file-id="r.file_id" :anchor="r.anchor" title="本镜连续性" initially-expanded />
                </article>
                <aside class="stack">
                  <section class="panel">
                    <h2>相关资料</h2>
                    <RouterLink class="related-link" :to="link({ tab: 'production', episode: continuityEntry.episode.key, scene: continuityEntry.scene.key, shot: continuityEntry.key, continuity: undefined })">查看镜头与固定素材 →</RouterLink>
                    <RouterLink class="related-link" :to="link({ kind: 'script', episode: continuityEntry.episode.key, continuity: undefined })">阅读本集剧本 →</RouterLink>
                  </section>
                  <DramaTips :project-id="project.project_id" :revision-id="project.revision_id" :target="target" />
                </aside>
              </div>
              <p v-else class="empty">本镜未关联可定位的连续性章节，请查看完整资料。</p>
            </template>

            <div v-else class="setting-grid">
              <RouterLink v-for="entry in continuityEntries.filter(e => matches(e.title + ' ' + e.episode.title))" :key="entry.key" class="setting-card" :to="link({ continuity: entry.key })">
                <div class="material-copy">
                  <span class="muted">{{ entry.episode.title }}</span>
                  <h3>{{ entry.title }}</h3>
                  <p>核对本镜的人物、服装、道具、场景状态与灯光衔接。</p>
                  <span class="read-more">阅读连续性 →</span>
                </div>
              </RouterLink>
              <p v-if="continuityEntries.length && !continuityEntries.some(e => matches(e.title + ' ' + e.episode.title))" class="empty">暂无匹配镜头，请调整关键词。</p>
            </div>

            <section v-for="c in drama.continuity" :key="c.key" class="panel">
              <h3>{{ human(c.name) }}</h3>
              <DramaProse :text="c.description" :drama="drama" />
              <DramaDocument v-for="(r, i) in c.definition_refs" :key="`${r.file_id}:${i}`" :reader="reader" :file-id="r.file_id" :anchor="r.anchor" :title="`连续性资料 ${i + 1}`" />
              <DramaDocument v-for="(id, i) in c.reference_file_ids.filter(id => !c.definition_refs.some(r => r.file_id === id))" :key="id" :reader="reader" :file-id="id" :title="`连续性完整原稿 ${i + 1}`" />
            </section>
            <p v-if="!drama.continuity.length" class="empty">连续性设定尚未整理。</p>
          </template>

          <div v-else :class="tab === 'materials' ? ['material-grid', `kind-${activeKind}`] : 'setting-grid'">
            <RouterLink
              v-for="g in filtered"
              :key="g.key"
              :class="tab === 'materials' ? 'material-card' : 'setting-card'"
              :to="link({ material: g.key, kind: g.kind, returnMaterial: undefined, state: tab === 'materials' ? materialPreview(g).version?.state_key : undefined, version: tab === 'materials' ? materialPreview(g).version?.key : undefined, compare: undefined })"
            >
              <div v-if="tab === 'materials'" class="media-thumb">
                <ProjectMedia v-if="materialPreview(g).fileId" :reader="reader" :file-id="materialPreview(g).fileId" kind="image" :label="g.name" thumbnail />
                <span v-else class="material-empty-icon">{{ ({ character: '人', wardrobe: '衣', prop: '物', location: '景', sound: '♪', music: '♫' } as Record<string, string>)[g.kind] }}</span>
              </div>
              <div class="material-copy">
                <span class="muted">{{ tab === 'materials' ? cardStatus(g) : activeName }}</span>
                <h3>{{ g.name }}</h3>
                <span v-if="tab === 'materials' && materialPreview(g).version" class="preview-status">{{ previewStatus(g) }}</span>
                <p>{{ human(g.description) || '查看设定与关联内容' }}</p>
                <span class="read-more">{{ tab === 'materials' ? '查看设定与素材' : '阅读全文' }} →</span>
              </div>
            </RouterLink>
            <p v-if="!filtered.length" class="empty">暂无匹配资料，请调整名称或筛选条件。</p>
          </div>
        </template>
      </template>

      <DramaProductionView
        v-else-if="tab === 'production'"
        :reader="reader"
        :q="q"
        :link="link"
        :material-link="materialLink"
        :shot-link="shotLink"
      />
    </div>
  </div>
</template>

<style scoped>
.material-toolbar{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin:18px 0}.material-toolbar label{display:grid;gap:6px;font-size:12px;color:var(--muted)}.material-toolbar select{max-width:100%}.material-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:16px;align-items:stretch}.material-grid.kind-location{grid-template-columns:repeat(auto-fill,minmax(320px,1fr))}.material-card{display:flex;flex-direction:column;background:var(--shell-panel);border:1px solid var(--line);border-radius:8px;overflow:hidden;min-width:0;height:100%}.material-card:hover{border-color:var(--accent)}.material-card .media-thumb{width:100%;aspect-ratio:3/4;height:auto;display:flex;align-items:center;justify-content:center;background:var(--subtle);padding:0;position:relative;overflow:hidden}.material-grid.kind-location .material-card .media-thumb{aspect-ratio:16/10}.material-copy{padding:14px 16px;display:flex;flex-direction:column;flex:1;min-width:0}.material-copy .muted{font-size:12px;line-height:1.4;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.material-copy h3{font-size:16px;margin:6px 0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.material-copy .preview-status{font-size:12px;color:var(--muted);line-height:1.4;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.material-copy p{font-size:13px;color:var(--muted);margin:6px 0 10px;line-height:1.6;display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:2;overflow:hidden}
.material-empty-icon{font-size:36px;color:var(--muted);display:grid;place-items:center;width:100%;height:100%;background:var(--subtle)}.material-card .read-more{display:block;margin-top:12px}
@media(max-width:800px){.material-grid{grid-template-columns:repeat(auto-fill,minmax(160px,1fr));gap:12px}.material-toolbar{align-items:stretch}.material-toolbar label{width:100%}.material-toolbar select{width:100%}}

.short-drama { min-width:0; overflow-wrap:anywhere; }
.with-rail { display:grid; grid-template-columns:162px minmax(0,1fr); gap:26px; align-items:start; }
.workspace-body { min-width:0; }
.category-rail { display:grid; gap:4px; border-right:1px solid var(--line); padding-right:15px; }
.category-rail h2 { font-size:12px; color:var(--muted); margin:3px 8px 12px; }
.category-rail a { padding:10px; font-size:13px; color:var(--muted); border-radius:5px; }
.category-rail a.active { background:var(--shell-button); color:var(--ink); box-shadow:inset 2px 0 var(--accent); }
.panel { background:var(--shell-panel); border:1px solid var(--line); border-radius:8px; padding:20px; min-width:0; }
h2 { font-size:18px; line-height:1.35; margin:0 0 10px; }.panel h2 { font-size:15px; }h3 { font-size:15px; margin:8px 0; }
.muted,small { color:var(--muted); font-size:12px; line-height:1.85; }
.section-head { margin:24px 0 16px; display:flex; align-items:start; justify-content:space-between; gap:20px; }.section-head h2 { margin:3px 0 0; }.workspace-body > .section-head { margin-top:0; }
.stack { display:grid; gap:18px; min-width:0; }
.media-thumb :deep(.project-media) { width:100%; height:100%; aspect-ratio:auto; border-radius:0; background:transparent; border:0; }.media-thumb :deep(.image-open) { width:100%; height:100%; display:block; }.media-thumb :deep(img) { width:100%; height:100%; object-fit:cover; object-position:center 20%; display:block; }.material-grid.kind-location .media-thumb :deep(img) { object-position:center center; }.media-thumb :deep(.media-state) { min-height:0; height:100%; font-size:12px; display:flex; align-items:center; justify-content:center; padding:12px; }
.preview-status { display:block; color:var(--muted); font-size:12px; line-height:1.8; }
.material-copy p { display:-webkit-box; -webkit-box-orient:vertical; -webkit-line-clamp:3; overflow:hidden; }.read-more { display:block; margin-top:12px; color:var(--accent); font-size:12px; }
.setting-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:14px; }
.setting-card { border:1px solid var(--line); border-radius:7px; background:var(--shell-panel); min-width:0; }.setting-card .material-copy { padding:22px; }.setting-card h3 { font-size:18px; margin:12px 0; }.setting-card p { -webkit-line-clamp:4; }
.setting-card:hover { border-color:var(--accent); }
.search { display:flex; gap:8px; flex:1; min-width:180px; }.search input { flex:1; min-width:0; }.search .btn { white-space:nowrap; }
.episode-tabs,.scene-links { display:flex; gap:8px; flex-wrap:wrap; }.episode-tabs { margin:18px 0; }.episode-tabs a { border:1px solid var(--line); padding:7px 12px; border-radius:5px; font-size:12px; color:var(--muted); }.episode-tabs a.active { border-color:var(--accent); color:var(--accent); background:transparent; }
.related-link { display:block; color:var(--accent); font-size:13px; line-height:1.7; padding:6px 0; }.scene-links { margin-top:18px; gap:12px; }.scene-links a { color:var(--accent); font-size:12px; }
.empty { color:var(--muted); padding:0; border:0; font-size:13px; line-height:1.8; }
.crumb { display:flex; flex-wrap:wrap; gap:6px 8px; color:var(--muted); font-size:12px; margin-bottom:12px; }
.crumb a { color:var(--muted); }
.reader-layout > .stack { position:sticky; top:16px; align-self:start; gap:16px; }
.btn { justify-self:start; font-size:14px; border-radius:8px; padding:8px 12px; min-height:35px; }
input,select { border:1px solid var(--line); background:var(--panel); color:var(--ink); padding:9px 11px; border-radius:6px; max-width:100%; font:inherit; font-size:13px; }
.eyebrow { color:var(--accent); font-size:11px; letter-spacing:.12em; }
.reader-layout { display:grid; grid-template-columns:minmax(0,1fr) 235px; gap:22px; align-items:start; }.manuscript { padding:32px clamp(20px,3vw,44px); border:1px solid var(--line); border-radius:7px; background:var(--shell-panel); min-width:0; }.manuscript :deep(.drama-prose) { font-size:15px; line-height:1.95; }.manuscript :deep(.drama-prose p) { font-size:15px; line-height:1.95; margin:13px 0; }.manuscript h2 { font-size:22px; }.chapter-heading { border-bottom:1px solid var(--line); padding-bottom:24px; }.chapter-heading h2 { margin:6px 0 12px; font-size:28px; }.script-scene { padding:24px 0; border-bottom:1px solid var(--line); }.scene-heading { display:flex; align-items:center; justify-content:space-between; gap:12px; margin-bottom:16px; }.scene-heading h3 { font-size:19px; margin:0; }
.chapter-nav { display:flex; justify-content:space-between; gap:10px; margin-top:24px; color:var(--accent); font-size:13px; }
@media(max-width:1250px) { .reader-layout { grid-template-columns:minmax(0,1fr); }.reader-layout > aside { grid-template-columns:repeat(2,minmax(0,1fr)); position:static; } }
@media(max-width:800px) { .with-rail { grid-template-columns:minmax(0,1fr); gap:20px; }.category-rail { display:flex; flex-wrap:wrap; border-right:0; border-bottom:1px solid var(--line); padding:0 0 16px; gap:3px; }.category-rail h2 { display:none; }.category-rail a { padding:8px; }.category-rail a.active { box-shadow:none; }.setting-grid,.reader-layout > aside { grid-template-columns:minmax(0,1fr); }.section-head { flex-wrap:wrap; }.panel { padding:16px; }.manuscript { padding:24px 18px; }.chapter-heading h2 { font-size:23px; }.scene-heading h3 { font-size:17px; } }
</style>
