import test from 'node:test'
import assert from 'node:assert/strict'
import { downloadPromptSource } from '../src/platform/prompt-source-download.ts'
const path='/studio/dashboards/00000000-0000-4000-8000-000000000001/content/00000000-0000-4000-8000-000000000002'
const hash=async(bytes:Uint8Array)=>[...new Uint8Array(await crypto.subtle.digest('SHA-256',bytes))].map(v=>v.toString(16).padStart(2,'0')).join('')
const signal=()=>new AbortController().signal

test('download rejects external handles and oversize metadata before any fetch',async()=>{
 const fetcher=async()=>{assert.fail('must not fetch');return new Response('')}
 for(const address of ['https://foreign.invalid/file',path+'?token=x',path+'/../other','/studio/media/not-a-handle'])await assert.rejects(downloadPromptSource(address,1,'a'.repeat(64),signal(),fetcher))
 for(const size of [-1,4*1024*1024+1,1.5])await assert.rejects(downloadPromptSource(path,size,'a'.repeat(64),signal(),fetcher))
})
test('download fails closed on permission denial, truncation, excess bytes, hash mismatch and invalid UTF-8',async()=>{
 const bytes=new TextEncoder().encode('source'),digest=await hash(bytes)
 for(const response of [new Response('',{status:403}),new Response('sourc'),new Response('sources'),new Response('other!')])await assert.rejects(downloadPromptSource(path,bytes.length,digest,signal(),async()=>response))
 const invalid=new Uint8Array([255]);await assert.rejects(downloadPromptSource(path,1,await hash(invalid),signal(),async()=>new Response(invalid)))
})
test('abort cancels a stalled response stream without returning partial source',async()=>{
 const controller=new AbortController();let cancelled=false,started!:()=>void
 const ready=new Promise<void>(r=>{started=r})
 const stream=new ReadableStream<Uint8Array>({pull(){started()},cancel(){cancelled=true}})
 const pending=downloadPromptSource(path,1,'a'.repeat(64),controller.signal,async()=>new Response(stream))
 await ready;controller.abort();await assert.rejects(pending,{name:'AbortError'});assert.equal(cancelled,true)
})
