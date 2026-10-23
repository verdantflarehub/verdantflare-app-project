import type { DramaGlobal } from './project-content'
import { materialPreview } from './drama-presentation.ts'
import type { ReviewLocator } from './set-review'

/** Keep the space entry on a version in the already loaded revision, including
 * engineering/supporting-only candidates that have no ordinary image preview. */
export function spaceReviewTarget(projectID: string, revisionID: string, material: DramaGlobal, requestedVersion?: string): ReviewLocator | null {
  const version = requestedVersion
    ? material.versions.find(v => v.key === requestedVersion)
    : materialPreview(material).version || (material.versions.length === 1 ? material.versions[0] : undefined)
  if (requestedVersion && !version || material.versions.length > 0 && !version) return null
  return { kind: 'project', project_id: projectID, content_revision_id: revisionID, material_key: material.key,
    state_key: version?.state_key || '', version_key: version?.key || '' }
}
