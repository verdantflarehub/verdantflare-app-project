import type { ProjectOpen } from './project-content'
const isProjectID = (value: string) => /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(value)

/** Only follow verified parents within this project; a malformed parent is never sent. */
export async function readProjectHistory(projectID: string, head: string, read: (revision: string) => Promise<ProjectOpen>, active: () => boolean, append: (revision: ProjectOpen) => void): Promise<void> {
  const seen = new Set<string>()
  let next = head
  while (next) {
    if (!active()) return
    if (!isProjectID(next) || seen.has(next)) throw new Error('修订关系不正确，请重试。')
    seen.add(next)
    const item = await read(next)
    if (!active()) return
    if (item.project_id !== projectID || item.revision_id !== next || item.manifest?.project_id !== projectID || typeof item.manifest.name !== 'string' || !Array.isArray(item.manifest.files)) throw new Error('修订内容与当前项目不匹配。')
    if (item.parent_revision_id !== undefined && item.parent_revision_id !== '' && !isProjectID(item.parent_revision_id)) throw new Error('项目父修订信息不正确。')
    append(item)
    if (item.parent_revision_id === undefined) throw new Error('当前服务未提供完整修订关系。')
    next = item.parent_revision_id
  }
}
