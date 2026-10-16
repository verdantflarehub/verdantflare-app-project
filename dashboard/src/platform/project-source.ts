import type { ProjectContent, ProjectOpen } from './project-content.ts'
import { musicVersionRoute, type MusicSource, type MusicGroup } from './music-production.ts'

export interface ProjectSource {
  key: string; name: string; purpose: string; category: 'character' | 'music'; sourceItem?: string
  project_id: string; revision_id: string; preview_file_id: string | null; item: string
  musicInput?: MusicSource; ownerGroup?: MusicGroup; ownerVersion?: string
}

/** Use explicit, fixed relations only; never derive a source from a name or Markdown. */
export function projectSources(content: ProjectContent | null): ProjectSource[] {
  const castSources: ProjectSource[] = (content?.video?.cast || []).flatMap(cast => cast.project_id && cast.revision_id ? [{
    key: 'project:cast:' + cast.key, name: cast.name, purpose: cast.role === 'lead' ? '本片主角' : '本片配角',
    category: 'character' as const, project_id: cast.project_id, revision_id: cast.revision_id,
    preview_file_id: cast.portrait_file_id, item: cast.key,
  }] : [])
  const musicSources: ProjectSource[] = (content?.ai_short_drama?.globals || []).flatMap(g => g.kind === 'music' && g.music_source ? [{
    key: 'project:music:' + g.key, name: g.name, purpose: '短剧配乐', category: 'music',
    project_id: g.music_source.project_id, revision_id: g.music_source.revision_id,
    sourceItem: g.music_source.song_key, preview_file_id: null, item: g.key,
  }] : [])
  const productionSources: ProjectSource[] = []
  const music = content?.music
  if (music) for (const [group, entries] of [['voice', music.voice.models], ['songs', music.songs], ['mvs', music.mvs]] as const) {
    for (const entry of entries) for (const version of entry.production?.versions || []) version.sources.forEach((source, index) => productionSources.push({
      key: `project:production:${group}:${entry.key}:${version.key}:${index}`, name: entry.title, purpose: `${version.label} · ${source.kind === 'song' ? '音乐来源' : source.kind === 'model' ? '人声模型来源' : '干声来源'}`,
      category: 'music', project_id: source.project_id, revision_id: source.revision_id, preview_file_id: null, item: entry.key,
      sourceItem: source.item_key, musicInput: source, ownerGroup: group, ownerVersion: version.key,
    }))
  }
  return [...castSources, ...musicSources, ...productionSources]
}

export function projectSourceRoute(owner: ProjectOpen, source: ProjectSource, query: Record<string, unknown>) {
  const target = { path: '/projects', query: {
    ...query, category: source.category, project: source.project_id, revision: source.revision_id, tab: source.category === 'music' ? 'songs' : 'setting',
    item: source.sourceItem, reference: undefined, candidate: undefined, file: undefined, fixed: undefined, run: undefined,
    material: undefined, scene: undefined, shot: undefined, episode: undefined, continuity: undefined,
    deliverable: undefined, state: undefined, version: undefined, compare: undefined, package: undefined, lighting: undefined,
    from: owner.project_id, fromRevision: owner.revision_id, fromName: owner.manifest.name,
    fromItem: undefined, fromReference: source.key, fromCategory: owner.manifest.category,
  } }
  if (source.musicInput) Object.assign(target.query, musicVersionRoute(query, source.musicInput).query)
  return target
}

export function sourceReturnCategory(value: string): 'video' | 'ai_short_drama' | 'music' {
  return value === 'ai_short_drama' || value === 'music' ? value : 'video'
}

export function fixedMusicSource(project: ProjectOpen, content: ProjectContent | null, songKey: string) {
  if (project.manifest.category !== 'music' || !content?.music?.songs.some(s => s.key === songKey)) throw Error('来源固定修订没有这首歌曲。')
  return { project_id: project.project_id, revision_id: project.revision_id, song_key: songKey }
}
