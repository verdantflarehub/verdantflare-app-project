import test from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'
import { effectScope, nextTick, reactive, ref } from 'vue'
const require=createRequire(import.meta.url),{parse,compileScript}=require('vue/compiler-sfc'),ts=require('typescript')

test('scroll sentinel appends a directory batch, stops outside view, and resets for another version',async()=>{
 const {descriptor}=parse(readFileSync(new URL('../src/components/drama/DramaVersions.vue',import.meta.url),'utf8'))
 const code=ts.transpileModule(compileScript(descriptor,{id:'pagination'}).content,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText
 const nearEnd=ref(false),module={exports:{} as any}
 const dependencies:Record<string,any>={vue:require('vue'),'../../platform/visible':{useVisible:()=>nearEnd},'vue-router':{useRoute:()=>({query:{}}),useRouter:()=>({push(){}})},'../../platform/project-reader':{ProjectReader:class{}},'./DramaProse.vue':{},'./DramaDocument.vue':{},'./DramaFile.vue':{}}
 new Function('require','module','exports',code)((name:string)=>{assert(name in dependencies,name);return dependencies[name]},module,module.exports)
 const previous=Object.getOwnPropertyDescriptor(globalThis,'window')
 Object.defineProperty(globalThis,'window',{configurable:true,value:{innerHeight:800}})
 const scope=effectScope(),props=reactive({reader:{},versions:[{key:'large',file_refs:Array.from({length:148},(_,i)=>({file_id:String(i)}))}]})
 try{
  const state=scope.run(()=>module.exports.default.setup(props,{expose(){}}))
  let reachedBatch=12
  state.sentinel.value={getBoundingClientRect:()=>({top:state.visibleCount.value<=reachedBatch?200:2000,bottom:2100})}
  const settle=async()=>{for(let i=0;i<5;i++)await nextTick()}
  assert.equal(state.visibleFiles.value.length,12)
  nearEnd.value=true;await settle();assert.equal(state.visibleFiles.value.length,24)
  await settle();assert.equal(state.visibleFiles.value.length,24,'old observer state must not append all files')
  while(state.visibleFiles.value.length<148){reachedBatch=state.visibleCount.value;nearEnd.value=false;await settle();nearEnd.value=true;await settle()}
  assert.equal(state.visibleFiles.value.length,148)
  nearEnd.value=false;props.versions=[{key:'other',file_refs:Array.from({length:30},(_,i)=>({file_id:String(i)}))}]
  await settle();assert.equal(state.visibleFiles.value.length,12)
 }finally{scope.stop();if(previous)Object.defineProperty(globalThis,'window',previous);else Reflect.deleteProperty(globalThis,'window')}
})
