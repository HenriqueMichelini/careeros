import test from "node:test"
import assert from "node:assert/strict"
import {mkdtempSync,readFileSync,writeFileSync} from "node:fs"
import {tmpdir} from "node:os"
import {join} from "node:path"
import ts from "typescript"
const dir=mkdtempSync(join(tmpdir(),"managed-profile-review-"))
for(const name of ["managedQualification","managedProfileReview","workflowRequest"]){
 const source=readFileSync(new URL(`../src/lib/${name}.ts`,import.meta.url),"utf8")
 writeFileSync(join(dir,`${name}.mjs`),ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText.replaceAll(/"\.\/(managedQualification|workflowRequest)"/g,'"./$1.mjs"'))
}
const {managedProfileReviewOwner,ManagedProfileReview}=await import(join(dir,"managedProfileReview.mjs"))
test("Profile review retains per-account section identity and only explicit known-failure Retry",async()=>{
 const values=new Map();const storage={getItem:k=>values.get(k)??null,setItem:(k,v)=>values.set(k,v),removeItem:k=>values.delete(k)}
 const c=managedProfileReviewOwner("a","skills",storage);const command=c.command("source")
 assert.deepEqual(managedProfileReviewOwner("a","skills",storage).command("source"),command)
 assert.notEqual(managedProfileReviewOwner("b","skills",storage).command("source").actionKey,command.actionKey)
 assert.notEqual(managedProfileReviewOwner("a","goals",storage).command("source").actionKey,command.actionKey)
 c.knownFailure(true);const retry=c.command("source");assert.equal(retry.actionKey,command.actionKey);assert.ok(retry.retryKey)
 c.completed();assert.notEqual(c.command("source").actionKey,command.actionKey)
 assert.ok(!JSON.stringify([...values]).includes("source"))
})

test("fresh Profile review owner rehydrates finished success only for the next deliberate new rewrite",async()=>{
 const previousFetch=globalThis.fetch
 globalThis.document={cookie:"__Host-careeros_csrf=synthetic-csrf"}
 try {
  for(const state of ["result_unavailable","unknown","known_failure"]){
   const values=new Map();const storage={getItem:k=>values.get(k)??null,setItem:(k,v)=>values.set(k,v),removeItem:k=>values.delete(k)}
   const first=new ManagedProfileReview("reload-account",storage,"skills").command("unchanged-root")
   const retainedBefore=JSON.stringify([...values]);const calls=[]
   globalThis.fetch=async(url,init)=>{calls.push({url,init});return new Response(JSON.stringify({actionId:"server-action",state,attempt:1,retryAllowed:state==="known_failure",resultAvailable:false}))}
   const restored=new ManagedProfileReview("reload-account",storage,"skills")
   const status=await restored.status()
   assert.equal(status.resultAvailable,false);assert.equal(calls.length,1)
   assert.equal(calls[0].init.method,undefined);assert.equal(calls[0].init.headers["X-OpenAI-Api-Key"],undefined)
   assert.equal(JSON.stringify([...values]),retainedBefore,"GET must not rotate or mutate the persisted key")
   const deliberate=restored.command("unchanged-root")
   if(state==="result_unavailable"){
    assert.notEqual(deliberate.actionKey,first.actionKey,"finished successful work needs a new Action after reload")
    assert.equal(deliberate.retryKey,undefined)
   }else{
    assert.equal(deliberate.actionKey,first.actionKey)
    assert.equal(!!deliberate.retryKey,state==="known_failure")
    assert.deepEqual(restored.command("unchanged-root"),deliberate,"known Retry identity remains stable; unknown never resends")
   }
   const changed=restored.command("changed-root");assert.notEqual(changed.actionKey,deliberate.actionKey)
   assert.notEqual(new ManagedProfileReview("other-account",storage,"skills").command("unchanged-root").actionKey,deliberate.actionKey)
   assert.notEqual(new ManagedProfileReview("reload-account",storage,"goals").command("unchanged-root").actionKey,deliberate.actionKey)
  }
 }finally{globalThis.fetch=previousFetch;delete globalThis.document}
})
