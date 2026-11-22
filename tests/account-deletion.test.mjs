import test from "node:test"
import assert from "node:assert/strict"
import ts from "typescript"
import { mkdtempSync,readFileSync,writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { workflowModuleURL } from "./workflow-module.mjs"
const temp=mkdtempSync(join(tmpdir(),"careeros-deletion-client-"))
const source=readFileSync(new URL("../src/lib/accountDeletion.ts",import.meta.url),"utf8")
writeFileSync(join(temp,"client.mjs"),ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText.replaceAll('"./workflowRequest"',JSON.stringify(workflowModuleURL)))
const {AccountDeletion}=await import(join(temp,"client.mjs"))
globalThis.document={cookie:"__Host-careeros_csrf=synthetic"}
const status={operationId:"delete-one",kind:"profile",state:"fenced_pending",fencedAt:"2026-11-22T10:00:00Z",recoveryCopyDeadline:"2026-12-22T10:00:00Z",recoveryCopies:"not_verified",recipients:"not_verified"}
const storage=()=>{const values=new Map();return{getItem:k=>values.get(k)??null,setItem:(k,v)=>values.set(k,v),removeItem:k=>values.delete(k)}}
test("explicit deletion has one stable identity; lost reply recovers by GET and reconstruction never POSTs",async()=>{
 const calls=[],tab=storage()
 const request=async(path,init)=>{calls.push([path,init]);if(init.method==="POST")throw Error("lost response");return new Response(JSON.stringify(status),{status:200})}
 const client=new AccountDeletion("owner-a",tab,request,()=>"delete-one")
 assert.deepEqual(await client.confirm({kind:"profile",profileId:"profile-a",revision:7}),status)
 assert.equal(calls[0][1].method,"POST");assert.equal(JSON.parse(calls[0][1].body).operationId,"delete-one")
 assert.equal(calls[1][1].method,"GET")
 const again=new AccountDeletion("owner-a",tab,request)
 assert.deepEqual(await again.recover(),status)
 assert.equal(calls.filter(([,r])=>r.method==="POST").length,1)
 const foreign=new AccountDeletion("owner-b",tab,request)
 assert.equal(foreign.pendingOperation,null)
})
test("malformed or denied status has no career payload and cannot cause an automatic mutation",async()=>{
 const calls=[];const client=new AccountDeletion("owner-a",storage(),async(path,init)=>{calls.push(init.method);return new Response(JSON.stringify({...status,bundle:{secret:"career"}}),{status:403})})
 await assert.rejects(client.recover(),e=>e.code==="authentication")
 assert.deepEqual(calls,["GET"])
})

test("a deliberate corrected confirmation after authenticated no-record recovery keeps its original key",async()=>{
 const calls=[];let admitted=false
 const client=new AccountDeletion("owner-a",storage(),async(path,init)=>{
  calls.push([path,init]);if(init.method==="GET")return new Response('{}',{status:404})
  if(!admitted){admitted=true;return new Response('{}',{status:401})}
  return new Response(JSON.stringify(status),{status:202})
 },()=>"delete-one")
 const input={kind:"profile",profileId:"profile-a",revision:7}
 await assert.rejects(client.confirm(input))
 await assert.rejects(client.recover())
 assert.equal(calls.length,2)
 await client.confirm(input)
 assert.deepEqual(calls.filter(([,i])=>i.method==="POST").map(([,i])=>JSON.parse(i.body).operationId),["delete-one","delete-one"])
})
