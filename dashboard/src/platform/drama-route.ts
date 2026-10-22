const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const keyPattern = /^[A-Za-z0-9_-]+$/
const kinds = new Set(['character', 'wardrobe', 'prop', 'location', 'sound', 'music'])
const scriptKinds = new Set(['character', 'location', 'wardrobe', 'prop', 'continuity', 'sound', 'music'])
const shared = new Set(['files', 'revisions', 'runs', 'references'])
const queryKeys = ['revision', 'version', 'q', 'status', 'sort', 'shown', 'deleted'] as const

export interface DramaQuery { revision?: string; version?: string; q?: string; status?: string; sort?: string; shown?: string; deleted?: string }
export interface DramaView {
  projectId?: string
  tab: string
  kind?: string
  material?: string
  state?: string
  episode?: string
  scene?: string
  shot?: string
  deliverable?: string
  continuity?: string
  lighting?: string
  package?: string
  view?: string
  query: DramaQuery
}

export function isDramaKey(value: string) { return keyPattern.test(value) && value.length <= 64 }
function one(params: URLSearchParams, key: string) { const values = params.getAll(key); return values.length === 1 ? values[0] : undefined }
function keep(value: string | undefined, pattern: RegExp) { return value && pattern.test(value) ? value : undefined }

export function dramaQuery(params: URLSearchParams): DramaQuery {
  const query: DramaQuery = {}
  for (const key of queryKeys) {
    const value = one(params, key)
    if (!value) continue
    if (key === 'revision' && !uuid.test(value)) continue
    if (key === 'version' && !isDramaKey(value)) continue
    if (key === 'status' && !['all', 'ready', 'missing'].includes(value)) continue
    if (key === 'sort' && value !== 'name') continue
    if (key === 'shown' && !/^\d{1,6}$/.test(value)) continue
    if (key === 'deleted' && value !== 'true') continue
    if (key === 'q' && value.length > 256) continue
    query[key] = value
  }
  const find = one(params, 'find')
  if (!query.q && find && find.length <= 256) query.q = find
  return query
}

function blank(projectId: string | undefined, query: DramaQuery): DramaView {
  return { projectId, tab: projectId ? 'overview' : 'list', query }
}

/** Parse a short-drama pure path. Other project paths return undefined. */
export function parseDramaPath(path: string): DramaView | undefined {
  if (typeof path !== 'string' || path.length > 2048 || !path.startsWith('/ai_short_drama')) return
  const url = new URL(path, 'https://app.invalid')
  if (url.origin !== 'https://app.invalid') return
  const parts = url.pathname.split('/').filter(Boolean)
  if (parts[0] !== 'ai_short_drama' || parts.length > 8) return
  const query = dramaQuery(url.searchParams)
  if (parts.length === 1) return blank(undefined, query)
  const projectId = parts[1]
  if (!uuid.test(projectId)) return blank(undefined, query)
  const rest = parts.slice(2)
  if (!rest.length) return blank(projectId, query)
  const [a, b, c, d, e, f] = rest
  if (shared.has(a) && rest.length === 1) return { projectId, tab: a, query }
  if (a === 'script') return parseScript(projectId, rest.slice(1), query)
  if (a === 'materials') return parseMaterial(projectId, rest.slice(1), query)
  if (a === 'production') return parseProduction(projectId, rest.slice(1), query)
  if (a === 'delivery') return parseDelivery(projectId, rest.slice(1), query)
  if ([a, b, c, d, e, f].some(part => part && !isDramaKey(part))) return blank(projectId, query)
  return blank(projectId, query)
}

function parseScript(projectId: string, parts: string[], query: DramaQuery): DramaView {
  if (!parts.length || parts[0] === 'body' && parts.length === 1) return { projectId, tab: 'script', kind: 'script', query }
  if (parts[0] === 'body' && parts.slice(1).every(isDramaKey)) return { projectId, tab: 'script', kind: 'script', episode: parts[1], scene: parts[2], query }
  if (parts[0] === 'continuity' && parts.length === 2 && isDramaKey(parts[1])) return { projectId, tab: 'continuity', kind: 'continuity', continuity: parts[1], query }
  if (parts.length === 1 && scriptKinds.has(parts[0])) return { projectId, tab: parts[0] === 'continuity' ? 'continuity' : 'script', kind: parts[0], query }
  return { projectId, tab: 'script', kind: 'script', query }
}

