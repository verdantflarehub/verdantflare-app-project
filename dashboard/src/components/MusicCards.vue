<script setup lang="ts">
import { useRoute } from 'vue-router'
import type { Detail, ProjectReader } from '../platform/project-reader'
import { currentMusicOutput } from '../platform/music-production'
import ProjectMedia from './ProjectMedia.vue'
defineProps<{ reader: ProjectReader; entries: Detail[]; group: 'songs' | 'mvs' }>()
const route = useRoute()
</script>
<template>
  <div class="music-cards">
    <article v-for="entry in entries" :key="entry.key" class="music-card">
      <RouterLink class="project-card" :to="{ path: route.path, query: { ...route.query, tab: group, item: entry.key, version: undefined } }">
        <ProjectMedia v-if="entry.image_file_id" :reader="reader" :file-id="entry.image_file_id" kind="image" :label="entry.title" thumbnail /><div v-else class="music-glyph" aria-hidden="true">♫</div>
        <div class="card-text"><h2>{{ entry.title }}</h2><p>{{ entry.overview || '未填写' }}</p><span>{{ group === 'songs' ? '查看歌曲' : '查看 MV' }} →</span></div>
      </RouterLink>
      <div v-if="group === 'songs'" class="preview"><ProjectMedia :reader="reader" :file-id="currentMusicOutput(entry)" kind="audio" :label="entry.title + '试听'" /></div>
    </article>
  </div>
</template>
<style scoped>
.music-cards { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:20px; }.music-card { min-width:0; border:1px solid var(--line); border-radius:14px; overflow:hidden; background:var(--panel); }.project-card { display:block; }.project-card:hover { color:var(--accent); }.card-text,.preview { padding:16px; }.preview { padding-top:0; }h2 { font-size:18px; margin:0 0 10px; }p { color:var(--muted); white-space:pre-wrap; overflow-wrap:anywhere; }.music-glyph { aspect-ratio:16/10; display:grid; place-items:center; font-size:56px; color:var(--accent); background:var(--subtle); }@media(max-width:1150px) { .music-cards { grid-template-columns:repeat(2,minmax(0,1fr)); } }@media(max-width:700px) { .music-cards { grid-template-columns:minmax(0,1fr); } }
</style>
