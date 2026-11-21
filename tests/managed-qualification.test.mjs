import test from "node:test"
import assert from "node:assert/strict"
import { mkdtempSync,readFileSync,writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"
const dir=mkdtempSync(join(tmpdir(),"managed-qualification-"))
for(const name of ["managedQualification","workflowRequest"]){
 const source=readFileSync(new URL(`../src/lib/${name}.ts`,import.meta.url),"utf8")
 writeFileSync(join(dir,`${name}.mjs`),ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText.replace('"./workflowRequest"','"./workflowRequest.mjs"'))
}
const {ManagedQualification,QualificationStatusDenied}=await import(join(dir,"managedQualification.mjs"))

test("managed qualification repeats stable identity and allocates one deliberate retry",()=>{
 const retained=new Map();const storage={getItem:key=>retained.get(key)??null,setItem:(key,value)=>retained.set(key,value),removeItem:key=>retained.delete(key)}
 const client=new ManagedQualification("account-a",storage)
 const first=client.command("synthetic career input")
 assert.deepEqual(client.command("synthetic career input"),first)
 assert.equal(first.retryKey,undefined)
 assert.ok(!JSON.stringify([...retained]).includes("synthetic career input"))
 client.knownFailure(true)
 const retry=client.command("synthetic career input")
 assert.equal(retry.actionKey,first.actionKey);assert.notEqual(retry.retryKey,undefined)
 assert.deepEqual(client.command("synthetic career input"),retry)
 const changed=client.command("changed input")
 assert.notEqual(changed.actionKey,first.actionKey);assert.equal(changed.retryKey,undefined)
})

test("recovered opaque key performs status GET only and never promises content",async()=>{
 globalThis.document={cookie:"__Host-careeros_csrf=synthetic-csrf"}
 const calls=[]
 const previous=globalThis.fetch
 globalThis.fetch=async(url,init)=>{calls.push({url,init});return new Response(JSON.stringify({actionId:"server-action",state:"unknown",attempt:1,retryAllowed:false,resultAvailable:false}))}
 try{
  const client=new ManagedQualification("account-a",{getItem:()=>"retained-opaque-key"})
  const status=await client.status()
  assert.equal(status.state,"unknown");assert.equal(status.resultAvailable,false)
  assert.equal(calls.length,1);assert.equal(calls[0].url,"/api/ai/qualification/actions/retained-opaque-key")
  assert.equal(calls[0].init.method,undefined)
  assert.equal(calls[0].init.headers["X-CareerOS-Account"],"account-a")
  assert.equal(calls[0].init.headers["X-OpenAI-Api-Key"],undefined)
  assert.equal(calls[0].init.headers["X-TypeSafe-Api-Key"],undefined)
 }finally{globalThis.fetch=previous;delete globalThis.document}
})

test("qualification status preserves typed authority denials without granting Retry",async()=>{
 const previous=globalThis.fetch
 globalThis.document={cookie:"__Host-careeros_csrf=synthetic-csrf"}
 try{
  for(const status of [401,403]){
   globalThis.fetch=async()=>new Response('denied',{status})
   const client=new ManagedQualification("account-a",{getItem:()=>"same-opaque-action"})
   client.knownFailure(true)
   await assert.rejects(client.status(),error=>error instanceof QualificationStatusDenied&&error.status===status&&error.message==="qualification_status_unavailable")
   assert.equal(client.retryAvailable(),false)
   assert.equal(client.command("same binding").actionKey,"same-opaque-action")
  }
 }finally{globalThis.fetch=previous;delete globalThis.document}
})
