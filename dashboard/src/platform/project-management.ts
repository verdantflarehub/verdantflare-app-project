import type { Category, ProjectItem } from './project-content.ts'

export const projectCategories = [
  { key: 'character', label: '角色设计', description: '管理角色身份、设定图和形象扩展。' },
  { key: 'music', label: '音乐制作', description: '管理专辑中的人声、歌曲与 MV。' },
  { key: 'video', label: '创意视频', description: '管理故事板、片中角色和成片。' },
  { key: 'ai_short_drama', label: '短剧制作', description: '管理剧本、人物设定、镜头制作与交付。' },
  { key: 'creative_video', label: '创意视频制作', description: '管理任务书、创意分镜、剪辑版本与交付审核。' },
] as const
export const commonMenus = [
  { key: 'files', label: '文件' }, { key: 'revisions', label: '版本' },
  { key: 'runs', label: '运行' }, { key: 'references', label: '资产引用' },
]
const typeMenus: Record<Category, { key: string; label: string }[]> = {
  character: [{ key: 'setting', label: '角色设定' }, { key: 'extensions', label: '角色扩展' }],
  music: [{ key: 'voice', label: '人声训练' }, { key: 'songs', label: '歌曲创作' }, { key: 'mvs', label: 'MV创作' }],
  video: [{ key: 'storyboards', label: '故事板' }, { key: 'cast', label: '角色创作' }],
  ai_short_drama: [
    { key: 'script', label: '剧本正文' }, { key: 'character', label: '人物' }, { key: 'wardrobe', label: '服装造型' },
    { key: 'prop', label: '道具' }, { key: 'location', label: '场景' }, { key: 'continuity', label: '连续性' },
    { key: 'sound', label: '声音' }, { key: 'music', label: '配乐' }, { key: 'production', label: '镜头制作' },
    { key: 'delivery', label: '成果与交付' },
  ],
  creative_video: [
    { key: 'overview', label: '概览' }, { key: 'brief', label: '任务书' },
    { key: 'story', label: '创意与分镜' }, { key: 'shots', label: '镜头素材' },
    { key: 'edit', label: '剪辑版本' }, { key: 'delivery', label: '交付与审核' },
  ],
}
export function isCategory(value: string): value is Category { return projectCategories.some(c => c.key === value) }
export function projectMenus(category: string) { return [...(isCategory(category) ? typeMenus[category] : []), ...commonMenus] }
/** Old links remain valid; the displayed navigation always uses the new business hierarchy. */
export function businessKey(category: string, tab: string, kind = ''): string {
  if (category === 'ai_short_drama') {
    if (tab === 'overview' || tab === 'episodes') return 'script'
    if (tab === 'tasks') return 'production'
    if (tab === 'materials') return ['character', 'wardrobe', 'prop', 'location', 'sound', 'music'].includes(kind) ? kind : 'character'
    if (tab === 'script' && ['character', 'wardrobe', 'prop', 'location', 'continuity', 'sound', 'music'].includes(kind)) return kind
  }
  return projectMenus(category).find(m => m.key === tab)?.key || projectMenus(category)[0]?.key || 'files'
}
export function selectProjects(items: ProjectItem[], search: string, sort: string): ProjectItem[] {
  const needle = search.trim().toLocaleLowerCase()
  return items.filter(item => item.name.toLocaleLowerCase().includes(needle)).sort((a, b) => {
    const order = sort === 'name' ? a.name.localeCompare(b.name, 'zh-CN') : (Date.parse(b.updated_at || '') || 0) - (Date.parse(a.updated_at || '') || 0)
    return order || a.project_id.localeCompare(b.project_id)
  })
}
export type ListView = { q?: string; sort?: string; shown?: string }
export function restoredListView(to: Record<string, unknown>, from: Record<string, unknown>, views: ReadonlyMap<string, ListView>): ListView | undefined {
  if (to.project || to.q || to.sort || to.shown || (!from.project && from.category === to.category)) return
  const view = views.get(String(to.category))
  return view && Object.values(view).some(Boolean) ? view : undefined
}
/** Complete the authorized category before sorting/counting; never claim a partial page is the full result. */
export async function collectProjects(category: Category, read: (args: Record<string, unknown>) => Promise<{ items: ProjectItem[]; next_cursor?: string | null }>, active: () => boolean): Promise<ProjectItem[]> {
  const projects = new Map<string, ProjectItem>(), cursors = new Set<string>()
  let cursor = ''
  do {
    if (!active()) throw new DOMException('查询已取消。', 'AbortError')
    const page = await read({ category, limit: 100, ...(cursor ? { cursor } : {}) })
    if (!active()) throw new DOMException('查询已取消。', 'AbortError')
    if (!Array.isArray(page.items) || page.items.length > 100 || page.items.some(p => p.category !== category || typeof p.name !== 'string' || !validID(p.project_id) || !validID(p.head_revision_id))) throw new Error('项目列表返回内容不正确。')
    if (page.next_cursor != null && typeof page.next_cursor !== 'string') throw new Error('项目分页信息不正确。')
    if (page.items.some(p => (p.can_write !== undefined && typeof p.can_write !== 'boolean') || (p.updated_at !== undefined && typeof p.updated_at !== 'string'))) throw new Error('项目资料返回内容不正确。')
    for (const item of page.items) projects.set(item.project_id, item)
    cursor = page.next_cursor || ''
    if (cursor && (!validID(cursor) || cursors.has(cursor))) throw new Error('项目分页没有继续，请刷新后重试。')
    cursors.add(cursor)
  } while (cursor)
  return [...projects.values()]
}
const validID = (id: unknown): id is string => typeof id === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(id)
