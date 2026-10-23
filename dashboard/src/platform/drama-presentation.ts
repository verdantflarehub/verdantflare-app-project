import type { DramaGlobal, DramaShot, DramaVersion, ProjectContent } from './project-content'

export interface ShotPreview {
  tier: 'ready' | 'previs' | 'missing'
  kind: 'video' | 'image' | null
  fileId?: string
  version?: DramaVersion
  selected: boolean
  badge: string
}

const visualPurposeKind: Record<string, 'video' | 'image'> = {
  shot_video: 'video',
  segment_video: 'video',
  video: 'video',
  keyframe: 'image',
  storyboard: 'image',
  preview: 'image',
  image: 'image',
  portrait: 'image',
}

function resolveVisualKind(fileId: string, purpose?: string | null, filePath?: (id: string) => string | undefined, fallback?: 'video' | 'image'): 'video' | 'image' | null {
  const path = filePath?.(fileId) || ''
  const ext = path.split('.').pop()?.toLowerCase() || ''
  if (['mp4', 'webm', 'mov'].includes(ext)) return 'video'
  if (['png', 'jpg', 'jpeg', 'webp', 'gif'].includes(ext)) return 'image'
  if (purpose && visualPurposeKind[purpose]) return visualPurposeKind[purpose]
  if (['md', 'json', 'blend', 'txt', 'fbx', 'obj', 'glb', 'gltf'].includes(ext)) return null
  return fallback || null
}

/** 4-tier visual resolution for the storyboard wall:
 * 1. Selected or latest visual version (video/image)
 * 2. Direct shot result_file_ids or image_file_id
 * 3. Camera previs / spatial reference in reference_file_ids (or whitebox version)
 * 4. Slate fallback (missing)
 */
export function shotPreview(shot: DramaShot, filePath?: (fileId: string) => string | undefined): ShotPreview {
  const versions = shot.versions || []
  const selections = shot.selected_versions || []
  const versionVisual = (v: DramaVersion) => {
    for (const ref of v.file_refs || []) {
      const kind = resolveVisualKind(ref.file_id, ref.purpose || v.purpose, filePath)
      if (kind) return { fileId: ref.file_id, kind }
    }
    return undefined
  }
  const selectedVersion = versions.find(v => selections.some(s => s.version_key === v.key) && versionVisual(v))
  const latestVersion = selectedVersion || [...versions].reverse().find(v => versionVisual(v))
  if (latestVersion) {
    const media = versionVisual(latestVersion)!
    const isSelected = !!selectedVersion
    return {
      tier: 'ready',
      kind: media.kind,
      fileId: media.fileId,
      version: latestVersion,
      selected: isSelected,
      badge: isSelected ? `已选用 · ${latestVersion.label}` : `${versions.length} 个版本 · ${latestVersion.label}`,
    }
  }
  const directResultId = shot.selected_result_file_id || shot.result_file_ids?.[0]
  if (directResultId) {
    const kind = resolveVisualKind(directResultId, 'video', filePath, 'video') || 'video'
    return {
      tier: 'ready',
      kind,
      fileId: directResultId,
      selected: !!shot.selected_result_file_id,
      badge: versions.length ? `${versions.length} 个版本` : '已有结果',
    }
  }
  if (shot.image_file_id) {
    const kind = resolveVisualKind(shot.image_file_id, 'image', filePath, 'image') || 'image'
    return {
      tier: 'ready',
      kind,
      fileId: shot.image_file_id,
      selected: false,
      badge: '镜头静帧',
    }
  }
  for (const refId of shot.reference_file_ids || []) {
    const kind = resolveVisualKind(refId, undefined, filePath, 'video')
    if (kind) {
      return {
        tier: 'previs',
        kind,
        fileId: refId,
        selected: false,
        badge: versions.length ? `${versions.length} 个版本 · 机位预演` : '机位预演',
      }
    }
  }
  if (versions.some(v => v.purpose === 'whitebox' || v.file_refs?.some(f => f.purpose === 'whitebox'))) {
    return {
      tier: 'previs',
      kind: null,
      selected: false,
      badge: '白模已登记',
    }
  }
  return {
    tier: 'missing',
    kind: null,
    selected: false,
    badge: versions.length ? `${versions.length} 个版本` : '待制作',
  }
}

/** Extracts a clean, human-readable shot action/visual summary from authored Markdown overview
 * without locking onto a single hardcoded field prefix like `**时空：**`. */
