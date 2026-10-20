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

test('creative video reads a fixed music project source without treating it as a local file', () => {
  const work: ProjectOpen = { ...project, manifest: { ...project.manifest, category: 'creative_video' } }
  const source = { kind: 'project_file', project_id: id(7), revision_id: id(8), file_id: id(9), content_ref: { store_id: id(10), artifact_id: id(11), version_id: id(12) } }
  const body = { schema_version: 1, kind: 'project-content', category: 'creative_video', creative_video: {
    direction: 'music_video', brief: { key: 'main', file_id: id(3), goal: 'Release teaser' }, music_source: source,
    rights: [], concepts: [], selected_concept_key: null, sequences: [], shots: [], takes: [], edits: [], delivery_variants: [], reviews: [], packages: [],
  } }
  assert.deepEqual(parseProjectContent(JSON.stringify(body), work).creative_video?.music_source, source)
  const selected = { ...source, selection_purpose: 'master' }
  assert.deepEqual(parseProjectContent(JSON.stringify({ ...body, creative_video: { ...body.creative_video, music_source: selected } }), work).creative_video?.music_source, selected)
  assert.throws(() => parseProjectContent(JSON.stringify({ ...body, creative_video: { ...body.creative_video, brief: { ...body.creative_video.brief, file_id: id(99) } } }), work), /当前项目修订/)
  assert.throws(() => parseProjectContent(JSON.stringify({ ...body, music: {} }), work), /格式/)
})

test('download paths accept only host session opaque handles', () => {
  Object.defineProperty(globalThis, 'location', { value: { origin: 'https://studio.example.invalid' }, configurable: true })
  const path = '/studio/dashboards/01bc51ec-c948-4118-9859-8a731a82e8cb/content/ae8439fb-76c4-41cf-99a0-b1eaaf914227'
  assert.equal(controlledContentPath(path), path)
  for (const value of ['https://other.example.invalid' + path, 'https://studio.example.invalid' + path, path + '?token=untrusted', path + '?project_id=' + id(1), path.replace('4118', '7118'), path + '#fragment', path.replace('/content/', '/%63ontent/'), '/v2/artifacts/' + id(6) + '/content', path.replace('/content/', '/x/../content/')]) {
    assert.throws(() => controlledContentPath(value))
  }
})

test('video output versions are optional and reject files outside the fixed revision', () => {
  const work: ProjectOpen = { ...project, manifest: { ...project.manifest, category: 'video' } }
  const body = { schema_version: 1, kind: 'project-content', category: 'video', video: { output_file_id: id(3) } }
  assert.deepEqual(parseProjectContent(JSON.stringify(body), work).video?.outputs, [])
  const version = { key: 'cut2', title: 'Second cut', result_file_ids: [id(3)] }
  const parse = (outputs: unknown[]) => parseProjectContent(JSON.stringify({ ...body, video: { ...body.video, outputs } }), work)
  assert.equal(parse([version]).video?.outputs?.[0].result_file_ids[0], id(3))
  assert.throws(() => parse([{ ...version, result_file_ids: [id(99)] }]), /当前项目修订/)
  assert.throws(() => parse([version, version]), /格式/)
})


test('image works keep versions and fixed file references', () => {
 const work: ProjectOpen = { ...project, manifest: { ...project.manifest, category: 'image' } }
 const version = { key:'v1', label:'First', parent_version_key:null, change_note:'Initial', file_refs:[{file_id:id(3),content_ref:project.manifest.files[0].content_ref,purpose:'output'}],reference_file_ids:[id(3)],prompt_file_ids:[id(3)] }
 const item = {key:'poster',title:'静水',display_version_key:'v1',versions:[version]}
 const body = {schema_version:1,kind:'project-content',category:'image',image:{works:[item]}}
 const parse = (b:unknown) => parseProjectContent(JSON.stringify(b),work)
 assert.equal(parse(body).image?.works[0].versions[0].file_refs[0].file_id,id(3))
 for(const mutate of [
  (b:any)=>b.image.works.push(b.image.works[0]),
  (b:any)=>b.image.works[0].versions.push(b.image.works[0].versions[0]),
  (b:any)=>b.image.works[0].display_version_key='missing',
  (b:any)=>b.image.works[0].versions[0].parent_version_key='v1',
  (b:any)=>b.image.works[0].versions[0].parent_version_key='missing',
  (b:any)=>b.image.works[0].versions[0].file_refs[0].content_ref.version_id=id(99),
  (b:any)=>b.image.works[0].versions[0].prompt_file_ids=[id(99)],
  (b:any)=>b.image.works[0].versions[0].file_refs[0].purpose='record',
  (b:any)=>b.image.works[0].versions[0].reference_file_ids=[id(3),id(3)],
  (b:any)=>b.character={}
 ]) {const b=structuredClone(body);mutate(b);assert.throws(()=>parse(b))}
 assert.equal(parse({...body,image:{}}).image?.works.length,0)
})
