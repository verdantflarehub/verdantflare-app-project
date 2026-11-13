<script setup lang="ts">
import { ref } from 'vue'
defineProps<{ text?: string | null }>()
const message = ref('')
function fallbackCopy(text: string): boolean {
  if (typeof document === 'undefined' || !document.body || typeof document.createElement !== 'function' || typeof document.execCommand !== 'function') return false
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.top = '0'
  textarea.style.left = '0'
  textarea.style.opacity = '0'
  textarea.style.pointerEvents = 'none'
  document.body.appendChild(textarea)
  try {
    textarea.focus?.()
    textarea.select?.()
    textarea.setSelectionRange?.(0, textarea.value.length)
    return document.execCommand('copy') === true
  } catch {
    return false
  } finally {
    if (typeof textarea.remove === 'function') textarea.remove()
    else textarea.parentNode?.removeChild?.(textarea)
  }
}
async function copy(text: string) {
  try {
    if (!navigator?.clipboard?.writeText) throw new Error('CLIPBOARD_UNAVAILABLE')
    await navigator.clipboard.writeText(text)
    message.value = '已复制'
  } catch {
    message.value = fallbackCopy(text) ? '已复制' : '复制失败，请选择文字复制。'
  }
}
</script>

<template>
  <details class="project-prompt">
    <summary>提示词</summary>
    <p>{{ text?.trim() ? text : '未填写' }}</p>
    <button v-if="text?.trim()" class="btn" @click="copy(text)">复制提示词</button>
    <span role="status">{{ message }}</span>
  </details>
</template>

<style scoped>
.project-prompt { margin-top: 16px; border-top: 0.5px solid var(--vf-hairline-outer, var(--line)); padding-top: 12px; font-variant-numeric: tabular-nums; }
summary { cursor: pointer; color: var(--muted); transition: color 150ms var(--vf-ease-out, cubic-bezier(0.16, 1, 0.3, 1)); }
summary:hover { color: var(--accent); }
p { white-space: pre-wrap; overflow-wrap: anywhere; margin: 14px 0; }
span { margin-left: 12px; color: var(--muted); }
</style>
