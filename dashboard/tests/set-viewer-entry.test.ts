import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildProjectViewerOpenTarget,
  classifyViewerOpenError,
  detectProjectSetPackages,
  findSetPackageForFile,
} from '../src/platform/set-viewer-entry.ts'

const SAMPLE_FILES = [
  {
    file_id: '01a1691d-0000-7000-8000-000000000001',
    path: 'sets/saw-bathroom-v9/set.json',
    role: 'set_package',
    content_ref: {
      store_id: '01a1691d-0000-7000-8000-000000000010',
      artifact_id: '01a1691d-0000-7000-8000-000000000011',
      version_id: '01a1691d-0000-7000-8000-000000000012',
    },
  },
  {
    file_id: '01a1691d-0000-7000-8000-000000000002',
    path: 'sets/saw-bathroom-v9/scene.glb',
    role: 'model',
    content_ref: {
      store_id: '01a1691d-0000-7000-8000-000000000010',
      artifact_id: '01a1691d-0000-7000-8000-000000000013',
      version_id: '01a1691d-0000-7000-8000-000000000014',
    },
  },
]

test('detectProjectSetPackages and findSetPackageForFile identify Set Package v1.0 directories', () => {
  const pkgs = detectProjectSetPackages(SAMPLE_FILES)
  assert.equal(pkgs.length, 1)
  assert.equal(pkgs[0].setId, 'saw-bathroom-v9')
  assert.equal(pkgs[0].sceneGlbFileId, '01a1691d-0000-7000-8000-000000000002')

  const matched = findSetPackageForFile(SAMPLE_FILES, '01a1691d-0000-7000-8000-000000000002')
  assert.ok(matched)
  assert.equal(matched.setId, 'saw-bathroom-v9')
})

test('buildProjectViewerOpenTarget pins fixed revision_id and classifies uninstalled/forbidden errors', () => {
  const target = buildProjectViewerOpenTarget({
    projectId: '01a16250-d1aa-7667-add1-588f58042bbc',
    revisionId: '01a1691d-bd00-7195-bb28-982913b20c14',
    setId: 'saw-bathroom-v9',
    cameraId: 'C1',
  })
  assert.equal(target.appId, 'viewer')
  assert.equal(
    target.path,
    '/?project=01a16250-d1aa-7667-add1-588f58042bbc&revision=01a1691d-bd00-7195-bb28-982913b20c14&set=saw-bathroom-v9&cam=C1'
  )

  assert.throws(() =>
    buildProjectViewerOpenTarget({
      projectId: '01a16250-d1aa-7667-add1-588f58042bbc',
      revisionId: 'head',
    })
  )

  assert.equal(classifyViewerOpenError({ code: 'APP_NOT_INSTALLED' }), 'not_installed')
  assert.equal(classifyViewerOpenError({ code: 'NO_ACTIVE_INSTALLATION' }), 'not_installed')
  assert.equal(classifyViewerOpenError({ code: 'FORBIDDEN' }), 'forbidden')
})