function parseMaterial(projectId: string, parts: string[], query: DramaQuery): DramaView {
  if (!parts.length || !kinds.has(parts[0]) || parts.slice(1).some(part => part !== 'lighting' && !isDramaKey(part))) return { projectId, tab: 'character', kind: 'character', query }
  const [kind, material, state, tail] = parts
  const lighting = kind === 'location' && tail === 'lighting' ? state : undefined
  return { projectId, tab: kind, kind, material, state: lighting ? state : state, lighting, query }
}

function parseProduction(projectId: string, parts: string[], query: DramaQuery): DramaView {
  if (parts[0] === 'spaces' && parts.length === 1) return { projectId, tab: 'production', view: 'spaces', query }
  if (parts[0] !== 'shots' || parts.slice(1).some(part => !isDramaKey(part))) return { projectId, tab: 'production', query }
  return { projectId, tab: 'production', episode: parts[1], scene: parts[2], shot: parts[3], query }
}

function parseDelivery(projectId: string, parts: string[], query: DramaQuery): DramaView {
  if (parts.some(part => part !== 'package' && !isDramaKey(part))) return { projectId, tab: 'delivery', query }
  return { projectId, tab: 'delivery', deliverable: parts[0], package: parts[1] === 'package' ? parts[0] : undefined, query }
}

export function dramaHref(view: DramaView): string {
  const params = new URLSearchParams()
  for (const key of queryKeys) if (view.query[key]) params.set(key, view.query[key]!)
  const search = params.toString()
  return dramaPath(view) + (search ? '?' + search : '')
}

export function dramaPath(view: DramaView): string {
  if (!view.projectId) return '/ai_short_drama'
  const base = '/ai_short_drama/' + view.projectId
  if (shared.has(view.tab)) return `${base}/${view.tab}`
  if (view.tab === 'overview' || !view.tab) return base
  if (view.tab === 'delivery') return view.deliverable ? `${base}/delivery/${view.deliverable}${view.package ? '/package' : ''}` : `${base}/delivery`
  if (view.tab === 'production' && view.view === 'spaces') return `${base}/production/spaces`
  if (view.tab === 'production') {
    const parts = [view.episode, view.scene, view.shot].filter(Boolean)
    return `${base}/production/shots${parts.length ? '/' + parts.join('/') : ''}`
  }
  if (view.kind === 'continuity' || view.tab === 'continuity') return view.continuity ? `${base}/script/continuity/${view.continuity}` : `${base}/script/continuity`
  if (view.tab === 'script' || view.kind === 'script') {
    const parts = [view.episode, view.scene].filter(Boolean)
    return `${base}/script/body${parts.length ? '/' + parts.join('/') : ''}`
  }
  if ((view.tab === 'script' || view.tab === 'continuity') && view.kind && scriptKinds.has(view.kind) && !view.material) return `${base}/script/${view.kind}`
  const kind = view.kind && kinds.has(view.kind) ? view.kind : 'character'
  let path = `${base}/materials/${kind}`
  if (view.material) path += '/' + view.material
  if (view.state) path += '/' + view.state
  if (view.lighting) path += '/lighting'
  return path
}

/** Old short-drama query addresses become pure paths. Other categories stay unchanged. */
export function canonicalDramaRoute(path: string): string | undefined {
  if (typeof path !== 'string' || !path.startsWith('/')) return
  const parsed = parseDramaPath(path)
  if (parsed && path.startsWith('/ai_short_drama')) return dramaHref(parsed)
  const url = new URL(path, 'https://app.invalid')
  if (url.pathname !== '/projects' || one(url.searchParams, 'category') !== 'ai_short_drama') return
  const projectId = keep(one(url.searchParams, 'project'), uuid)
  const query = dramaQuery(url.searchParams)
  const tab = one(url.searchParams, 'tab') || ''
  const kind = one(url.searchParams, 'kind') || ''
  const view: DramaView = { projectId, tab: projectId ? legacyTab(tab, kind) : 'list', query }
  if (!projectId) return dramaHref(view)
  view.kind = keep(kind, keyPattern)
  view.material = keep(one(url.searchParams, 'material'), keyPattern)
  view.state = keep(one(url.searchParams, 'state'), keyPattern)
  view.episode = keep(one(url.searchParams, 'episode'), keyPattern)
  view.scene = keep(one(url.searchParams, 'scene'), keyPattern)
  view.shot = keep(one(url.searchParams, 'shot'), keyPattern)
  view.deliverable = keep(one(url.searchParams, 'deliverable'), keyPattern)
  view.continuity = keep(one(url.searchParams, 'continuity'), keyPattern)
  view.lighting = keep(one(url.searchParams, 'lighting'), keyPattern)
  view.package = one(url.searchParams, 'package') === view.deliverable ? view.deliverable : undefined
  view.view = one(url.searchParams, 'view') === 'spaces' ? 'spaces' : undefined
  if (['character', 'wardrobe', 'prop', 'location', 'sound', 'music'].includes(tab) && !view.kind) view.kind = tab
  return dramaHref(view)
}

