<script setup lang="ts">
import type { CharacterReferenceGroup } from '../platform/project-content'
import type { ProjectReader } from '../platform/project-reader'
import ProjectMedia from './ProjectMedia.vue'
defineProps<{ reader: ProjectReader; groups: CharacterReferenceGroup[] }>()
</script>

<template>
  <section v-if="groups.length" class="panel character-references">
    <h2>多角度参考</h2>
    <section v-for="(group, groupIndex) in groups" :key="group.key" class="reference-group">
      <h3>{{ group.title }}</h3>
      <div class="reference-grid">
        <figure v-for="(image, index) in group.images" :key="image.key">
          <ProjectMedia :reader="reader" :file-id="image.file_id" kind="image" :label="`${image.key} · ${image.label}`" thumbnail contain
            :gallery="{ id: `character-${groupIndex}`, index, items: group.images.map(item => ({ fileId: item.file_id, label: `${item.key} · ${item.label}` })) }" />
          <figcaption><strong>{{ image.key }}</strong><span>{{ image.label }}</span></figcaption>
        </figure>
      </div>
    </section>
  </section>
</template>

<style scoped>
.reference-group + .reference-group { margin-top: 28px; }
h3 { margin-bottom: 14px; }
.reference-grid { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 14px; }
figure { min-width: 0; margin: 0; }
figcaption { display: grid; gap: 4px; padding: 10px 0; font-size: 12px; text-align: center; }
figcaption span { color: var(--muted); }
.reference-grid :deep(.thumbnail.contain) { aspect-ratio: 3 / 4; }
@media (max-width: 900px) { .reference-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
</style>
