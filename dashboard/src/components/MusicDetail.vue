<script setup lang="ts">
import type { ProjectReader, Detail } from '../platform/project-reader'
import type { MusicGroup } from '../platform/music-production'
import ProjectMedia from './ProjectMedia.vue'
import ProjectPrompt from './ProjectPrompt.vue'
import MusicProduction from './MusicProduction.vue'
defineProps<{ reader: ProjectReader; detail: Detail; group: MusicGroup }>()
defineEmits<{ saved: [revision: string] }>()
</script>
<template>
  <article class="music-detail">
    <section class="panel"><h2>基本信息</h2><div v-if="detail.image_file_id" class="cover"><ProjectMedia :reader="reader" :file-id="detail.image_file_id" kind="image" :label="detail.title + '封面'" /></div><p class="prose">{{ detail.overview || '未填写' }}</p><dl><dt>署名</dt><dd>{{ reader.content?.music?.artist || '未填写' }}</dd><template v-for="(field, i) in detail.fields" :key="i"><dt>{{ field.label }}</dt><dd>{{ field.value || '未填写' }}</dd></template></dl></section>
    <section v-if="group === 'songs'" class="panel"><h2>歌词</h2><p class="prose">{{ detail.lyrics || '未填写' }}</p></section>
    <section v-if="group !== 'voice'" class="panel"><h2>创作参考</h2><div v-if="detail.reference_file_ids.length" class="reference-row"><ProjectMedia v-for="id in detail.reference_file_ids" :key="id" :reader="reader" :file-id="id" kind="image" label="参考图" /></div><p v-else class="muted">未添加图片</p><ProjectPrompt :text="detail.prompt" /></section>
    <MusicProduction :reader="reader" :detail="detail" :group="group" @saved="$emit('saved', $event)" />
    <section v-if="detail.file_ids.length" class="panel"><h2>{{ group === 'voice' ? '模型与版本文件' : '相关文件' }}</h2><ProjectMedia v-for="id in detail.file_ids" :key="id" :reader="reader" :file-id="id" kind="file" :label="reader.file(id).path" /></section>
  </article>
</template>
<style scoped>
.cover { max-width:240px; margin-bottom:16px; }.music-detail { display:grid; gap:16px; min-width:0; }.panel { padding:20px; border:1px solid var(--line); border-radius:12px; background:var(--shell-panel); overflow-wrap:anywhere; }h2 { font-size:18px; margin:0 0 12px; }p { line-height:1.7; }.prose { white-space:pre-wrap; }.muted,dt { color:var(--muted); }dl { display:grid; grid-template-columns:100px minmax(0,1fr); gap:12px; }.reference-row { display:flex; gap:12px; overflow-x:auto; }.reference-row :deep(figure) { flex:0 0 180px; }
</style>
