import test from 'node:test'
import assert from 'node:assert/strict'
import { ProjectReader } from '../src/platform/project-reader.ts'
import { connectHost, disconnectHost } from '../src/platform/host.ts'

test('an evicted stable descriptor is rebuilt once and the original still passes its fixed hash', async () => {
 const saved = new Map(['window','location','fetch'].map(key => [key,Object.getOwnPropertyDescriptor(globalThis,key)]))
 const id = (n: number) => `00000000-0000-7000-8000-${String(n).padStart(12,'0')}`
 const content_ref = {store_id:id(3),artifact_id:id(4),version_id:id(5)}
 const bytes = new TextEncoder().encode('image bytes'), hash = [...new Uint8Array(await crypto.subtle.digest('SHA-256',bytes))].map(x=>x.toString(16).padStart(2,'0')).join('')
 let descriptions=0, requests=0
 const sdk = {
  version:'1.1.0', context:{get:()=>({appId:'project'}),subscribe:()=>()=>{}},theme:{subscribe:()=>()=>{}},navigation:{subscribe:()=>()=>{}},lifecycle:{subscribe:()=>()=>{}},
  capabilities:{call:async()=>{descriptions++;return {version:{...content_ref,mime:'image/png',size:bytes.length,sha256:hash},content_path:`/studio/media/${'a'.repeat(64)}/${'b'.repeat(64)}/original-v1`}}},
 }
 Object.defineProperty(globalThis,'window',{configurable:true,value:{$wujie:{props:{appSDK:sdk,dashboardHost:{ready(){},resize(){}}}}}})
 Object.defineProperty(globalThis,'location',{configurable:true,value:{origin:'https://studio.test'}})
 globalThis.fetch=async (_path,options)=>{
   assert.equal(options?.cache,'default');requests++
   return requests===1?new Response('',{status:404}):new Response(bytes,{headers:{'Content-Type':'image/png'}})
 }
 const reader=new ProjectReader(id(1),id(2))
 reader.project={project_id:id(1),revision_id:id(2),manifest:{name:'Fixed',category:'ai_short_drama',domain_documents:[],files:[{file_id:id(6),path:'image.png',content_ref}]}}
 try {
   connectHost('project')
   const first=await reader.media(id(6),'image')
   assert.equal(await reader.media(id(6),'image'),first)
   assert.equal(descriptions,2);assert.equal(requests,2)
 } finally {
   reader.dispose();disconnectHost()
   for(const [key,value] of saved) {if(value)Object.defineProperty(globalThis,key,value);else Reflect.deleteProperty(globalThis,key)}
 }
})
