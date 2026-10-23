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
    if (key === 'status' && !['all', 'ready', 'previs', 'missing'].includes(value)) continue
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

const materialTabs = new Set(['character', 'wardrobe', 'prop', 'location', 'sound', 'music'])
const extraQueryKeys = ['file', 'fixed', 'run', 'reference', 'candidate', 'find', 'fileType'] as const

function dramaFromProjects(path: string): DramaView | undefined {
  if (!path.startsWith('/projects?') && path !== '/projects') return
  const url = new URL(path, 'https://app.invalid')
  if (one(url.searchParams, 'category') !== 'ai_short_drama') return
  const canonical = canonicalDramaRoute(path)
  return canonical ? parseDramaPath(canonical) : undefined
}

/** In-project tabs and list filters stay on the short-drama path. Other categories return undefined. */
export function dramaBrowserTarget(currentPath: string, change: Record<string, string | undefined>): { path: string; query: Record<string, string> } | undefined {
  if (change.category && change.category !== 'ai_short_drama') return
  const current = parseDramaPath(currentPath) || dramaFromProjects(currentPath)
  if (!current && change.category !== 'ai_short_drama') return
  const carried = new URL(currentPath, 'https://app.invalid')
  const carriedProject = keep(one(carried.searchParams, 'project'), uuid)
  const view: DramaView = current ? { ...current, query: { ...current.query } } : { projectId: carriedProject, tab: carriedProject ? legacyTab(one(carried.searchParams, 'tab') || '', one(carried.searchParams, 'kind') || '') : 'list', kind: keep(one(carried.searchParams, 'kind'), keyPattern), query: { revision: keep(one(carried.searchParams, 'revision'), uuid) } }
  if ('project' in change) view.projectId = change.project || undefined
  if (!view.projectId) view.tab = 'list'
  else if ('tab' in change) {
    const tab = change.tab || 'overview'
    view.tab = tab === 'tasks' ? 'production' : tab === 'episodes' ? 'overview' : tab === 'materials' ? (materialTabs.has(change.kind || '') ? change.kind! : 'character') : tab
    if (materialTabs.has(view.tab)) view.kind = view.tab
    else if (view.tab === 'continuity') view.kind = 'continuity'
    else if (view.tab === 'script') view.kind = 'script'
    else view.kind = undefined
  }
  for (const key of ['kind', 'material', 'state', 'episode', 'scene', 'shot', 'deliverable', 'continuity', 'lighting', 'package', 'view'] as const) {
    if (key in change) view[key] = change[key] || undefined
  }
  if (view.projectId && materialTabs.has(view.tab)) view.kind = view.tab
  if ('q' in change) view.query.q = change.q || undefined
  else if (change.find) view.query.q = change.find
  for (const key of ['revision', 'version', 'status', 'sort', 'shown', 'deleted'] as const) {
    if (key in change) view.query[key] = change[key] || undefined
  }
  if (!view.projectId) {
    view.kind = view.material = view.state = view.episode = view.scene = view.shot = view.deliverable = view.continuity = view.lighting = view.package = view.view = undefined
    view.query.revision = undefined
    view.query.version = undefined
  }
  const href = dramaHref(view)
  const url = new URL(href, 'https://app.invalid')
  const query: Record<string, string> = Object.fromEntries(url.searchParams)
  const currentUrl = new URL(currentPath, 'https://app.invalid')
  for (const key of extraQueryKeys) {
    const value = key in change ? change[key] : currentUrl.searchParams.get(key) || undefined
    if (value) query[key] = value
    else delete query[key]
  }
  return { path: url.pathname, query }
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

const legacyParamKeys = ['project', 'tab', 'kind', 'material', 'state', 'episode', 'scene', 'shot', 'deliverable', 'continuity', 'lighting', 'package', 'view', 'compare', 'returnMaterial', 'find'] as const
const savedListQueries = new Map<string, { q?: string; status?: string; focusKey?: string }>()

function listMemoryKey(projectId: string | undefined, tab: string, kind?: string, revision?: string): string {
  const normalizedTab = kinds.has(tab) || tab === 'materials' ? `materials:${kind || tab}` : tab
  return `${projectId || 'list'}|${revision || 'head'}|${normalizedTab}`
}

export function rememberDramaListContext(view: DramaView, focusKey?: string) {
  if (view.material || view.shot || view.deliverable || view.continuity || view.lighting || view.package) return
  const key = listMemoryKey(view.projectId, view.tab, view.kind, view.query.revision)
  const prev = savedListQueries.get(key)
  savedListQueries.set(key, {
    q: view.query.q,
    status: view.query.status,
    focusKey: focusKey !== undefined ? focusKey : prev?.focusKey,
  })
}

export function recalledDramaListContext(view: DramaView): { q?: string; status?: string; focusKey?: string } | undefined {
  if (view.material || view.shot || view.deliverable || view.continuity || view.lighting || view.package) return undefined
  return savedListQueries.get(listMemoryKey(view.projectId, view.tab, view.kind, view.query.revision))
}

/** Old short-drama query addresses become pure paths. Other categories stay unchanged. */
export function canonicalDramaRoute(path: string): string | undefined {
  if (typeof path !== 'string' || !path.startsWith('/')) return
  const url = new URL(path, 'https://app.invalid')
  const isLegacyDramaQuery = url.pathname === '/ai_short_drama' && legacyParamKeys.some(k => url.searchParams.has(k))
  if (!isLegacyDramaQuery) {
    const parsed = parseDramaPath(path)
    if (parsed && path.startsWith('/ai_short_drama')) return dramaHref(parsed)
  }
  if (!isLegacyDramaQuery && (url.pathname !== '/projects' || one(url.searchParams, 'category') !== 'ai_short_drama')) return
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
  const listRecall = recalledDramaListContext({ tab: 'list', query: { revision: view.query.revision } })
  const items: { label: string; to: DramaView }[] = [
    { label: '项目管理', to: { tab: 'list', query: {} } },
    { label: '短剧制作', to: { tab: 'list', query: view.query.q || view.query.status ? { q: view.query.q, status: view.query.status } : { q: listRecall?.q, status: listRecall?.status } } },
  ]
  if (!view.projectId) return items
  const work = { label: names.work || '作品', to: { projectId: view.projectId, tab: 'overview', query: { revision: view.query.revision } } }
  items.push(work)
  const tabLabel: Record<string, string> = { overview: '概览', script: '剧本', continuity: '连续性', production: '镜头制作', delivery: '成果与交付', files: '文件', revisions: '版本', runs: '运行', references: '资产引用', character: '人物', wardrobe: '服装造型', prop: '道具', location: '场景', sound: '声音', music: '配乐' }
  if (view.tab === 'overview') return items
  if (shared.has(view.tab)) return [...items, { label: tabLabel[view.tab], to: { ...view, query: { revision: view.query.revision } } }]
  if (view.tab === 'production') {
    const prodRecall = recalledDramaListContext({ projectId: view.projectId, tab: 'production', query: { revision: view.query.revision } })
    items.push({ label: '镜头制作', to: { projectId: view.projectId, tab: 'production', query: { revision: view.query.revision, q: view.query.q || prodRecall?.q, status: view.query.status || prodRecall?.status } } })
    if (view.view === 'spaces') items.push({ label: '场景白模与灯光', to: view })
    if (names.episode) items.push({ label: names.episode, to: { projectId: view.projectId, tab: 'production', episode: view.episode, query: { revision: view.query.revision } } })
    if (names.scene) items.push({ label: names.scene, to: { projectId: view.projectId, tab: 'production', episode: view.episode, scene: view.scene, query: { revision: view.query.revision } } })
    if (names.shot) items.push({ label: names.shot, to: { ...view, query: { revision: view.query.revision } } })
    if (names.shot && names.version) items.push({ label: names.version, to: view })
    return items
  }
  if (view.tab === 'delivery') {
    items.push({ label: '成果与交付', to: { projectId: view.projectId, tab: 'delivery', query: { revision: view.query.revision } } })
    if (names.deliverable) items.push({ label: names.deliverable, to: { ...view, package: undefined, query: { revision: view.query.revision } } })
    if (names.deliverable && names.version && !view.package) items.push({ label: names.version, to: view })
    if (view.package) items.push({ label: '交付包', to: view })
    return items
  }
  if (view.tab === 'script' || view.tab === 'continuity' || (view.kind && scriptKinds.has(view.kind) && !view.material)) {
    items.push({ label: '剧本', to: { projectId: view.projectId, tab: 'script', kind: 'script', query: { revision: view.query.revision } } })
    if (view.continuity) {
      items.push({ label: '连续性', to: { projectId: view.projectId, tab: 'continuity', kind: 'continuity', query: { revision: view.query.revision } } })
      items.push({ label: names.shot || view.continuity, to: view })
    }
    else if (view.kind && view.kind !== 'script') items.push({ label: tabLabel[view.kind] || view.kind, to: view })
    else if (names.episode) items.push({ label: names.episode, to: view })
    return items
  }
  const kind = view.kind && kinds.has(view.kind) ? view.kind : view.tab
  const matRecall = recalledDramaListContext({ projectId: view.projectId, tab: kind, kind, query: { revision: view.query.revision } })
  const matQuery = { revision: view.query.revision, q: view.query.q || matRecall?.q, status: view.query.status || matRecall?.status }
  items.push({ label: '素材', to: { projectId: view.projectId, tab: kind, kind, query: matQuery } })
  items.push({ label: tabLabel[kind] || '素材', to: { projectId: view.projectId, tab: kind, kind, query: matQuery } })
  if (names.material) items.push({ label: names.material, to: { ...view, lighting: undefined, query: { revision: view.query.revision } } })
  if (names.state) items.push({ label: names.state, to: { ...view, lighting: undefined, query: { revision: view.query.revision } } })
  if (names.version && !view.lighting) items.push({ label: names.version, to: { ...view, lighting: undefined, query: { revision: view.query.revision, version: view.query.version } } })
  if (view.lighting) items.push({ label: '环境与灯光', to: view })
  return items
}
