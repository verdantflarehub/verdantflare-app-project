<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import ProjectMedia from '../ProjectMedia.vue'
import CreativeVideoEditor from './CreativeVideoEditor.vue'
import CreativePlanningEditor from './CreativePlanningEditor.vue'
import CreativeEditEditor from './CreativeEditEditor.vue'
import CreativeDeliveryEditor from './CreativeDeliveryEditor.vue'
import type { ProjectReader } from '../../platform/project-reader'

const props = defineProps<{ reader: ProjectReader; tab: string }>()
const route = useRoute()
const work = computed(() => props.reader.content!.creative_video!)
const query = (key: string) => typeof route.query[key] === 'string' ? route.query[key] as string : ''
const link = (change: Record<string, string | undefined>) => ({ path: route.path, query: { ...route.query, ...change } })
const selectedVariant = computed(() => work.value.delivery_variants.find(v => v.key === query('variant')) || work.value.delivery_variants[0])
const selectedEdit = computed(() => work.value.edits.find(e => e.key === query('edit')) || work.value.edits[0])
const selectedShot = computed(() => work.value.shots.find(s => s.key === query('shot')) || work.value.shots[0])
const variantPackages = computed(() => work.value.packages.filter(p => p.variant_key === selectedVariant.value?.key))
const variantPackage = computed(() => variantPackages.value.find(p => p.key === query('package')) || variantPackages.value[variantPackages.value.length - 1])
const variantReviews = computed(() => work.value.reviews.filter(r => r.target_key === variantPackage.value?.key || r.target_key === selectedVariant.value?.edit_key))
const directionName: Record<string, string> = { promo: '宣传片', advertisement: '广告', brand_film: '品牌片', music_video: '音乐驱动视频' }
const reviewName: Record<string, string> = { pending: '待审核', in_review: '审核中', approved: '已通过', changes_requested: '需修改', rejected: '未通过' }
const label = (value: string | null | undefined) => value || '未填写'
const emit = defineEmits<{ saved: [revision: string] }>()
</script>

