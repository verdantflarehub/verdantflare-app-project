import type { ContentRef, DramaGlobal, DramaDeliverable, DramaWorldCandidate, ProjectOpen, WorldRegistration, WorldRegistrationStatus, WorldResult } from './project-content'

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const fail = (message = '入库说明与固定版本不一致，请经 MCP 核对。'): never => { throw new Error(message) }
export const canonical = (value: unknown): string => JSON.stringify(value, (_key, v) => v && typeof v === 'object' && !Array.isArray(v) ? Object.fromEntries(Object.entries(v).sort(([a], [b]) => a.localeCompare(b))) : v)
const obj = (value: unknown, keys: string[]) => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fail()
  const o = value as Record<string, unknown>
  if (Object.keys(o).some(k => !keys.includes(k))) fail()
  return o
}
export interface PublicationSelection { version_key: string; file_ids: string[] }
export interface PublicationPackage { files: string[]; selections: PublicationSelection[] }

/** Validates only the package scope. Server authority and World outcomes remain authoritative. */
export function publicationPackage(raw: string, candidate: DramaWorldCandidate, owner: DramaGlobal | DramaDeliverable, project: ProjectOpen): PublicationPackage {
  let value: unknown
  try { value = JSON.parse(raw) } catch { return fail() }
  const r = obj(value, ['schema_version', 'kind', 'source', 'versions', 'supporting_file_refs'])
  if (r.schema_version !== 1 || r.kind !== 'short-drama-publication' || !Array.isArray(r.versions) || !r.versions.length || !Array.isArray(r.supporting_file_refs)) fail()
  if (canonical(r.source) !== canonical(candidate.source) || !candidate.publication_record_file_id) fail()
  const files = new Set<string>([candidate.publication_record_file_id!]), seen = new Set<string>(), support = new Set<string>(), required = new Set<string>(), selections: PublicationSelection[] = []
  const readFile = (raw: unknown) => {
    const file = obj(raw, ['file_id', 'content_ref', 'purpose'])
    if (typeof file.file_id !== 'string' || typeof file.purpose !== 'string' || !/^[a-zA-Z0-9_-]+$/.test(file.purpose)) return fail()
    const actual = project.manifest.files.find(f => f.file_id === file.file_id)
    if (!actual || files.has(file.file_id) || canonical(actual.content_ref) !== canonical(file.content_ref)) return fail()
    files.add(file.file_id)
    return { file_id: file.file_id, purpose: file.purpose, content_ref: file.content_ref as ContentRef }
  }
  for (const entry of r.versions as unknown[]) {
    const selection = obj(entry, ['version_key', 'file_refs'])
    const version = owner.versions.find(v => v.key === selection.version_key)
    if (!version || seen.has(version.key) || !Array.isArray(selection.file_refs) || !selection.file_refs.length) fail()
    seen.add(version!.key)
    const ids = (selection.file_refs as unknown[]).map(raw => {
      const file = readFile(raw)
      if (!version!.file_refs.some(f => canonical(f) === canonical(file))) fail()
      if (file.purpose === 'whitebox') required.add(version!.spatial_record_file_id || '')
      if (file.purpose === 'lighting') required.add(version!.lighting_record_file_id || '')
      return file.file_id
    })
    selections.push({ version_key: version!.key, file_ids: ids })
    for (const id of [version!.change_record_file_id, version!.spatial_record_file_id, version!.lighting_record_file_id, version!.composition_file_id, version!.review.record_file_id, ...version!.generation_record_file_ids]) if (id) support.add(id)
    if ('states' in owner) for (const ref of owner.states.find(s => s.key === version!.state_key)?.definition_refs || []) support.add(ref.file_id)
  }
  if ('definition_refs' in owner) for (const ref of owner.definition_refs) support.add(ref.file_id)
  for (const entry of r.supporting_file_refs as unknown[]) if (!support.has(readFile(entry).file_id)) fail()
  if ([...required].some(id => !id || !files.has(id)) || !project.manifest.files.some(f => f.file_id === candidate.publication_record_file_id)) fail()
  return { files: [...files].sort(), selections }
}
