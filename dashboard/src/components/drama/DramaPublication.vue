<script setup lang="ts">
import { onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useHost } from '../../platform/host'
import { ProjectReader, readerError, type DramaWorldCandidate, type DramaGlobal, type DramaDeliverable } from '../../platform/project-reader'
import { canonical, publicationPackage, type PublicationPackage } from '../../platform/drama-publication-package'
import { createProjectPublication, type PublicationIntent } from '../../platform/project-publication'
import { publicationError, publicationIdentity } from '../../platform/publication-client'
import { worldTarget } from '../../platform/route-links'
const props = defineProps<{ reader: ProjectReader; candidates: DramaWorldCandidate[] }>()
const { appSDK, dashboardHost } = useHost()
const route = useRoute(), router = useRouter()
const selected = shallowRef<DramaWorldCandidate>(), pack = shallowRef<PublicationPackage>(), owner = shallowRef<DramaGlobal | DramaDeliverable>()
const client = shallowRef<ReturnType<typeof createProjectPublication>>(), pending = shallowRef<PublicationIntent>()
const busy = ref(false), error = ref(''), acknowledged = ref(false), hidden = ref(false)
let serial = 0, navigation: AbortController | undefined
async function snapshot(project: string, revision?: string) {
  const reader = new ProjectReader(project, revision)
  try { await reader.load(); if (!reader.project || !reader.content) throw Error('项目内容暂不可用。'); return { project: reader.project, content: reader.content } } finally { reader.dispose() }
}
async function inspect(candidate: DramaWorldCandidate) {
  if (busy.value || hidden.value) return
  const current = ++serial; busy.value = true; error.value = ''; selected.value = candidate; pack.value = undefined; owner.value = undefined; acknowledged.value = false
  client.value?.dispose(); client.value = undefined; pending.value = undefined
  try {
    const project = props.reader.projectID
    const next = createProjectPublication(appSDK, localStorage, project, candidate.key, revision => snapshot(project, revision)); client.value = next; pending.value = next.pending()
    const originRevision = pending.value?.origin_revision_id || props.reader.project!.revision_id
    const original = await snapshot(project, originRevision), originalCandidate = original.content.ai_short_drama?.world_candidates.find(c => c.key === candidate.key)
    if (!originalCandidate) throw Error('原准备修订中没有该入库候选。')
    const source = new ProjectReader(project, originalCandidate.registration_request?.source_revision_id || originRevision)
    try {
      await source.load()
      const drama = source.content?.ai_short_drama, c = originalCandidate
      const material = c.source.kind === 'global' ? drama?.globals.find(g => g.key === c.source.key) : drama?.deliverables.find(g => g.key === c.source.key)
      if (!material || !c.publication_record_file_id || c.review.state !== 'approved') throw Error('此版入库说明或审核记录尚未准备。')
      const record = await source.document(c.publication_record_file_id), checked = publicationPackage(record.text, c, material, source.project!)
      const request = pending.value?.request || c.registration_request
      if (request && canonical([...request.file_ids].sort()) !== canonical(checked.files)) throw Error('原请求文件范围与入库说明不一致。')
      if (serial === current) { selected.value = c; pack.value = checked; owner.value = material }
    } finally { source.dispose() }
  } catch (cause) { if (serial === current) error.value = readerError(cause) }
  finally { if (serial === current) busy.value = false }
}
function versionLabel(key: string) { const version = owner.value?.versions.find(v => v.key === key); const state = owner.value && 'states' in owner.value ? owner.value.states.find(s => s.key === version?.state_key)?.name : ''; return [version?.label || '固定版本', state].filter(Boolean).join(' · ') }
async function continueToWorld() {
  if (!client.value || !pack.value || !acknowledged.value || busy.value || hidden.value) return
  const c = client.value, current = serial; busy.value = true; error.value = ''
  try {
    const path = await c.prepare(pending.value?.origin_revision_id || props.reader.project!.revision_id)
    if (serial !== current || client.value !== c || hidden.value) return
    pending.value = c.pending()
    navigation?.abort(); const controller = new AbortController(); navigation = controller
    await appSDK.capabilities.call('navigation.openApp', { appId: 'world', path }, { signal: controller.signal })
  } catch (cause) { if (serial === current && !hidden.value) error.value = publicationError(cause) }
  finally { if (serial === current) { pending.value = c.pending(); busy.value = false } }
}
async function openResult(candidate: DramaWorldCandidate) {
  if (candidate.registration_status?.state !== 'committed' || busy.value || hidden.value) return
  const result = candidate.registration_status.result, target = worldTarget(result.asset_id, result.asset_version_id)
  navigation?.abort(); const controller = new AbortController(); navigation = controller
  try { await appSDK.capabilities.call('navigation.openApp', { appId: target.appId, path: target.path }, { signal: controller.signal }) }
  catch { if (!controller.signal.aborted) error.value = '无法打开数字世界，请稍后重试。' }
}
async function restartPreparation() {
  if (!client.value || busy.value || hidden.value) return
  const c = client.value, current = serial; busy.value = true; error.value = ''
  try {
    const revision = await c.restartPreparation()
    if (serial !== current || client.value !== c || hidden.value) return
    pending.value = undefined; selected.value = undefined; pack.value = undefined; owner.value = undefined; acknowledged.value = false
    await router.replace({ path: route.path, query: { ...route.query, revision } })
  } catch (cause) { if (serial === current && !hidden.value) error.value = publicationError(cause) }
  finally { if (serial === current) { pending.value = c.pending(); busy.value = false } }
}
watch(busy, value => dashboardHost.setDirty(value), { flush: 'sync' })
watch(() => [props.reader, props.candidates], () => { serial++; navigation?.abort(); client.value?.dispose(); client.value = undefined; selected.value = undefined; pack.value = undefined; owner.value = undefined; pending.value = undefined; busy.value = false; error.value = ''; acknowledged.value = false })
const initial = publicationIdentity(appSDK.context.get())
const stopContext = appSDK.context.subscribe(value => { if (publicationIdentity(value) !== initial) { serial++; navigation?.abort(); client.value?.dispose(); client.value = undefined; selected.value = undefined; pending.value = undefined; pack.value = undefined; owner.value = undefined; busy.value = false; error.value = '会话已变化，请重新打开应用。原操作记录仍保留。' } })
const stopLifecycle = appSDK.lifecycle.subscribe(event => { hidden.value = event === 'hidden'; if (event === 'hidden' || event === 'dispose') navigation?.abort() })
onBeforeUnmount(() => { serial++; client.value?.dispose(); navigation?.abort(); stopContext(); stopLifecycle(); dashboardHost.setDirty(false) })
</script>
<template>
  <section class="panel">
    <h2>入库到数字世界</h2><p class="muted">保存固定版本与文件范围，前往数字世界核对并确认入库。项目当前选用可以与入库版本不同。</p>
    <p v-if="!candidates.length" class="muted">尚无入库候选。</p>
    <div v-for="candidate in candidates" :key="candidate.key" class="row"><strong>{{ candidate.name }}</strong><p class="muted">{{ candidate.registration_status?.state === 'committed' ? '已有固定版本入库记录' : candidate.review.state === 'approved' && candidate.publication_record_file_id ? '入库范围已准备' : '说明或审核尚未准备' }}</p><button class="btn" :disabled="busy || hidden || candidate.review.state !== 'approved' || !candidate.publication_record_file_id" @click="inspect(candidate)">{{ candidate.registration_request ? '核对／恢复原操作' : '核对入库范围' }}</button><button v-if="candidate.registration_status?.state === 'committed'" class="btn" :disabled="busy || hidden" @click="openResult(candidate)">查看已入库版本</button></div>
    <div v-if="selected && pack && owner" class="confirmation"><h3>{{ selected.name }}</h3><ul><li v-for="selection in pack.selections" :key="selection.version_key">{{ versionLabel(selection.version_key) }} · {{ selection.file_ids.length }} 个成果文件</li></ul><p class="muted">共 {{ pack.files.length }} 个文件，包含入库说明及明确随附资料。</p><p v-if="pending" class="muted">检测到原操作记录，将按原版本和范围恢复。</p><label><input v-model="acknowledged" type="checkbox" :disabled="busy || hidden">确认保存该次准备信息并前往数字世界</label><button class="btn primary" :disabled="busy || hidden || !acknowledged" @click="continueToWorld">{{ busy ? '正在处理…' : pending ? '恢复并前往数字世界' : '保存准备信息并继续' }}</button></div>
    <p v-if="busy" role="status" class="muted">正在核对固定版本…</p><p v-if="error" role="alert" class="error">{{ error }}</p>
    <div v-if="pending && !pending.prepared_revision_id && ['conflict','failed'].includes(pending.prepare_state || '')"><p class="muted">上次准备未完成，重新核对前会确认其结果。</p><button class="btn" :disabled="busy || hidden" @click="restartPreparation">按最新版本重新核对</button></div>
  </section>
</template>
<style scoped>
.panel { min-width: 0; padding: 20px; background: var(--shell-panel); border: 1px solid var(--line); border-radius: 8px; }h2 { font-size: 15px; margin-bottom: 14px; }h3 { font-size: 15px; margin: 14px 0; }.muted { color: var(--muted); font-size: 13px; line-height: 1.85; }.row,.confirmation { margin-top: 14px; padding-top: 14px; border-top: 1px solid var(--line); }.error { color: var(--danger); }ul { padding-left: 20px; margin: 12px 0; }label { display: flex; gap: 8px; align-items: start; margin: 14px 0; }.btn { margin-right: 8px; }
</style>
