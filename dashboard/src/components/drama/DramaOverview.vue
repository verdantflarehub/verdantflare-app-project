<script setup lang="ts">
import { computed } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import { ProjectReader, projectContentView } from '../../platform/project-reader'
import { dramaText } from '../../platform/drama-presentation'

const props = defineProps<{
  reader: ProjectReader
  link: (change: Record<string, string | undefined>) => RouteLocationRaw
}>()

const project = computed(() => props.reader.project!)
const drama = computed(() => projectContentView(project.value, props.reader.content)!.ai_short_drama!)
const allShots = computed(() => drama.value.episodes.flatMap(e => e.scenes.flatMap(s => s.shots)))
const shotCount = computed(() => allShots.value.length)
const budget = computed(() => allShots.value.reduce((n, s) => n + (s.duration_seconds || 0), 0))
const coverTitle = computed(() => project.value.manifest.name.split('·'))
const scriptGlobal = computed(() => drama.value.globals.find(g => g.kind === 'script'))
const reviewName = (value?: string) => ({ unreviewed: '待审核', in_review: '审核中', approved: '已有审核记录', rejected: '需修改' }[value || ''] || '审核未登记')
const human = (text?: string | null) => dramaText(text, drama.value).replace(/\*\*/g, '').replace(/^\s*\||\|\s*$/g, '').replace(/\s*\|\s*/g, ' · ').trim()

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

const categoryDescription: Record<string, string> = {
  script: '分集故事与对白',
  character: '身份、经历与人物关系',
  location: '空间、时段与场面调度',
  wardrobe: '人物在不同阶段的衣着',
  prop: '外观、用途与流转',
  continuity: '逐镜核对人物与物件状态',
  sound: '人物声线与环境声音',
  music: '情绪、节奏与音乐安排',
}

function definitionVersion(kind: string) {
  const id = kind === 'continuity'
    ? drama.value.continuity[0]?.reference_file_ids[0] || drama.value.continuity[0]?.definition_refs[0]?.file_id
    : drama.value.globals.find(g => g.kind === kind)?.definition_refs[0]?.file_id
  const path = project.value.manifest.files.find(f => f.file_id === id)?.path
  const version = path?.match(/-v(\d+)\.[^.]+$/i)?.[1]
  return version ? `第 ${version} 稿` : '项目创作依据'
}
</script>

<template>
  <div class="overview-grid">
    <section class="story-cover">
      <div class="cover-label">
        {{ coverTitle[0] }}
        <template v-if="coverTitle.length > 1">
          <br><span>{{ coverTitle.slice(1).join('·') }}</span>
        </template>
      </div>
      <div>
        <span class="eyebrow">故事提要</span>
        <p>{{ human(reader.content?.overview) }}</p>
        <RouterLink class="btn primary" :to="link({ tab: 'script', kind: undefined })">阅读剧本 →</RouterLink>
      </div>
    </section>
    <aside class="panel">
      <h2>本轮审阅</h2>
      <p class="muted">先确认故事、人物与配套设定，再据此准备素材。</p>
      <dl>
        <dt>篇幅</dt>
        <dd>{{ drama.episodes.length }} 集 · {{ shotCount }} 镜</dd>
        <dt>画面预算</dt>
        <dd>{{ budget ? `${Math.floor(budget / 60)} 分 ${budget % 60} 秒` : '尚未填写' }}</dd>
        <dt>剧本</dt>
        <dd>{{ scriptGlobal?.name || definitionVersion('script') }} · {{ reviewName(scriptGlobal?.definition_review.state) }}</dd>
        <dt>制作素材</dt>
        <dd>{{ drama.globals.filter(g => g.versions.length).length }} 项已有候选</dd>
      </dl>
      <p class="muted">各版本的审核和选用分别记录，可从素材设定查看。</p>
    </aside>
  </div>
  <header class="section-head">
    <h2>剧本与配套设定</h2>
    <span class="muted">一部作品，共用一套创作依据</span>
  </header>
  <div class="document-grid">
    <RouterLink v-for="k in scriptKinds" :key="k.key" class="document-card" :to="link({ tab: k.key, kind: k.key })">
      <span class="doc-symbol">{{ k.label.slice(0, 1) }}</span>
      <div>
        <span class="muted">{{ definitionVersion(k.key) }}</span>
        <h3>{{ k.label }}</h3>
        <p>{{ categoryDescription[k.key] }}</p>
      </div>
      <span>↗</span>
    </RouterLink>
  </div>
  <section class="future">
    <h2>后续创作</h2>
    <p class="muted">人物小传用于人物形象，服装设定用于造型，地点与道具设定用于场景制作。选定成果可整理为数字世界资产，供后续作品复用。</p>
  </section>
</template>

<style scoped>
.panel { background: var(--shell-panel); border: 1px solid var(--line); border-radius: 8px; padding: 20px; min-width: 0; }
h2 { font-size: 18px; line-height: 1.35; margin: 0 0 10px; }
.panel h2 { font-size: 15px; }
h3 { font-size: 15px; margin: 8px 0; }
.muted { color: var(--muted); font-size: 12px; line-height: 1.85; }
.section-head { margin: 24px 0 16px; display: flex; align-items: start; justify-content: space-between; gap: 20px; }
.section-head h2 { margin: 3px 0 0; }
.btn { justify-self: start; font-size: 14px; border-radius: 8px; padding: 8px 12px; min-height: 35px; }
.overview-grid { display: grid; grid-template-columns: minmax(0, 1fr) 280px; gap: 22px; }
.story-cover { border: 1px solid var(--line); border-radius: 8px; background: var(--shell-panel); padding: 30px; display: grid; grid-template-columns: 190px minmax(0, 1fr); gap: 30px; align-items: center; }
.story-cover p { margin: 10px 0 24px; font-size: 16px; line-height: 1.9; }
.cover-label { font-family: "Songti SC", serif; font-size: 38px; line-height: 1.9; letter-spacing: .16em; border-right: 1px solid var(--line); padding: 25px 18px 25px 0; }
.cover-label span { font-size: 24px; }
.eyebrow { color: var(--accent); font-size: 11px; letter-spacing: .12em; }
dl { display: grid; grid-template-columns: 85px 1fr; font-size: 13px; gap: 12px; margin: 20px 0; }
dt { color: var(--muted); }
dd { margin: 0; }
.document-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.document-card { display: grid; grid-template-columns: 42px minmax(0, 1fr) 14px; gap: 16px; align-items: center; border: 1px solid var(--line); border-radius: 7px; padding: 20px; background: var(--shell-panel); }
.document-card:hover { border-color: var(--accent); }
.doc-symbol { font-family: "Songti SC", serif; font-size: 22px; border-radius: 4px; width: 42px; height: 48px; display: grid; place-items: center; border: 1px solid var(--line); color: var(--accent); }
.document-card h3 { margin: 5px 0; font-size: 16px; }
.document-card p { color: var(--muted); font-size: 12px; margin: 0; }
.future { border-top: 1px solid var(--line); padding-top: 22px; margin-top: 30px; }
@media (max-width: 1250px) { .overview-grid { grid-template-columns: minmax(0, 1fr); } }
@media (max-width: 800px) {
  .document-grid, .overview-grid, .story-cover { grid-template-columns: minmax(0, 1fr); }
  .section-head { flex-wrap: wrap; }
  .cover-label { border-right: 0; border-bottom: 1px solid var(--line); padding: 0 0 20px; font-size: 30px; line-height: 1.5; }
  .story-cover { padding: 24px; }
  .panel { padding: 16px; }
}
</style>
