<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef } from "vue";
import { ProjectReader, type ProjectItem } from "../platform/project-reader";
import ProjectMedia from "./ProjectMedia.vue";
import { useHost } from '../platform/host';
import { coverFile, defaultCoverPosition, type CoverPosition } from '../platform/cover-position';
const props = defineProps<{ project: ProjectItem }>();
const root = ref<HTMLElement | null>(null),
  reader = shallowRef<ProjectReader | null>(null),
  fileID = ref<string | null>(null);
let observer: IntersectionObserver | undefined,
  disposed = false;
let stopLifecycle = () => {};
const position = ref<CoverPosition>(defaultCoverPosition());
function load() {
  reader.value?.dispose();
  fileID.value = null;
  const next = new ProjectReader(props.project.project_id, props.project.head_revision_id);
  reader.value = next;
  void next.load().then(() => {
    if (!disposed && reader.value === next) {
      fileID.value = coverFile(next.content);
      position.value = next.content?.cover_position || defaultCoverPosition();
    }
  }).catch(() => {});
}
onMounted(() => {
  stopLifecycle = useHost().appSDK.lifecycle.subscribe(event => {
    if (event === 'hidden') { reader.value?.dispose(); reader.value = null; fileID.value = null; }
    if (event === 'active' && root.value && root.value.getBoundingClientRect().top < window.innerHeight + 100) load();
  });
  observer = new IntersectionObserver(
    (entries) => {
      if (!entries.some((e) => e.isIntersecting)) return;
      observer?.disconnect();
      load();
    },
    { rootMargin: "100px" },
  );
  if (root.value) observer.observe(root.value);
});
onBeforeUnmount(() => {
  disposed = true;
  observer?.disconnect();
  stopLifecycle();
  reader.value?.dispose();
});
</script>

<template>
  <div ref="root" class="project-cover">
    <ProjectMedia
      v-if="reader && fileID"
      :reader="reader"
      :file-id="fileID"
      kind="image"
      :label="project.name"
      thumbnail
      :contain="project.category === 'image'"
      :object-position="position"
    />
    <span v-else aria-hidden="true">{{
      project.category === "music"
        ? "♫"
        : project.category === "video"
          ? "▷"
          : "◉"
    }}</span>
  </div>
</template>

<style scoped>
.project-cover {
  aspect-ratio: 16 / 10;
  display: grid;
  place-items: center;
  background: var(--subtle);
  border-bottom: 1px solid var(--line);
}
.project-cover > span {
  font-size: 56px;
  line-height: 1;
  color: var(--accent);
}
.project-cover > :deep(.project-media) {
  width: 100%;
  border-radius: 0;
}
</style>
