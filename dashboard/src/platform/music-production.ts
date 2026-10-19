import type { ContentRef, Detail, DramaReview, ProjectOpen } from './project-content.ts'

export type MusicGroup = 'voice' | 'songs' | 'mvs'
export interface MusicSource {
  project_id: string; revision_id: string; kind: 'recording' | 'model' | 'song'; item_key: string
  version_key: string | null; file_id: string | null
}
export interface MusicVersion {
  key: string; label: string; kind: 'model' | 'demo' | 'audio' | 'master' | 'video'
  basis_revision_id: string; parent_version_key: string | null; change_note: string
  file_refs: { file_id: string; content_ref: ContentRef; purpose: 'output' | 'sample' | 'attachment' }[]
  review: DramaReview; sources: MusicSource[]
}
export interface MusicProduction { versions: MusicVersion[]; selected_version_key: string | null }
// Rejected revisions remain immutable history but are not current listening choices.
export const visibleMusicVersions = (production?: MusicProduction) => (production?.versions || []).filter(v => v.review.state !== 'rejected')
export function currentMusicVersion(production?: MusicProduction, requested = '') {
  const versions = visibleMusicVersions(production)
  if (requested) return versions.find(v => v.key === requested)
  return versions.find(v => v.key === production?.selected_version_key) || versions[0]
}
export function currentMusicOutput(entry: Pick<Detail, 'production' | 'result_file_ids'>) {
  if (!entry.production?.versions.length) return entry.result_file_ids[0]
  return currentMusicVersion(entry.production)?.file_refs.find(f => f.purpose === 'output')?.file_id
}
export const musicVersionKinds = { model: '人声模型', demo: 'Demo', audio: '音频版本', master: '母带', video: 'MV' }
export const musicReview = (review: DramaReview) => ({ unreviewed: '未审核', in_review: '审核中', approved: '已有审核记录', rejected: '需修改' })[review.state]
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const key = /^[a-zA-Z0-9_-]+$/
const fail = (): never => { throw Error('音乐制作版本或固定来源不正确。') }
const object = (value: unknown): Record<string, any> => value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, any> : fail()
const exact = (value: Record<string, any>, keys: string[]) => { if (Object.keys(value).sort().join() !== keys.sort().join()) fail() }
const list = (value: unknown): unknown[] => Array.isArray(value) && value.length <= 1000 ? value : fail()
const text = (v: unknown): string => typeof v === 'string' && v.trim() ? v : fail()
const identifier = (v: unknown): string => typeof v === 'string' && uuid.test(v) ? v : fail()
const stableKey = (v: unknown): string => typeof v === 'string' && key.test(v) ? v : fail()

