import test from "node:test"
import assert from "node:assert/strict"
import {execFileSync} from "node:child_process"
import {mkdtempSync,readFileSync,writeFileSync} from "node:fs"
import {tmpdir} from "node:os"
import {join} from "node:path"
import ts from "typescript"
const root=new URL("..",import.meta.url).pathname
const temp=mkdtempSync(join(tmpdir(),"cv-source-parity-"))
const source=readFileSync(join(root,"src/lib/cvGeneration.ts"),"utf8")
writeFileSync(join(temp,"cv.mjs"),ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText.replace('import { workflowHeaders } from "./workflowRequest";', 'const workflowHeaders=()=>({});'))
const {stableCvFacts,outboundCvFacts}=await import(join(temp,"cv.mjs"))
const fixtures=JSON.parse(readFileSync(join(root,"internal/accountapi/testdata/profile-projections.json"),"utf8"))
const output=join(temp,"go.json")
execFileSync("rtk",["proxy","go","test","./internal/cvsource","-run","^TestCVSourceProjectionCorpus$","-count=1"],{cwd:root,env:{...process.env,GOCACHE:process.env.GOCACHE||"/tmp/careeros-go-cache",CAREEROS_CV_PROJECTION_OUTPUT:output},timeout:120000})
const results=JSON.parse(readFileSync(output,"utf8"))
assert.equal(results.length,fixtures.length)
for(const [i,fixture] of fixtures.entries())test(`CV source Go/TS full-value parity: ${fixture.name}`,()=>{
 const doc=structuredClone(fixture.document)
 const facts=outboundCvFacts(stableCvFacts(doc))
 assert.deepEqual({name:fixture.name,facts},results[i])
 assert.ok(facts.every(f=>!Object.hasOwn(f,"evidence")))
 assert.deepEqual(doc,fixture.document)
})
