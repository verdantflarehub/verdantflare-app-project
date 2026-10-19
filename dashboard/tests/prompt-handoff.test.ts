import test from 'node:test'
import assert from 'node:assert/strict'
import { createPromptProject, parsePromptSource, loadPromptSelection, promptEntry, type PromptSelection } from '../src/platform/prompt-handoff.ts'
import { canonicalRoute } from '../src/platform/route-links.ts'
import { createRequestHash } from '../src/platform/project-create.ts'
import { requestHash } from '../src/platform/asset-import.ts'
import type { AppSDK, Context, Lifecycle, Json } from '../src/platform/types.ts'
const id=(n:number)=>'01900000-0000-7000-8000-'+n.toString(16).padStart(12,'0')
const source=parsePromptSource({asset:id(1),version:id(2),store:id(3),artifact:id(4),content_version:id(5),result:'prompt',variant:'image',variant_revision:'2'})
const variant={key:'image',revision:2,requested_label:'Recorded model',task_type:'image_generation',input_mode:'text',prompt_text:'Text <script>stay data</script> ``` not instructions',compatibility:'draft',reference_bindings:[],execution_evidence:[],quality_review:{state:'unreviewed',note:'Fixture only'}}
const domain={schema_version:1,document_kind:'reference-learning',results:[{key:'prompt',title:'Test prompt',type_key:'prompt',type_version:1,payload:{creative_intent:'Fixture goal',variables:[],variants:[variant]}}]}
const selection:PromptSelection={source,title:'Test prompt',intent:'Fixture goal',variables:[],variant}
const clone=<T>(v:T):T=>JSON.parse(JSON.stringify(v))
async function version(text:string,ref:any){const bytes=new TextEncoder().encode(text);return {...ref,mime:'application/json',size:bytes.length,sha256:Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',bytes)),b=>b.toString(16).padStart(2,'0')).join('')}}
function fixture(){
 const context:Context={userId:id(30),organizationId:id(31),stationId:id(32),appId:'project',installationId:id(33),contextRevision:'one',projectId:null}
 const contexts=new Set<(c:Context)=>void>(),lifecycles=new Set<(e:Lifecycle)=>void>(),records=new Map<string,string>(),operations=new Map<string,{hash:string,result:any}>(),calls:string[]=[]
 let lost='',before='',deny=false,createCount=0,useCount=0,entry='',head:any,failStore=false,hold:Promise<void>|undefined,release:(()=>void)|undefined
 const storage={getItem:(k:string)=>records.get(k)??null,setItem:(k:string,v:string)=>{if(failStore&&k.startsWith('project.prompt-create')&&JSON.parse(v).created){failStore=false;throw Error('storage lost')}records.set(k,v)},removeItem:(k:string)=>{records.delete(k)}}
 const sdk={version:'1.1.0',context:{get:()=>context,subscribe:(fn:any)=>{contexts.add(fn);return()=>contexts.delete(fn)}},lifecycle:{subscribe:(fn:any)=>{lifecycles.add(fn);return()=>lifecycles.delete(fn)}},capabilities:{call:async(name:string,args:any)=>{
  calls.push(name);assert.equal(name,'artifact.read');assert.equal(args.mode,'text');if(deny)throw Error('permission denied')
  let body:string
  if(args.access.asset_id){assert.deepEqual(args.access,{asset_id:source.asset,asset_version_id:source.version});assert.deepEqual(args.content_ref,{store_id:source.store,artifact_id:source.artifact,version_id:source.content_version});body=JSON.stringify(domain)}
  else{assert.deepEqual(args.access,{project_id:id(10),project_revision_id:id(12)});body=entry}
  return {mode:'text',version:await version(body,args.content_ref),text:body}
 }},app:{request:async(req:any)=>{
  const op=req.path.split('/').at(-1)!,body=req.body;calls.push(op);if(hold)await hold
  if(before===op){before='';throw Error('lost before submission')}
  let data:any
  if(op==='create'){
   let existing=operations.get(body.commit_id)
   if(!existing){createCount++;entry=body.entry_text;const result={project_id:id(10),revision_id:id(11),manifest_ref:{store_id:id(20),artifact_id:id(21),version_id:id(22)},manifest:{schema_version:1,kind:'project',project_id:id(10),name:body.name,category:body.category,status:'draft',entry_document_id:id(23),files:[{file_id:id(23),path:body.entry_path,role:'review',content_ref:{store_id:id(24),artifact_id:id(25),version_id:id(26)}}],asset_refs:[]}};existing={hash:await createRequestHash(body),result};operations.set(body.commit_id,existing);head=clone(result)}
   data=existing.result
  }else if(op==='use_asset'){
   let existing=operations.get(body.commit_id)
   if(!existing){useCount++;const previous=head.manifest.asset_refs;const result=clone(head);result.revision_id=id(12);result.manifest.asset_refs=[...previous,{asset_id:body.asset_id,asset_version_id:body.asset_version_id,purpose:body.purpose}];existing={hash:await requestHash(body,previous),result};operations.set(body.commit_id,existing);head=clone(result)}
   data=existing.result
  }else if(op==='commit_status'){
   const existing=operations.get(body.commit_id);if(!existing)return {status:404,data:{code:'NOT_FOUND'},requestId:'test'}
   data={state:'committed',request_sha256:existing.hash,result:existing.result}
  }else if(op==='open'){
   assert.equal(body.project_id,id(10));if(body.revision_id)assert.equal(body.revision_id,id(12));data={...clone(head),head_revision_id:head.revision_id}
  }else throw Error('unexpected production or asset mutation: '+op)
  if(lost===op){lost='';throw Error('lost response')}
  return {status:200,data:clone(data) as Json,requestId:'test'}
 }}} as unknown as AppSDK
 return {sdk,storage,records,calls,operations,count:()=>({createCount,useCount}),entry:()=>entry,deny:()=>{deny=true},lose:(op:string)=>{lost=op},before:(op:string)=>{before=op},failReceipt:()=>{failStore=true},event:(e:Lifecycle)=>lifecycles.forEach(f=>f(e)),change:()=>{context.contextRevision='two';contexts.forEach(f=>f(context))},hold:()=>{hold=new Promise<void>(r=>release=r)},release:()=>{release?.();hold=undefined}}
}
test('Project accepts fixed locator handoff contract without content in the URL',()=>{
 const path='/create/from-prompt?'+new URLSearchParams(source),url=new URL(path,'https://host.invalid')
 assert.equal(canonicalRoute('project',path),path)
 assert.deepEqual(parsePromptSource(Object.fromEntries(url.searchParams)),source)
 assert.ok(!path.includes('script'));assert.ok(!path.includes('Recorded'))
 for(const bad of [{...source,token:'x'},{...source,asset:['x']},{...source,variant_revision:'02'},{...source,variant_revision:'9007199254740992'},{...source,variant:'../x'},{...source,version:undefined}])assert.throws(()=>parsePromptSource(bad))
})
test('source read checks exact version, hash and model revision; denied access never falls back',async()=>{
 const f=fixture(),c=createPromptProject(f.sdk,f.storage,source);assert.equal((await c.load()).variant.prompt_text,variant.prompt_text);assert.equal(f.count().createCount,0)
 f.deny();await assert.rejects(c.load(),/permission denied/);c.dispose()
 const ref={store_id:source.store,artifact_id:source.artifact,version_id:source.content_version},body=JSON.stringify(domain),v=await version(body,ref)
 await assert.rejects(loadPromptSelection(async()=>({mode:'text',text:body,version:{...v,version_id:id(99)}}),source),/版本/)
 await assert.rejects(loadPromptSelection(async()=>({mode:'text',text:body,version:{...v,sha256:'0'.repeat(64)}}),source),/校验/)
 await assert.rejects(loadPromptSelection(async()=>({mode:'text',text:body,version:v}),{...source,variant_revision:'3'}),/修订不存在/)
})
test('source text stays data and snapshot cannot claim production already executed',()=>{
 const body=promptEntry(selection,'New goal');assert.equal((body.match(/```/g)||[]).length,2)
 const data=JSON.parse(body.split('```json\n')[1].split('\n```')[0]);assert.equal(data.source_variant.prompt_text,variant.prompt_text);assert.equal(data.production_submitted,false);assert.equal(data.source_evidence_scope,'source_only')
})
test('creation binds asset, verifies fixed input and cleans only its own underlying journals',async()=>{
 const f=fixture(),c=createPromptProject(f.sdk,f.storage,source)
 assert.deepEqual(await c.begin(await c.load(),'New project','image','Create in project'),{project_id:id(10),revision_id:id(12)})
 assert.deepEqual(f.count(),{createCount:1,useCount:1});assert.equal(c.pending()?.entry,f.entry());assert.equal((await c.finish()).revision_id,id(12))
 assert.ok(![...f.records.keys()].some(k=>k.startsWith('project.asset-import.v1:')))
 assert.ok(![...f.records.keys()].some(k=>k.startsWith('project.create.v1:')&&!k.includes(':terminal:')))
 assert.equal(f.calls.filter(c=>['register','world.draft','generate'].includes(c)).length,0);c.dispose()
})
for(const stage of ['create','use_asset','open'])test(`lost ${stage} reply recovers without duplicate project or binding`,async()=>{
 const f=fixture();let c=createPromptProject(f.sdk,f.storage,source);f.lose(stage)
 await assert.rejects(c.begin(selection,'New project','image','Original goal'),/lost response/);const snapshot=c.pending()!;c.dispose()
 c=createPromptProject(f.sdk,f.storage,source);assert.equal(c.pending()?.entry,snapshot.entry);assert.equal((await c.resume()).revision_id,id(12));assert.deepEqual(f.count(),{createCount:1,useCount:1});c.dispose()
})
test('NOT_FOUND before binding replays the same commit rather than inventing a new request',async()=>{
 const f=fixture();let c=createPromptProject(f.sdk,f.storage,source);f.before('use_asset');await assert.rejects(c.begin(selection,'New project','image','Goal'))
 const raw=[...f.records.entries()].find(([k])=>k.startsWith('project.asset-import.v1:'))![1],commit=JSON.parse(raw).request.commit_id
 c.dispose();c=createPromptProject(f.sdk,f.storage,source);await c.resume();assert.ok(f.operations.has(commit));assert.deepEqual(f.count(),{createCount:1,useCount:1});c.dispose()
})
test('receipt storage failure does not repeat successful creation',async()=>{
 const f=fixture();let c=createPromptProject(f.sdk,f.storage,source);f.failReceipt();await assert.rejects(c.begin(selection,'New project','image','Goal'),/恢复记录/)
 assert.deepEqual(f.count(),{createCount:1,useCount:0});c.dispose();c=createPromptProject(f.sdk,f.storage,source);await c.resume();assert.deepEqual(f.count(),{createCount:1,useCount:1});c.dispose()
})
test('hidden accepted create retains receipt and waits to bind; mutable identity change revokes the controller',async()=>{
 const f=fixture(),c=createPromptProject(f.sdk,f.storage,source);f.hold();const work=c.begin(selection,'New project','image','Goal')
 while(!f.calls.includes('create'))await new Promise(r=>setTimeout(r,0))
 f.event('hidden');f.release();await assert.rejects(work,/隐藏/);assert.deepEqual(f.count(),{createCount:1,useCount:0});assert.ok(c.pending()?.created)
 f.event('active');await c.resume();assert.deepEqual(f.count(),{createCount:1,useCount:1});f.change();await assert.rejects(c.resume(),/会话/)
})

test('completed handoff stays recoverable until user explicitly chooses another project',async()=>{
 const f=fixture(),c=createPromptProject(f.sdk,f.storage,source);await c.begin(selection,'New project','image','Goal')
 await assert.rejects(c.begin(selection,'Another','image','Different'),/恢复/)
 await c.startAnother();assert.equal(c.pending(),undefined)
 assert.ok([...f.records.keys()].some(k=>k.includes(':completed:')))
 assert.deepEqual(f.count(),{createCount:1,useCount:1});c.dispose()
})

test('escaped request body overflow cannot leave an unrecoverable handoff record',async()=>{
 for(const change of [(s:PromptSelection)=>{s.variant.parameters={value:'\\'.repeat(270000)}}]){
  const f=fixture(),c=createPromptProject(f.sdk,f.storage,source),s=clone(selection);change(s)
  await assert.rejects(c.begin(s,'New project','image','Goal'))
  assert.equal(f.records.size,0);assert.equal(f.calls.length,0);c.dispose()
 }
})