<template>
  <div class="creative-work">
    <template v-if="tab === 'overview'">
      <div class="summary"><div><small>创作方向</small><strong>{{ directionName[work.direction] }}</strong></div><div><small>计划镜头</small><strong>{{ work.shots.length }}</strong></div><div><small>交付版</small><strong>{{ work.delivery_variants.length }}</strong></div><div><small>待审核</small><strong>{{ work.reviews.filter(r => r.state === 'pending' || r.state === 'in_review').length }}</strong></div></div>
      <div class="columns"><section><h2>传播目标</h2><p>{{ work.brief.goal }}</p><dl class="facts"><dt>受众</dt><dd>{{ label(work.brief.audience) }}</dd><dt>核心信息</dt><dd>{{ label(work.brief.message) }}</dd><dt>期望行动</dt><dd>{{ label(work.brief.action) }}</dd></dl></section><section><h2>交付进度</h2><div v-if="!work.delivery_variants.length" class="empty">尚未规划交付版。</div><div v-for="variant in work.delivery_variants" :key="variant.key" class="line"><RouterLink :to="link({ tab: 'delivery', variant: variant.key })">{{ variant.key }}</RouterLink><span>{{ variant.width }} × {{ variant.height }} · {{ variant.target_frames }} 帧</span></div></section></div>
    </template>

    <template v-else-if="tab === 'brief'">
      <CreativeVideoEditor :reader="reader" @saved="emit('saved', $event)" />
      <div class="columns"><section><h2>作品任务书</h2><dl class="facts"><dt>目标</dt><dd>{{ work.brief.goal }}</dd><dt>委托方</dt><dd>{{ label(work.brief.client) }}</dd><dt>受众</dt><dd>{{ label(work.brief.audience) }}</dd><dt>核心信息</dt><dd>{{ label(work.brief.message) }}</dd><dt>期望行动</dt><dd>{{ label(work.brief.action) }}</dd></dl><ProjectMedia v-if="work.brief.file_id" :reader="reader" :file-id="work.brief.file_id" kind="file" label="完整任务书" /></section><section><h2>音乐来源</h2><div v-if="work.music_source?.kind === 'project_file'"><RouterLink :to="{ path: route.path, query: { project: work.music_source.project_id, revision: work.music_source.revision_id, tab: 'songs' } }">打开来源音乐项目</RouterLink><p class="muted">引用固定项目修订与文件版本，使用前须核对选用和授权。</p></div><p v-else-if="work.music_source?.kind === 'world_asset'">已引用数字世界固定音乐版本。</p><p v-else class="empty">尚未关联音乐项目。</p><h2>权利记录</h2><p v-if="!work.rights.length" class="empty">尚未登记权利记录。</p><div v-for="right in work.rights" :key="right.key" class="line"><span>{{ right.subject_kind }} · {{ right.subject_key }}</span><span>{{ right.status }}</span></div></section></div>
    </template>

    <template v-else-if="tab === 'story'">
      <CreativePlanningEditor :reader="reader" view="story" @saved="emit('saved', $event)" />
      <section><h2>创意方案</h2><p v-if="!work.concepts.length" class="empty">尚未登记创意方案。</p><div v-for="concept in work.concepts" :key="concept.key" class="line"><span>{{ concept.key }} · {{ concept.status === 'selected' ? '已选用' : concept.status }}</span><ProjectMedia v-if="concept.file_id" :reader="reader" :file-id="concept.file_id" kind="file" label="查看方案" /></div></section>
      <section><h2>分镜顺序</h2><p v-if="!work.sequences.length" class="empty">尚未登记分镜。</p><div v-for="sequence in work.sequences" :key="sequence.key" class="line"><strong>{{ sequence.label || sequence.key }}</strong><span>{{ sequence.shot_keys.length }} 镜</span></div></section>
    </template>

    <template v-else-if="tab === 'shots'">
      <CreativePlanningEditor :reader="reader" view="shots" @saved="emit('saved', $event)" />
      <div v-if="!work.shots.length" class="empty">尚未登记镜头。</div><div v-else class="columns"><section><h2>镜头列表</h2><div v-for="shot in work.shots" :key="shot.key" class="line"><RouterLink :to="link({ shot: shot.key })">{{ shot.key }}</RouterLink><span>{{ shot.planned_frames }} 帧 · {{ shot.take_keys.length }} 个候选</span></div></section><section v-if="selectedShot"><h2>{{ selectedShot.key }} · 镜头详情</h2><ProjectMedia v-if="selectedShot.storyboard_file_id" :reader="reader" :file-id="selectedShot.storyboard_file_id" kind="image" label="分镜" /><p v-else class="muted">尚无分镜画面。</p><div v-for="take in work.takes.filter(t => t.shot_key === selectedShot?.key)" :key="take.key" class="take"><strong>{{ take.key }} · {{ take.status }}</strong><ProjectMedia v-if="take.file_id" :reader="reader" :file-id="take.file_id" kind="video" label="镜头候选" /></div></section></div>
    </template>

    <template v-else-if="tab === 'edit'">
      <CreativeEditEditor :reader="reader" @saved="emit('saved', $event)" />
      <div v-if="!work.edits.length" class="empty">尚未登记剪辑版本。</div><div v-else class="columns"><section><h2>剪辑版本</h2><div v-for="edit in work.edits" :key="edit.key" class="line"><RouterLink :to="link({ edit: edit.key })">{{ edit.key }}</RouterLink><span>{{ edit.items.length }} 个片段 · {{ label(edit.state) }}</span></div></section><section v-if="selectedEdit"><h2>{{ selectedEdit.key }} · 时间线</h2><p v-if="selectedEdit.parent_key" class="muted">基于 {{ selectedEdit.parent_key }}</p><p v-if="selectedEdit.music_source?.kind === 'project_file'" class="muted">音乐：固定音乐项目修订 {{ selectedEdit.music_source.revision_id }}</p><p v-else-if="selectedEdit.music_source?.kind === 'world_asset'" class="muted">音乐：固定数字世界版本</p><div v-for="item in selectedEdit.items" :key="`${item.take_key}:${item.timeline_in_frame}`" class="line"><span>{{ item.take_key }}</span><span>{{ item.timeline_in_frame }} → {{ item.timeline_in_frame + item.source_out_frame - item.source_in_frame }} 帧</span></div><ProjectMedia v-if="selectedEdit.project_file_id" :reader="reader" :file-id="selectedEdit.project_file_id" kind="file" label="剪辑工程" /></section></div>
    </template>

    <template v-else-if="tab === 'delivery'">
      <CreativeDeliveryEditor :reader="reader" @saved="emit('saved', $event)" />
      <div v-if="!work.delivery_variants.length" class="empty">尚未规划交付版。</div><div v-else class="columns"><section><h2>交付版</h2><div v-for="variant in work.delivery_variants" :key="variant.key" class="line"><RouterLink :to="link({ variant: variant.key, package: undefined })">{{ variant.key }}</RouterLink><span>{{ variant.width }} × {{ variant.height }} · {{ variant.target_frames }} 帧</span></div></section><section v-if="selectedVariant"><h2>{{ selectedVariant.key }}</h2><dl class="facts"><dt>剪辑来源</dt><dd>{{ label(selectedVariant.edit_key) }}</dd><dt>渠道</dt><dd>{{ selectedVariant.channel }}</dd><dt>地域</dt><dd>{{ selectedVariant.territory }}</dd><dt>语言</dt><dd>{{ selectedVariant.language }}</dd><dt>导出状态</dt><dd>{{ variantPackage?.state || '尚无导出' }}</dd></dl><h3>导出版本</h3><p v-if="!variantPackages.length" class="muted">尚无固定导出。</p><div v-for="pack in variantPackages" :key="pack.key" class="line"><RouterLink :to="link({ package: pack.key })">{{ pack.key }}</RouterLink><span>{{ pack.state }}</span></div><ProjectMedia v-if="variantPackage" :reader="reader" :file-id="variantPackage.media_file_id" kind="video" label="固定交付文件" /><h3>逐版审核</h3><p v-if="!variantReviews.length" class="muted">尚无审核记录。</p><div v-for="review in variantReviews" :key="review.key" class="line"><span>{{ review.kind }}</span><span>{{ reviewName[review.state] || review.state }}</span></div></section></div>
    </template>
  </div>
