import test from 'node:test'
import assert from 'node:assert/strict'
import { spaceReviewTarget } from '../src/platform/drama-set-review.ts'
import { parseReviewPath, reviewPath } from '../src/platform/set-review.ts'
import type { DramaGlobal } from '../src/platform/project-content.ts'

const id = (n: number) => `01900000-0000-7000-8000-${String(n).padStart(12, '0')}`
function material(): DramaGlobal {
  return { key: 'county', kind: 'location', file_ids: [], selected_versions: [], versions: [
    { key: 'historical', state_key: 'base', file_refs: [{ file_id: id(3), purpose: 'supporting' }] },
  ] } as unknown as DramaGlobal
}

test('space entry pins supporting-only evidence to its registered state and version', () => {
  const g = material(), target = spaceReviewTarget(id(1), id(2), g)!
  assert.deepEqual(parseReviewPath(reviewPath(target))?.locator, {
    kind: 'project', project_id: id(1), content_revision_id: id(2), material_key: 'county', state_key: 'base', version_key: 'historical',
  })
  assert.deepEqual(g.selected_versions, [])
})

test('ambiguous nonvisual versions require a choice and an invalid choice never falls back', () => {
  const g = material(); g.versions.push({ ...g.versions[0], key: 'engineering', state_key: 'night' })
  assert.equal(spaceReviewTarget(id(1), id(2), g), null)
  assert.equal(spaceReviewTarget(id(1), id(2), g, 'removed'), null)
  const target = spaceReviewTarget(id(1), id(2), g, 'engineering')!
  assert.equal(target.kind === 'project' && target.version_key, 'engineering')
  assert.equal(target.kind === 'project' && target.state_key, 'night')
})

test('unversioned legacy material retains its readonly legacy route', () => {
  const g = material(); g.versions = []
  const target = spaceReviewTarget(id(1), id(2), g)!
  assert.equal(target.kind === 'project' && target.version_key, '')
  assert.equal(target.kind === 'project' && target.state_key, '')
})
