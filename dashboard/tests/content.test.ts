import test from 'node:test'
import assert from 'node:assert/strict'
import { controlledContentPath, parseProjectContent, type ProjectOpen } from '../src/platform/project-reader.ts'

const id = (n: number) => `0192f3d4-0000-7000-8000-${String(n).padStart(12, '0')}`
const project: ProjectOpen = { project_id: id(1), revision_id: id(2), manifest: { name: 'Example', category: 'character', files: [{ file_id: id(3), path: 'portrait.png', content_ref: { store_id: id(4), artifact_id: id(5), version_id: id(6) } }], domain_documents: [] } }

test('business media must exist in this exact project revision', () => {
  const body = { schema_version: 1, kind: 'project-content', category: 'character', character: { name: 'Example', portrait_file_id: id(3) } }
  assert.equal(parseProjectContent(JSON.stringify(body), project).character?.portrait_file_id, id(3))
  assert.throws(() => parseProjectContent(JSON.stringify({ ...body, character: { portrait_file_id: id(9) } }), project), /当前项目修订/)
  assert.throws(() => parseProjectContent(JSON.stringify({ ...body, category: 'music', music: {} }), project), /格式/)
  assert.throws(() => parseProjectContent(JSON.stringify({ ...body, music: {} }), project), /格式/)
})

test('download paths accept only host session opaque handles', () => {
  Object.defineProperty(globalThis, 'location', { value: { origin: 'https://studio.example.invalid' }, configurable: true })
  const path = '/studio/dashboards/01bc51ec-c948-4118-9859-8a731a82e8cb/content/ae8439fb-76c4-41cf-99a0-b1eaaf914227'
  assert.equal(controlledContentPath(path), path)
  for (const value of ['https://other.example.invalid' + path, 'https://studio.example.invalid' + path, path + '?token=untrusted', path + '?project_id=' + id(1), path.replace('4118', '7118'), path + '#fragment', path.replace('/content/', '/%63ontent/'), '/v2/artifacts/' + id(6) + '/content', path.replace('/content/', '/x/../content/')]) {
    assert.throws(() => controlledContentPath(value))
  }
})
