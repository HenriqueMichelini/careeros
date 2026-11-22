import test from "node:test"
import assert from "node:assert/strict"
import {readFileSync,mkdtempSync,writeFileSync} from "node:fs"
import {tmpdir} from "node:os"
import {join} from "node:path"
import ts from "typescript"
const dir=mkdtempSync(join(tmpdir(),"lifecycle-client-"))
const source=readFileSync(new URL("../src/lib/accountLifecycle.ts",import.meta.url),"utf8")
writeFileSync(join(dir,"client.mjs"),ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText)
const {readLifecycle,LifecycleError}=await import(join(dir,"client.mjs"))
const status={notices:[{id:"opaque",kind:"access_ended",dueAt:"2026-11-01T00:00:00Z",removalAt:"2026-12-01T00:00:00Z",emailState:"unknown",attempts:1,operatorSignal:true}],cleanup:[],residuals:[],purposeCleanup:0,removalPending:false,contentExpired:false,careerReadAllowed:true,invitationCleanup:[]}
test("owner lifecycle read is metadata-only GET and rejects career payload or an ownership-denial body",async()=>{
 let calls=[]
 const fake=async(url,init)=>{calls.push([url,init]);return new Response(JSON.stringify(status),{status:200})}
 assert.deepEqual(await readLifecycle("owner-one",undefined,fake),status)
 assert.equal(calls.length,1);assert.equal(calls[0][0],"/api/lifecycle/status");assert.equal(calls[0][1].method,"GET");assert.equal(calls[0][1].headers["X-CareerOS-Account"],"owner-one")
 for(const code of [401,403])await assert.rejects(readLifecycle("owner-one",undefined,async()=>new Response("PRIVATE ERROR",{status:code})),e=>e instanceof LifecycleError&&e.status===code&&!e.message.includes("PRIVATE"))
 await assert.rejects(readLifecycle("owner-one",undefined,async()=>new Response(JSON.stringify({...status,profile:"PRIVATE CAREER"}))),/Invalid lifecycle status/)
})
