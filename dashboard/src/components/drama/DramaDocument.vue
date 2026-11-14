<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { ProjectReader, readerError } from '../../platform/project-reader'
import { documentSection } from '../../platform/drama-presentation'
import DramaProse from './DramaProse.vue'
import ProjectMedia from '../ProjectMedia.vue'
const props = defineProps<{ reader: ProjectReader; fileId?: string | null; title: string; anchor?: string; initiallyExpanded?: boolean }>()
const text = ref(''), expanded = ref(false), busy = ref(false), error = ref(''), structured = ref(false), loaded = ref(false)
let serial = 0
async function open() {
  expanded.value = !expanded.value
  if (!expanded.value || loaded.value || !props.fileId) return
  const current = ++serial; busy.value = true; error.value = ''
  try {
    const result = await props.reader.document(props.fileId)
    if (serial !== current) return
    structured.value = !['text/plain', 'text/markdown'].includes(result.mime)
    text.value = structured.value ? '' : documentSection(result.text, props.anchor)
    loaded.value = true
  } catch (cause) { if (serial === current) error.value = readerError(cause) }
  finally { if (serial === current) busy.value = false }
}
watch(() => [props.reader, props.fileId, props.anchor], () => { serial++; text.value = ''; expanded.value = false; loaded.value = false; busy.value = false; error.value = ''; structured.value = false; if (props.initiallyExpanded) void open() }, { immediate: true })
onBeforeUnmount(() => { serial++ })
</script>
<template>
  <div class="drama-document" :class="{ open: expanded }">
    <button v-if="fileId" type="button" class="btn doc-toggle" :aria-expanded="expanded" @click="open">
      <span>{{ title }}</span>
      <span class="doc-toggle-action">{{ expanded ? '收起' : '展开阅读' }}</span>
    </button>
    <p v-else class="muted">{{ title }}尚未登记</p>
    <div v-if="expanded" class="document-reader-box">
      <p v-if="busy" role="status" class="muted">正在读取固定版本…</p>
      <p v-else-if="error" role="alert" class="error">{{ error }}<button type="button" class="btn" @click="expanded = false; open()">重试</button></p>
      <template v-else-if="structured"><p class="muted">这份资料为工程记录，可下载查看；页面不展开内部字段。</p><ProjectMedia :reader="reader" :file-id="fileId" kind="file" :label="title" /></template>
      <DramaProse v-else subdued-headings class="document-text" :text="text" :drama="reader.content?.ai_short_drama" />
    </div>
  </div>
</template>
<style scoped>
.drama-document { min-width: 0; margin: 8px 0; }
.doc-toggle { width: 100%; justify-content: space-between; padding: 9px 14px; border-radius: 8px; background: var(--subtle); font-size: 13px; font-weight: 550; }
.doc-toggle-action { font-size: 12px; color: var(--accent); font-weight: 500; }
.document-reader-box { margin-top: 8px; padding: 12px 16px 14px; border: 0.5px solid var(--vf-hairline-outer, var(--line)); border-left: 2px solid var(--accent); border-radius: 0 8px 8px 0; background: var(--panel); color: var(--muted); }
.document-text { font: inherit; white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.8; margin: 0; }
.muted { color: var(--muted); }.error { color: var(--danger); }
</style>
