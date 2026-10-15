import type { ProjectContent, ProjectFile, ProjectOpen } from './project-content.ts'

const stable = (value: unknown): string => JSON.stringify(value, (_key, item) => item && typeof item === 'object' && !Array.isArray(item) ? Object.fromEntries(Object.entries(item).sort(([a], [b]) => a.localeCompare(b))) : item)
export interface Change<T> { key: string; before?: T; after?: T }
function changes<T>(before: T[], after: T[], key: (value: T) => string): Change<T>[] {
  const old = new Map(before.map(value => [key(value), value])), next = new Map(after.map(value => [key(value), value]))
  return [...new Set([...old.keys(), ...next.keys()])].flatMap(key => stable(old.get(key)) === stable(next.get(key)) ? [] : [{ key, before: old.get(key), after: next.get(key) }])
}
export function revisionChanges(before: ProjectOpen | null, after: ProjectOpen) {
  if (!before && after.parent_revision_id) throw Error('父修订不可读，不能按初始修订计算变化。')
  if (before && (before.project_id !== after.project_id || after.parent_revision_id !== before.revision_id)) throw Error('修订变化必须对照同一项目的真实父修订。')
  const previous = before?.manifest
  return {
    files: changes<ProjectFile>(previous?.files || [], after.manifest.files, f => f.file_id),
    assets: changes(previous?.asset_refs || [], after.manifest.asset_refs || [], r => JSON.stringify([r.asset_id, r.purpose])),
    runs: changes(previous?.run_refs || [], after.manifest.run_refs || [], r => JSON.stringify([r.service_id, r.run_id])),
    documents: changes(previous?.domain_documents || [], after.manifest.domain_documents, d => d.document_type),
    selections: changes(previous?.selections || [], after.manifest.selections || [], s => s.purpose),
    name: previous?.name === after.manifest.name ? null : { before: previous?.name, after: after.manifest.name },
    entry: previous?.entry_document_id === after.manifest.entry_document_id ? null : { before: previous?.entry_document_id, after: after.manifest.entry_document_id },
  }
}
export interface BusinessValue { key: string; label: string; value: unknown }
function businessValues(content: ProjectContent | null): BusinessValue[] {
  if (!content) return []
  const out: BusinessValue[] = []
  for (const c of content.video?.cast || []) if (c.project_id) out.push({ key: 'cast/' + c.key, label: '片中角色 · ' + c.name, value: { project_id: c.project_id, revision_id: c.revision_id, role: c.role } })
  const drama = content.ai_short_drama
  if (drama) {
    for (const g of drama.globals) {
      if (g.music_source) out.push({ key: 'global/' + g.key + '/music-source', label: '配乐来源 · ' + g.name, value: g.music_source })
      if (g.asset_id) out.push({ key: 'global/' + g.key + '/source', label: '资料来源 · ' + g.name, value: { asset_id: g.asset_id, asset_version_id: g.asset_version_id } })
      if (g.selected_versions.length) out.push({ key: 'global/' + g.key + '/selected', label: '资料选用 · ' + g.name, value: g.selected_versions })
    }
    for (const ep of drama.episodes) for (const scene of ep.scenes) for (const shot of scene.shots) if (shot.selected_versions.length) out.push({ key: 'shot/' + ep.key + '/' + scene.key + '/' + shot.key, label: '镜头选用 · ' + shot.title, value: shot.selected_versions })
    for (const d of drama.deliverables) if (d.selected_version_key) out.push({ key: 'delivery/' + d.key, label: '成果选用 · ' + d.title, value: d.selected_version_key })
  }
  return out
}
export function businessChanges(before: ProjectContent | null, after: ProjectContent | null) {
  return changes(businessValues(before), businessValues(after), v => v.key)
}