export function parseMusicProduction(value: unknown, project: ProjectOpen, group: MusicGroup): MusicProduction | undefined {
  if (value === undefined) return
  const p = object(value); exact(p, ['versions', 'selected_version_key'])
  const allowed = group === 'voice' ? ['model'] : group === 'mvs' ? ['video'] : ['demo', 'audio', 'master']
  const versions: MusicVersion[] = list(p.versions).map(item => {
    const v = object(item); exact(v, ['key', 'label', 'kind', 'basis_revision_id', 'parent_version_key', 'change_note', 'file_refs', 'review', 'sources'])
    if (!allowed.includes(v.kind)) fail()
    const review = object(v.review); exact(review, ['state', 'record_file_id'])
    if (!['unreviewed', 'in_review', 'approved', 'rejected'].includes(review.state)) fail()
    if (review.record_file_id !== null && !project.manifest.files.some(f => f.file_id === identifier(review.record_file_id))) fail()
    const file_refs = list(v.file_refs).map(value => {
      const f = object(value); exact(f, ['file_id', 'content_ref', 'purpose'])
      const current = project.manifest.files.find(file => file.file_id === identifier(f.file_id)), ref = object(f.content_ref)
      exact(ref, ['store_id', 'artifact_id', 'version_id'])
      if (!current) return fail()
      if (!['output', 'sample', 'attachment'].includes(f.purpose) || ['store_id', 'artifact_id', 'version_id'].some(k => current.content_ref[k as keyof ContentRef] !== identifier(ref[k]))) fail()
      return { file_id: current.file_id, content_ref: { ...current.content_ref }, purpose: f.purpose as MusicVersion['file_refs'][number]['purpose'] }
    })
    if (!file_refs.length || !file_refs.some(f => f.purpose === 'output') || new Set(file_refs.map(f => f.file_id)).size !== file_refs.length) fail()
    const sources: MusicSource[] = list(v.sources).map(value => {
      const s = object(value); exact(s, ['project_id', 'revision_id', 'kind', 'item_key', 'version_key', 'file_id'])
      const sourceKinds = group === 'voice' ? ['recording'] : group === 'songs' ? ['recording', 'model'] : ['song']
      if (!sourceKinds.includes(s.kind) || (s.version_key === null) === (s.file_id === null)) fail()
      if (s.kind === 'recording' && s.version_key !== null) fail()
      return { project_id: identifier(s.project_id), revision_id: identifier(s.revision_id), kind: s.kind, item_key: stableKey(s.item_key), version_key: s.version_key === null ? null : stableKey(s.version_key), file_id: s.file_id === null ? null : identifier(s.file_id) }
    })
    if (new Set(sources.map(s => JSON.stringify([s.project_id, s.revision_id, s.kind, s.item_key, s.version_key, s.file_id]))).size !== sources.length) fail()
    return { key: stableKey(v.key), label: text(v.label), kind: v.kind, basis_revision_id: identifier(v.basis_revision_id), parent_version_key: v.parent_version_key === null ? null : stableKey(v.parent_version_key), change_note: text(v.change_note), file_refs, review: { state: review.state, record_file_id: review.record_file_id }, sources }
  })
  if (new Set(versions.map(v => v.key)).size !== versions.length) fail()
  const index = new Map(versions.map(v => [v.key, v])), visited = new Set<string>(), active = new Set<string>()
  function visit(v: MusicVersion) {
    if (active.has(v.key)) fail()
    if (visited.has(v.key)) return
    active.add(v.key)
    if (v.parent_version_key !== null) { const parent = index.get(v.parent_version_key); if (!parent) return fail(); visit(parent) }
    active.delete(v.key); visited.add(v.key)
  }
  versions.forEach(visit)
  const selected = p.selected_version_key === null ? null : stableKey(p.selected_version_key)
  if (selected !== null && !index.has(selected)) fail()
  return { versions, selected_version_key: selected }
}

export function musicEntry(music: { voice: { models: Detail[]; recordings: Detail[] }; songs: Detail[]; mvs: Detail[] } | undefined, group: MusicGroup | 'recordings', item: string) {
  return (group === 'voice' ? music?.voice.models : group === 'recordings' ? music?.voice.recordings : music?.[group])?.find(e => e.key === item)
}
export const sourceGroup = (source: MusicSource): MusicGroup | 'recordings' => source.kind === 'model' ? 'voice' : source.kind === 'recording' ? 'recordings' : 'songs'
export function musicSourceEntry(music: Parameters<typeof musicEntry>[0], source: MusicSource) {
  const entry = musicEntry(music, sourceGroup(source), source.item_key)
  if (!entry) throw Error('来源固定修订没有对应业务对象。')
  if (source.version_key !== null ? !entry.production?.versions.some(v => v.key === source.version_key) : ![...entry.result_file_ids, ...entry.file_ids].includes(source.file_id!)) throw Error('来源文件或版本不属于指定业务对象。')
  return entry
}
export function musicVersionRoute(query: Record<string, any>, source: MusicSource) {
  return { path: '/projects', query: { category: 'music', project: source.project_id, revision: source.revision_id, tab: source.file_id ? 'files' : sourceGroup(source), item: source.file_id ? undefined : source.item_key, version: source.version_key || undefined, file: source.file_id || undefined, q: query.q, sort: query.sort } }
}

export function musicReturnRoute(value: unknown): string | undefined {
  if (typeof value !== 'string' || value.length > 4096 || !value.startsWith('/projects?') || /[\\\r\n]/.test(value)) return
  const url = new URL(value, 'https://studio.invalid')
  if (url.origin !== 'https://studio.invalid' || url.pathname !== '/projects' || url.hash || url.searchParams.get('category') !== 'music' || !uuid.test(url.searchParams.get('project') || '') || !uuid.test(url.searchParams.get('revision') || '')) return
  let duplicate = false
  url.searchParams.forEach((_value, k) => { if (url.searchParams.getAll(k).length !== 1) duplicate = true })
  if (duplicate) return
  url.searchParams.delete('fromMusic')
  return url.pathname + url.search
}