export function shotSummary(overview: string | null | undefined, drama?: ProjectContent['ai_short_drama']): string {
  const resolved = dramaText(overview, drama)
  if (!resolved.trim()) return '查看本镜分镜依据、机位参数与关联素材。'
  const lines = resolved
    .split(/\r?\n/)
    .map(line => line.replace(/^#{1,6}\s+/, '').replace(/^[-*+]\s+/, '').replace(/\*\*/g, '').replace(/^\s*\||\|\s*$/g, '').replace(/\s*\|\s*/g, ' · ').trim())
    .filter(line => line && !/^[-:| ·]+$/.test(line))
  if (!lines.length) return '查看本镜分镜依据、机位参数与关联素材。'
  const priority = lines.filter(line => /^(画面|动作|内容|描述|视觉|梗概|镜头|时空|机位|景别)[：:]/.test(line))
  const chosen = (priority.length ? priority : lines).map(line => line.replace(/^(画面|动作|内容|描述|视觉|梗概)[：:]\s*/, '').trim()).filter(Boolean)
  return chosen.join(' · ') || lines.join(' · ')
}

/** Viewing a candidate never changes selections or approval. Prefer a selected visual,
 * otherwise use the latest registered visual candidate in the supplied order. */
export function materialPreview(material: DramaGlobal) {
  const image = (v: DramaVersion) => v.file_refs.find(f => ['image', 'preview', 'portrait'].includes(f.purpose))
  const selected = material.versions.find(v => material.selected_versions.some(s => s.version_key === v.key) && image(v))
  const version = selected || [...material.versions].reverse().find(v => image(v))
  return { version, fileId: version ? image(version)!.file_id : (!material.versions.length && !['sound', 'music'].includes(material.kind) ? material.file_ids[0] : undefined), selected: !!selected }
}

/** Display aliases only; the stored text, keys and fixed references stay intact. */
export function dramaText(text: string | null | undefined, drama?: ProjectContent['ai_short_drama']): string {
  let result = text || ''
  if (drama) {
    const names = new Map(drama.globals.map(g => [g.key, g.name]))
    for (const g of drama.globals) {
      for (const v of g.versions || []) names.set(v.key, `${g.name} · ${v.label}`)
      if (g.kind === 'character') result = result.replace(new RegExp(`\\b${g.key}_ID_V\\d+\\b`, 'g'), `${g.name}身份参考`)
    }
    const sceneNames = new Map<string, Set<string>>()
    for (const ep of drama.episodes) for (const scene of ep.scenes) {
      names.set(scene.key, scene.title)
      const sharedKey = /^(S\d+)_SH\d+$/.exec(scene.key)?.[1]
      if (sharedKey) { const values = sceneNames.get(sharedKey) || new Set<string>(); values.add(scene.title); sceneNames.set(sharedKey, values) }
      for (const shot of scene.shots) names.set(shot.key, `「${shot.title}」`)
    }
    for (const [key, values] of sceneNames) if (values.size === 1) names.set(key, [...values][0])
    const escape = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const keys = [...names.keys()].sort((a, b) => b.length - a.length)
    if (keys.length) result = result.replace(new RegExp(`(?<![A-Za-z0-9_])(?:${keys.map(escape).join('|')})(?![A-Za-z0-9_])`, 'g'), key => names.get(key)!)
  }
  // Render links as their authored labels. Local source paths and Markdown markup
  // are not interactive URLs in the customer reader.
  return result.replace(/!?\[([^\]]+)\]\([^\n)]*\)/g, '$1').replace(/`([^`]+)`/g, '$1')
    .replace(/\b[A-Z][A-Z0-9_]*_V\d+\.(?:png|jpe?g|webp)\b/g, '参考图')
}

/** Resolve only the requested authored section; missing anchors must not silently
 * display a different state or the entire document as that state's basis. */
export function documentSection(text: string, anchor?: string): string {
  if (!anchor) return text
  const target = decodeURIComponent(anchor.replace(/^#/, ''))
  const lines = text.split('\n')
  const slug = (heading: string) => heading.toLocaleLowerCase().replace(/<[^>]+>/g, '').replace(/[^\p{L}\p{N}_ -]/gu, '').trim().replace(/ /g, '-')
  let start = lines.findIndex(line => {
    const explicit = /^\s*<a\s+(?:id|name)=["']([^"']+)["']\s*>\s*<\/a>\s*$/.exec(line)
    return explicit?.[1] === target || /^#{1,6}\s+/.test(line) && slug(line.replace(/^#{1,6}\s+/, '')) === target
  })
  if (start < 0) throw new Error('当前固定资料没有对应章节，请核对来源引用。')
  let heading = start
  while (heading >= 0 && !/^#{1,6}\s+/.test(lines[heading])) heading--
  if (heading >= 0) start = heading
  const level = /^#+/.exec(lines[start])?.[0].length || 6
  let end = start + 1
  while (end < lines.length && !new RegExp(`^#{1,${level}}\\s+`).test(lines[end])) end++
  return lines.slice(start, end).filter(line => !/^\s*<a\s+(?:id|name)=["'][^"']+["']\s*>\s*<\/a>\s*$/.test(line)).join('\n').trim()
}
