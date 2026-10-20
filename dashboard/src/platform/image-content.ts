import type { ContentRef, ProjectOpen } from './project-content.ts'
export interface ImageVersion {
  key: string; label: string; parent_version_key: string | null; change_note: string
  file_refs: { file_id: string; content_ref: ContentRef; purpose: 'output' | 'intermediate' | 'record' }[]
  reference_file_ids: string[]; prompt_file_ids: string[]
}
export interface ImageWork { key: string; title: string; overview: string | null; reference_file_ids: string[]; display_version_key: string | null; versions: ImageVersion[] }
export interface ImageContent { works: ImageWork[]; reference_file_ids: string[]; process_file_ids: string[] }
const invalid = (): never => { throw new Error('图像项目业务内容格式或固定版本引用不正确。') }
const object = (value: unknown, allowed: string[]): Record<string, unknown> => {
  if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).some(k => !allowed.includes(k))) return invalid()
  return value as Record<string, unknown>
}
const text = (value: unknown, max: number): string => typeof value === 'string' && /\S/.test(value) && value.length <= max ? value : invalid()
const key = (value: unknown): string => typeof value === 'string' && /^[a-zA-Z0-9_-]{1,128}$/.test(value) ? value : invalid()
const array = <T>(value: unknown, parse: (v: unknown) => T, optional = false): T[] => optional && value === undefined ? [] : Array.isArray(value) && value.length <= 1000 ? value.map(parse) : invalid()
const distinct = <T>(items: T[], keys: unknown[] = items): T[] => new Set(keys).size === keys.length ? items : invalid()
export function parseImageContent(value: unknown, project: ProjectOpen): ImageContent {
  const files = new Map(project.manifest.files.map(f => [f.file_id, f]))
  const file = (v: unknown): string => {
    if (typeof v !== 'string' || !files.has(v)) throw new Error('业务内容引用的文件不在当前项目修订中。')
    return v
  }
  const refs = (v: unknown, optional = false) => distinct(array(v, file, optional))
  const body = object(value, ['works', 'reference_file_ids', 'process_file_ids'])
  const works = array(body.works, item => {
    const w = object(item, ['key', 'title', 'overview', 'reference_file_ids', 'display_version_key', 'versions'])
    const versions = array(w.versions, item => {
      const v = object(item, ['key', 'label', 'parent_version_key', 'change_note', 'file_refs', 'reference_file_ids', 'prompt_file_ids'])
      const file_refs = array(v.file_refs, item => {
        const f = object(item, ['file_id', 'content_ref', 'purpose']), id = file(f.file_id)
        const c = object(f.content_ref, ['store_id', 'artifact_id', 'version_id']), actual = files.get(id)!.content_ref
        if (c.store_id !== actual.store_id || c.artifact_id !== actual.artifact_id || c.version_id !== actual.version_id || !['output', 'intermediate', 'record'].includes(String(f.purpose))) invalid()
        return { file_id: id, content_ref: { ...actual }, purpose: f.purpose as ImageVersion['file_refs'][number]['purpose'] }
      })
      distinct(file_refs, file_refs.map(f => f.file_id))
      if (!file_refs.some(f => f.purpose === 'output')) invalid()
      return { key: key(v.key), label: text(v.label, 256), parent_version_key: v.parent_version_key === null ? null : key(v.parent_version_key), change_note: text(v.change_note, 16000), file_refs, reference_file_ids: refs(v.reference_file_ids), prompt_file_ids: refs(v.prompt_file_ids) }
    })
    distinct(versions, versions.map(v => v.key))
    const index = new Map(versions.map(v => [v.key, v]))
    const display = w.display_version_key == null ? null : key(w.display_version_key)
    if (display && !index.has(display)) invalid()
    for (const v of versions) {
      const seen = new Set<string>(); let current: string | null = v.key
      while (current !== null) {
        if (seen.has(current) || !index.has(current)) invalid()
        seen.add(current); current = index.get(current)!.parent_version_key
      }
    }
    if (w.overview != null && (typeof w.overview !== 'string' || w.overview.length > 16000)) invalid()
    return { key: key(w.key), title: text(w.title, 256), overview: w.overview as string | null ?? null, reference_file_ids: refs(w.reference_file_ids, true), display_version_key: display, versions }
  }, true)
  distinct(works, works.map(w => w.key))
  return { works, reference_file_ids: refs(body.reference_file_ids, true), process_file_ids: refs(body.process_file_ids, true) }
}
export const imageDisplayVersion = (work: ImageWork): ImageVersion | undefined => work.versions.find(v => v.key === work.display_version_key)
