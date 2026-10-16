const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const projectCategories = new Set(['character', 'music', 'video', 'ai_short_drama', 'creative_video'])
type Domain = 'project' | 'world'
export interface AppTarget { appId: Domain | 'image-mcp' | 'video-mcp'; path: string }
export function nativeRunTarget(service: string, run: string): AppTarget | null {
  if (!['image', 'video'].includes(service) || !/^[a-zA-Z0-9_-]{1,256}$/.test(run)) return null
  return { appId: service === 'image' ? 'image-mcp' : 'video-mcp', path: '/dashboard/tasks/' + run }
}
const invalid = (): never => { throw new Error('应用地址无效。') }
function parsed(path: string): URL {
  if (typeof path !== 'string' || path.length > 4096 || !path.startsWith('/') || path.startsWith('//') || /[\\#\r\n]/.test(path)) invalid()
  const url = new URL(path, 'https://app.invalid')
  if (url.origin !== 'https://app.invalid' || url.pathname !== path.split('?')[0]) invalid()
  url.searchParams.forEach((_value, key) => { if (url.searchParams.getAll(key).length !== 1) invalid() })
  return url
}
export function projectTarget(project: string, revision: string): AppTarget {
  if (!uuid.test(project) || !uuid.test(revision)) invalid()
  return { appId: 'project', path: '/projects?' + new URLSearchParams({ project, revision }) }
}
export function worldTarget(asset: string, version: string): AppTarget {
  if (!uuid.test(asset) || !uuid.test(version)) invalid()
  return { appId: 'world', path: '/?' + new URLSearchParams({ asset, version }) }
}
export function useAssetTarget(asset: string, version: string): AppTarget {
  if (!uuid.test(asset) || !uuid.test(version)) invalid()
  return { appId: 'project', path: '/use-asset?' + new URLSearchParams({ asset, version }) }
}
/** Compatibility consumes an application-relative path; the host selects installation. */
export function canonicalRoute(domain: Domain, path: string): string {
  const url = parsed(path)
  const mode = url.searchParams.get('mode')
  const files = url.searchParams.get('view') === 'files'
  if (domain === 'project') {
    if (projectCategories.has(url.pathname.slice(1))) {
      const cat = url.pathname.slice(1)
      url.pathname = '/projects'
      url.searchParams.set('category', cat)
    }
    if (!['/', '/projects', '/create', '/workspace', '/use-asset', '/publication'].includes(url.pathname) || (mode && mode !== 'projects')) invalid()
    if (files) url.pathname = uuid.test(url.searchParams.get('project') || '') ? '/workspace' : '/projects'
    else if (url.pathname === '/') url.pathname = '/projects'
  } else {
    if (!['/', '/world', '/projects'].includes(url.pathname) || (mode && mode !== 'world') || (url.pathname === '/projects' && mode !== 'world')) invalid()
    url.pathname = '/'
  }
  url.searchParams.delete('mode')
  if (files) url.searchParams.delete('view')
  return url.pathname + url.search
}
/** Preinstalled compatibility; the legacy host mode still keeps its original page. */
export function legacyStudioTarget(path: string): AppTarget | null {
  const url = parsed(path)
  if (url.pathname !== '/projects') return null
  const appId = url.searchParams.get('mode') === 'world' ? 'world' : 'project'
  return { appId, path: canonicalRoute(appId, path) }
}