</template>

<style scoped>
.creative-work { display: grid; gap: 24px; min-width: 0; overflow-wrap: anywhere; }
.creative-work section { min-width: 0; padding-top: 16px; border-top: 1px solid var(--line); }
.creative-work h2 { font-size: 18px; margin: 0 0 16px; }
.creative-work h3 { font-size: 14px; margin: 22px 0 10px; }
.creative-work p { margin: 0 0 16px; }
.creative-work a { color: var(--accent); }
.creative-work a:hover { text-decoration: underline; }
.summary { display: grid; grid-template-columns: repeat(4,minmax(0,1fr)); gap: 16px; padding: 14px 0 22px; border-bottom: 1px solid var(--line); }
.summary div { display: grid; gap: 3px; }
.summary small,.muted { color: var(--muted); }
.summary strong { font-size: 19px; }
.columns { display: grid; grid-template-columns: minmax(0,1.35fr) minmax(240px,.85fr); gap: 30px; }
.facts { display: grid; grid-template-columns: 90px minmax(0,1fr); margin: 0; }
.facts dt,.facts dd { padding: 10px 0; border-bottom: 1px solid var(--line); }
.facts dt { color: var(--muted); }.facts dd { margin: 0; }
.line { display: flex; justify-content: space-between; flex-wrap: wrap; gap: 10px; padding: 11px 0; border-bottom: 1px solid var(--line); }
.line span:last-child { color: var(--muted); }.empty { padding: 20px; border: 1px dashed var(--line); color: var(--muted); }
.take { padding: 14px 0; border-top: 1px solid var(--line); }.take strong { display: block; margin-bottom: 10px; }
@media(max-width:760px) { .columns { grid-template-columns: minmax(0,1fr); }.summary { grid-template-columns: repeat(2,minmax(0,1fr)); } }
</style>
