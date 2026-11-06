import test from "node:test"
import assert from "node:assert/strict"
import {mkdtempSync,readFileSync,writeFileSync} from "node:fs"
import {tmpdir} from "node:os"
import {join} from "node:path"
import ts from "typescript"
const dir=mkdtempSync(join(tmpdir(),"managed-cv-"))
for(const name of ["managedQualification","managedStandaloneCV","workflowRequest","cvGeneration"]){
 const source=readFileSync(new URL(`../src/lib/${name}.ts`,import.meta.url),"utf8")
 writeFileSync(join(dir,`${name}.mjs`),ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText.replaceAll(/"\.\/(managedQualification|workflowRequest)"/g,'"./$1.mjs"'))
}
const {ManagedStandaloneCV}=await import(join(dir,"managedStandaloneCV.mjs"))
const {generateCv}=await import(join(dir,"cvGeneration.mjs"))
test("CV explicit new generation differs from replay; review/unknown cannot regenerate",async()=>{
 const values=new Map();const storage={getItem:k=>values.get(k)??null,setItem:(k,v)=>values.set(k,v),removeItem:k=>values.delete(k)}
 const c=new ManagedStandaloneCV("a",storage);const first=c.command("root")
 c.pending();assert.throws(()=>c.command("root"),/managed_status/)
 c.knownFailure(true);const retry=c.command("root");assert.equal(retry.actionKey,first.actionKey);assert.ok(retry.retryKey)
 c.completed(false);const another=c.command("root");assert.notEqual(another.actionKey,first.actionKey);assert.equal(another.retryKey,undefined)
 c.completed(true);assert.throws(()=>c.command("root"),/managed_review_required/)
 assert.notEqual(c.command("different-language").actionKey,another.actionKey)
 assert.ok(!JSON.stringify([...values]).includes("root"))
 assert.notEqual(new ManagedStandaloneCV("b",storage).command("root").actionKey,first.actionKey)
})

test("CV reload status stays GET-only and permits genuine new choices without bypassing server review authority",async()=>{
 const priorFetch=globalThis.fetch;globalThis.document={cookie:"__Host-careeros_csrf=synthetic"}
 try {
  for(const state of ["result_unavailable","unknown","known_failure"]){
   const values=new Map();const storage={getItem:k=>values.get(k)??null,setItem:(k,v)=>values.set(k,v),removeItem:k=>values.delete(k)}
   const first=new ManagedStandaloneCV("reload",storage).command("original")
   for(const reviewRequired of (state==="result_unavailable"?[false,true]:[false])){
    const calls=[];globalThis.fetch=async(url,init)=>{calls.push({url,init});return new Response(JSON.stringify({actionId:"server",state,attempt:1,retryAllowed:state==="known_failure",reviewRequired,resultAvailable:false}))}
    const retained=JSON.stringify([...values]);const restored=new ManagedStandaloneCV("reload",storage)
    await restored.status();assert.equal(calls.length,1);assert.equal(calls[0].init.method,undefined);assert.equal(JSON.stringify([...values]),retained)
    if(state==="unknown"){assert.throws(()=>restored.command("original"),/managed_status/);continue}
    const command=restored.command(reviewRequired?"genuinely-changed-language":"original")
    if(state==="result_unavailable") {assert.notEqual(command.actionKey,first.actionKey);assert.equal(command.retryKey,undefined)}
    else {assert.equal(command.actionKey,first.actionKey);assert.ok(command.retryKey)}
   }
  }
 } finally {globalThis.fetch=priorFetch;delete globalThis.document}
})

test("real managed CV client carries the explicit full Profile revision precondition",async()=>{
 const previous=globalThis.fetch;globalThis.document={cookie:"__Host-careeros_csrf=synthetic"}
 try{
  const calls=[];const facts=[{id:"skill",section:"skills",entryId:"",field:"skills",text:"Java",reference:{profileId:"p",id:"skill",revision:1},owner:{profileId:"p",id:"p",revision:7},context:[],kind:"statement",assertion:"affirmed",intent:"actual",certainty:"certain",support:"unsupported"}]
  globalThis.fetch=async(url,init)=>{calls.push({url,init});return new Response(JSON.stringify({schemaVersion:3,summary:[{sourceIds:["skill"],text:"Java"}],selected:["skill"],wording:{},support:{policyVersion:"cv-statement-support-v1",assessment:"complete",statements:[{id:"summary:0",text:"Java",sourceIds:["skill"],status:"supported",reason:"Synthetic"}]}}))}
  const owner=new ManagedStandaloneCV("a")
  await assert.rejects(generateCv(facts,"en","",new AbortController().signal,"balanced","a",owner),e=>e.code==="input")
  assert.equal(calls.length,0)
  await generateCv(facts,"en","",new AbortController().signal,"balanced","a",owner,{id:"p",revision:7})
  assert.equal(calls.length,1)
  assert.deepEqual(JSON.parse(calls[0].init.body).profileSource,{id:"p",revision:7})
  assert.equal(calls[0].init.headers["X-OpenAI-Api-Key"],undefined)
 }finally{globalThis.fetch=previous;delete globalThis.document}
})
