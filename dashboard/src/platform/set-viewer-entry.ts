// Set Package v1.0 detection and cross-app navigation.openApp helpers for Project dashboard.

import type { ProjectFile } from './project-content.ts'

const UUID_V7_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const SET_ID_RE = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/

export interface DetectedSetPackage {
  setId: string
  prefix: string
  setJsonFileId: string
  sceneGlbFileId: string
}

export type ViewerLaunchOutcome = 'opened' | 'not_installed' | 'forbidden' | 'failed'

export function detectProjectSetPackages(files: readonly ProjectFile[] | undefined | null): DetectedSetPackage[] {
  if (!Array.isArray(files) || files.length === 0) return []
  const byPath = new Map<string, ProjectFile>()
  for (const f of files) {
    if (f && typeof f.path === 'string' && typeof f.file_id === 'string') {
      const clean = f.path.replace(/^\/+/, '').trim()
      if (clean) byPath.set(clean, f)
    }
  }

  const results: DetectedSetPackage[] = []
  const seen = new Set<string>()

  for (const [pathStr, file] of byPath.entries()) {
    const m = /^sets\/([^/]+)\/set\.json$/.exec(pathStr)
    if (m) {
      const setId = m[1]
      const glb = byPath.get(`sets/${setId}/scene.glb`)
      if (glb && SET_ID_RE.test(setId) && !seen.has(setId)) {
        seen.add(setId)
        results.push({
          setId,
          prefix: `sets/${setId}`,
          setJsonFileId: file.file_id,
          sceneGlbFileId: glb.file_id,
        })
      }
    }
  }

  const rootSet = byPath.get('set.json')
  const rootGlb = byPath.get('scene.glb')
  if (rootSet && rootGlb && !seen.has('default')) {
    results.push({
      setId: 'default',
      prefix: '',
      setJsonFileId: rootSet.file_id,
      sceneGlbFileId: rootGlb.file_id,
    })
  }

  return results
}

export function findSetPackageForFile(
  files: readonly ProjectFile[] | undefined | null,
  fileId: string | undefined | null
): DetectedSetPackage | null {
  if (!fileId || !Array.isArray(files)) return null
  const packages = detectProjectSetPackages(files)
  if (packages.length === 0) return null
  const target = files.find(f => f.file_id === fileId)
  if (!target) return null
  const clean = target.path.replace(/^\/+/, '').trim()
  for (const pkg of packages) {
    if (pkg.setJsonFileId === fileId || pkg.sceneGlbFileId === fileId) {
      return pkg
    }
    if (pkg.prefix && clean.startsWith(`${pkg.prefix}/`) && (clean.endsWith('.glb') || clean.endsWith('set.json'))) {
      return pkg
    }
  }
  return null
}

export function buildProjectViewerOpenTarget(params: {
  projectId: string
  revisionId: string
  setId?: string
  cameraId?: string
}): { appId: 'viewer'; path: string } {
  const projectId = (params.projectId || '').trim()
  const revisionId = (params.revisionId || '').trim()
  if (!UUID_V7_RE.test(projectId) || !UUID_V7_RE.test(revisionId)) {
    throw new Error('打开片场查看器需要有效的项目 ID 与固定修订 ID。')
  }
  const q = new URLSearchParams({
    project: projectId,
    revision: revisionId,
  })
  const setId = (params.setId || '').trim()
  if (setId && setId !== 'default' && SET_ID_RE.test(setId)) {
    q.set('set', setId)
  }
  const cam = (params.cameraId || 'C1').trim()
  if (cam && SET_ID_RE.test(cam)) {
    q.set('cam', cam)
  }
  return {
    appId: 'viewer',
    path: `/?${q.toString()}`,
  }
}

export function classifyViewerOpenError(cause: unknown): ViewerLaunchOutcome {
  const code =
    cause && typeof cause === 'object' && 'code' in cause && (cause as { code?: unknown }).code
      ? String((cause as { code?: unknown }).code || '')
      : cause && typeof cause === 'object' && 'message' in cause && (cause as { message?: unknown }).message
        ? String((cause as { message?: unknown }).message || '')
        : typeof cause === 'string'
          ? cause
          : ''
  if (
    [
      'APP_NOT_INSTALLED',
      'NO_ACTIVE_INSTALLATION',
      'APP_NOT_FOUND',
      'NOT_FOUND',
      'CAPABILITY_UNAVAILABLE',
      'TARGET_NOT_FOUND',
      'APP_NAVIGATION_UNAVAILABLE',
    ].includes(code)
  ) {
    return 'not_installed'
  }
  if (['PERMISSION_DENIED', 'FORBIDDEN', 'CAPABILITY_DENIED'].includes(code)) {
    return 'forbidden'
  }
  return 'failed'
}
