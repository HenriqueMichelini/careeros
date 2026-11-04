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
const {managedProfileReviewOwner}=await import(join(dir,"managedProfileReview.mjs"))
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
