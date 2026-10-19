<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { useHost } from '../platform/host'
import { createPromptProject, parsePromptSource, type PromptSelection } from '../platform/prompt-handoff'
import { createCategories } from '../platform/project-create'
import { projectTarget } from '../platform/route-links'
const route=useRoute(),router=useRouter(),{appSDK,dashboardHost}=useHost()
const client=shallowRef<ReturnType<typeof createPromptProject>>(),selection=ref<PromptSelection>(),name=ref(''),category=ref(''),goal=ref(''),busy=ref(false),loading=ref(false),hidden=ref(false),error=ref('')
const pending=ref<ReturnType<ReturnType<typeof createPromptProject>['pending']>>()
let alive=true,generation=0,navigating=false
const identity=JSON.stringify(appSDK.context.get())
const model=computed(()=>selection.value?.variant.requested_label||'')
const preview=computed(()=>{const v=selection.value?.variant;if(!v)return '';return [v.prompt_text,...(Array.isArray(v.messages)?v.messages.map(raw=>{const m=raw as Record<string,unknown>;return `[${m.role}]\n${m.content}`}):[])].filter(Boolean).join('\n\n')})
function sync(){pending.value=client.value?.pending()}
async function load(){
 const c=client.value;if(!c||hidden.value||busy.value||pending.value)return
 const epoch=++generation;loading.value=true;error.value=''
 try{const selected=await c.load();if(alive&&!hidden.value&&epoch===generation){selection.value=selected;if(!name.value)name.value=selected.title+' · 新创作';if(!goal.value)goal.value=selected.intent}}
 catch{if(alive&&epoch===generation)error.value='无法读取所选提示词。请检查权限、固定版本和文件完整性后重试。'}
 finally{if(alive&&epoch===generation)loading.value=false}
}
async function act(resume=false){
 const c=client.value;if(!c||busy.value||hidden.value)return
 busy.value=true;error.value='';const epoch=generation
 try{if(resume)await c.resume();else if(selection.value)await c.begin(selection.value,name.value,category.value,goal.value)}
 catch(reason){if(alive&&epoch===generation)error.value=reason instanceof Error?reason.message:'交接尚未确认，请恢复原操作。'}
 finally{if(alive&&client.value===c){sync();busy.value=false}}
}
async function open(){
 const c=client.value;if(!c||busy.value||hidden.value)return
 busy.value=true;error.value=''
 try{const result=await c.finish();if(alive&&!hidden.value){navigating=true;await router.push(projectTarget(result.project_id,result.revision_id).path)}}
 catch{if(alive)error.value='项目已保留，暂时无法完成固定回读或打开。请恢复原交接，勿重复创建。'}
 finally{navigating=false;if(alive)busy.value=false}
}
async function another(){
 const c=client.value;if(!c||busy.value||hidden.value)return
 busy.value=true;error.value=''
 try{await c.startAnother();if(alive&&client.value===c){sync();selection.value=undefined;name.value='';goal.value=''}}
 catch{if(alive)error.value='原交接尚未完成归档，未开始新项目。'}
 finally{if(alive&&client.value===c){busy.value=false;if(!pending.value)void load()}}
}
function canLeave(){if(navigating||!busy.value)return true;error.value='正在确认交接结果，请稍后再离开。';return false}
onBeforeRouteLeave(canLeave);onBeforeRouteUpdate(canLeave)
watch(()=>route.fullPath,()=>{
 generation++;client.value?.dispose();client.value=undefined;selection.value=undefined;pending.value=undefined;name.value='';goal.value='';category.value='';error.value='';loading.value=false
 try{client.value=createPromptProject(appSDK,window.localStorage,parsePromptSource(route.query));sync();void load()}
 catch{error.value='交接地址或恢复记录无效，请从数字世界重新选择；原记录未清除。'}
},{immediate:true})
watch([busy,name,goal],()=>dashboardHost.setDirty(busy.value||(!pending.value&&!!goal.value)),{flush:'sync'})
const stop=appSDK.lifecycle.subscribe(event=>{
 if(event==='hidden'){hidden.value=true;generation++;loading.value=false;selection.value=undefined}
 if(event==='active'){hidden.value=false;sync();if(!pending.value)void load()}
 if(event==='dispose'){alive=false;generation++;client.value?.dispose()}
})
const stopContext=appSDK.context.subscribe(c=>{
 if(JSON.stringify(c)===identity)return
 generation++;client.value?.dispose();client.value=undefined;selection.value=undefined;pending.value=undefined;name.value='';goal.value='';busy.value=false;loading.value=false;error.value='会话已变化，请重新打开项目。原恢复记录仍保留。'
})
onBeforeUnmount(()=>{alive=false;generation++;client.value?.dispose();stop();stopContext();dashboardHost.setDirty(false)})
</script>
<template><main class="main prompt-project">
<header><div class="eyebrow">WORLD → PROJECT</div><h1>基于提示词新建项目</h1><p>保留来源固定版本，在项目中修改与创作。样例只读，项目成果不会自动入库。</p></header>
<p v-if="error" class="state error" role="alert">{{error}}</p><p v-if="loading||busy" class="state" role="status">{{loading?'正在读取所选模型配置…':'正在确认项目与资产引用…'}}</p>
<section v-if="pending" class="panel"><h2>{{pending.result?'项目输入已保存并回读':pending.created?'项目已创建，继续确认资产引用':'原交接尚未确认'}}</h2><p>{{pending.name}}</p><p>所选来源、模型配置与创作目标按原提交恢复，不读取最新资产替换。</p><div class="actions"><button v-if="!pending.result" class="btn primary" :disabled="busy||hidden" @click="act(true)">恢复这次交接</button><button v-else class="btn primary" :disabled="busy||hidden" @click="open">进入项目</button><button v-if="pending.result" class="btn" :disabled="busy||hidden" @click="another">用同一提示词再建项目</button></div><p class="hint">生产任务尚未提交。智能体将在项目内根据创作目标选择技能，经 MCP 执行并保存项目成果。</p></section>
<form v-else-if="selection" class="panel" @submit.prevent="act()"><h2>{{selection.title}}</h2><p class="hint">{{model}} · 配置 r{{selection.source.variant_revision}}</p><details><summary>查看固定提示词与配置</summary><pre>{{preview}}</pre><details><summary>参数、变量和参考绑定</summary><pre>{{JSON.stringify({variables:selection.variables,parameters:selection.variant.parameters,reference_bindings:selection.variant.reference_bindings},null,2)}}</pre></details></details>
<label>项目名称<input v-model="name" maxlength="256" required :disabled="busy||hidden"></label><label>项目类型<select v-model="category" required :disabled="busy||hidden"><option value="">请选择类型</option><option v-for="c in createCategories.filter(c=>c.key!=='music-mv')" :key="c.key" :value="c.key">{{c.label}}</option></select></label><label>这次要创作什么<textarea v-model="goal" rows="5" maxlength="8192" required :disabled="busy||hidden"/></label>
<button class="btn primary" type="submit" :disabled="busy||hidden||!name.trim()||!category||!goal.trim()">创建项目并保存创作输入</button></form>
<button v-else-if="client&&!loading&&!busy" class="btn" @click="load">重新读取提示词</button>
</main></template>
<style scoped>
header{margin-bottom:24px}h1{font-size:28px;margin:10px 0}header p,.hint{color:var(--muted);line-height:1.8}.eyebrow{color:var(--accent);font-size:11px;letter-spacing:2px}.panel,.state{padding:24px;border:1px solid var(--line);border-radius:12px;background:var(--shell-panel);margin-bottom:20px}.error{color:var(--danger)}h2{font-size:19px;margin-bottom:14px}label{display:grid;gap:8px;margin:18px 0;color:var(--muted)}input,select,textarea{width:100%;padding:10px;border:1px solid var(--line);border-radius:8px;color:var(--ink);background:var(--shell-button);font:inherit}textarea{resize:vertical}pre{white-space:pre-wrap;overflow-wrap:anywhere;font:inherit;line-height:1.8}details{margin:16px 0}summary{cursor:pointer;color:var(--muted)}.actions{margin-top:20px}
</style>