function legacyTab(tab: string, kind: string) {
  if (!tab || tab === 'overview' || tab === 'episodes') return 'overview'
  if (tab === 'materials') return kinds.has(kind) ? kind : 'character'
  if (tab === 'script' && scriptKinds.has(kind)) return kind === 'continuity' ? 'continuity' : 'script'
  if (tab === 'continuity') return 'continuity'
  if (['script', 'production', 'delivery', 'files', 'revisions', 'runs', 'references'].includes(tab)) return tab
  if (kinds.has(tab)) return tab
  return 'overview'
}

export function dramaBreadcrumb(view: DramaView, names: { work?: string; material?: string; episode?: string; scene?: string; shot?: string; deliverable?: string; state?: string; version?: string }) {
  const items: { label: string; to: DramaView }[] = [
    { label: '项目管理', to: { tab: 'list', query: {} } },
    { label: '短剧制作', to: { tab: 'list', query: view.query.q || view.query.status ? { q: view.query.q, status: view.query.status } : {} } },
  ]
  if (!view.projectId) return items
  const work = { label: names.work || '作品', to: { projectId: view.projectId, tab: 'overview', query: { revision: view.query.revision } } }
  items.push(work)
  const tabLabel: Record<string, string> = { overview: '概览', script: '剧本', continuity: '连续性', production: '镜头制作', delivery: '成果与交付', files: '文件', revisions: '版本', runs: '运行', references: '资产引用', character: '人物', wardrobe: '服装造型', prop: '道具', location: '场景', sound: '声音', music: '配乐' }
  if (view.tab === 'overview') return items
  if (shared.has(view.tab)) return [...items, { label: tabLabel[view.tab], to: { ...view, query: { revision: view.query.revision } } }]
  if (view.tab === 'production') {
    items.push({ label: '镜头制作', to: { projectId: view.projectId, tab: 'production', query: { revision: view.query.revision, q: view.query.q, status: view.query.status } } })
    if (view.view === 'spaces') items.push({ label: '场景白模与灯光', to: view })
    if (names.episode) items.push({ label: names.episode, to: { projectId: view.projectId, tab: 'production', episode: view.episode, query: { revision: view.query.revision } } })
    if (names.scene) items.push({ label: names.scene, to: { projectId: view.projectId, tab: 'production', episode: view.episode, scene: view.scene, query: { revision: view.query.revision } } })
    if (names.shot) items.push({ label: names.version || names.shot, to: view })
    return items
  }
  if (view.tab === 'delivery') {
    items.push({ label: '成果与交付', to: { projectId: view.projectId, tab: 'delivery', query: { revision: view.query.revision } } })
    if (names.deliverable) items.push({ label: names.version || names.deliverable, to: view })
    return items
  }
  if (view.tab === 'script' || view.tab === 'continuity' || (view.kind && scriptKinds.has(view.kind) && !view.material)) {
    items.push({ label: '剧本', to: { projectId: view.projectId, tab: 'script', kind: 'script', query: { revision: view.query.revision } } })
    if (view.continuity) items.push({ label: names.shot || '连续性', to: view })
    else if (view.kind && view.kind !== 'script') items.push({ label: tabLabel[view.kind] || view.kind, to: view })
    else if (names.episode) items.push({ label: names.episode, to: view })
    return items
  }
  const kind = view.kind && kinds.has(view.kind) ? view.kind : view.tab
  items.push({ label: '素材', to: { projectId: view.projectId, tab: kind, kind, query: { revision: view.query.revision, q: view.query.q, status: view.query.status } } })
  items.push({ label: tabLabel[kind] || '素材', to: { projectId: view.projectId, tab: kind, kind, query: { revision: view.query.revision, q: view.query.q, status: view.query.status } } })
  if (names.material) items.push({ label: names.material, to: { ...view, lighting: undefined, query: { revision: view.query.revision } } })
  if (names.state) items.push({ label: names.version || names.state, to: { ...view, lighting: undefined, query: { revision: view.query.revision, version: names.version ? view.query.version : undefined } } })
  if (view.lighting) items.push({ label: '环境与灯光', to: view })
  return items
}
