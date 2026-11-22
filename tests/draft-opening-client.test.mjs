import test from 'node:test'
import assert from 'node:assert/strict'
import {mkdtempSync,readFileSync,writeFileSync} from 'node:fs'
import {tmpdir} from 'node:os'
import {join} from 'node:path'
import ts from 'typescript'

const dir=mkdtempSync(join(tmpdir(),'draft-opening-client-'))
for(const name of ['draftOpening','draftRecovery','workflowRequest']){
 const source=readFileSync(new URL(`../src/lib/${name}.ts`,import.meta.url),'utf8')
 writeFileSync(join(dir,`${name}.mjs`),ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText.replaceAll('"./workflowRequest"','"./workflowRequest.mjs"').replaceAll('"./draftRecovery"','"./draftRecovery.mjs"'))
}
const {DraftOpeningClient}=await import(join(dir,'draftOpening.mjs'))
const policy='complete-draft-opening-local-20261122-v1'
const parts=['role_summary','resume','cover_letter','employer_answers','support']
function ready(owner='owner'){
 const action={namespace:'opening',id:'action',owner,workflow:'application_draft'}
 const reference={kind:'profile',profileId:'profile',id:'skill',revision:1}
 return {action,creationKey:'original-key',state:'ready',artifactVersion:1,admittedAt:'2026-11-22T00:00:00Z',deadline:'2026-11-22T00:30:00Z',historical:false,cleanupQueued:false,bundleDigest:'d'.repeat(64),opening:{policy,state:'reserved'},review:{schema:'application-draft-human-review-v1',revision:0,artifacts:['summary','resume','cover','answers'].map(artifact=>({artifact,state:'unreviewed'})),accepted:false},job:{posting:'Java required.'},bundle:{schema:'application-draft-bundle-v1',binding:{action,profileSource:{id:'profile',revision:1},sourceDigest:'a'.repeat(64),inputDigest:'b'.repeat(64),language:'en',generationVersion:'controlled-complete-draft-v1',reviewVersion:'application-draft-review-snapshot-v1'},identity:{fullName:'Original Person',email:'',phone:'',location:'',professionalLinks:''},jobTitle:null,company:null,roleSummary:'Java role.',resume:'Java',coverLetter:'Dear hiring team,\n\nJava.\n\nSincerely,\nOriginal Person',employerAnswers:{state:'none_requested',text:'',requests:[]},support:{version:'application-draft-review-snapshot-v1',check:'complete',facts:[{reference,snapshot:{value:'Java'}}],resume:[{text:'Java',state:'supported',nonfactual:false,sources:[reference],jobSources:[],concerns:[]}],artifacts:[{field:'roleSummary',text:'Java role.',state:'supported',nonfactual:false,sources:[],jobSources:[{start:0,end:14,quote:'Java required.'}],concerns:[]}],concerns:[],completion:{version:'complete-draft-eligibility-v1',state:'complete',candidateDigest:'c'.repeat(64),inputDigest:'b'.repeat(64),answerRequests:[],concerns:[]}}}}
}
function receipt(result=ready()) {return {receipt:'a'.repeat(32)+'.'+'b'.repeat(64),bundleDigest:result.bundleDigest,manifest:{schema:policy,bundleDigest:result.bundleDigest,artifactVersion:1,requiredParts:[...parts]}}}
const confirmed=()=>({...ready(),delivered:true,opening:{policy,state:'delivered',confirmedAt:'2026-11-22T00:01:00Z'}})
globalThis.document={cookie:'__Host-careeros_csrf=synthetic'}

test('partial review availability sends neither receipt nor confirmation',async()=>{
 const calls=[]
 const client=new DraftOpeningClient('owner',async(...args)=>{calls.push(args);throw new Error('unexpected send')})
 for(const available of [[],parts.slice(0,1),parts.slice(0,4),[...parts,'unbound']]) await assert.rejects(client.open(ready(),available),/draft_opening_incomplete/)
 assert.deepEqual(calls,[])
})

test('deliberate complete opening obtains bound receipt then confirms exact manifest once',async()=>{
 const calls=[]
 const client=new DraftOpeningClient('owner',async(url,options)=>{
  calls.push({url,options})
  return Response.json(calls.length===1?receipt():confirmed())
 })
 const result=await client.open(ready(),parts)
 assert.equal(result.delivered,true)
 assert.deepEqual(calls.map(c=>[c.options.method,c.url]),[['POST','/api/drafts/original-key/versions/1/opening'],['POST','/api/drafts/original-key/versions/1/opening/confirm']])
 assert.deepEqual(JSON.parse(calls[0].options.body),{})
 assert.deepEqual(JSON.parse(calls[1].options.body),receipt())
 assert.equal(calls[1].options.headers['X-CareerOS-Account'],'owner')
 assert.equal(calls[1].options.headers['X-OpenAI-Api-Key'],undefined)
})

test('unbound owner, partial server manifest and changed version cannot cause ACK',async()=>{
 for(const mutation of ['owner','parts','version','digest']){
  const calls=[]
  const selected=ready()
  if(mutation==='owner') selected.action.owner='other'
  const client=new DraftOpeningClient('owner',async(url,options)=>{
   calls.push(url)
   const issued=receipt()
   if(mutation==='parts')issued.manifest.requiredParts.pop()
   if(mutation==='version')issued.manifest.artifactVersion=2
   if(mutation==='digest')issued.bundleDigest='f'.repeat(64)
   return Response.json(issued)
  })
  await assert.rejects(client.open(selected,parts),/draft_invalid_result|draft_opening_incomplete/)
  assert.equal(calls.length,mutation==='owner'?0:1)
 }
})

test('lost confirmation response stays uncertain with no automatic retry',async()=>{
 let calls=0
 const client=new DraftOpeningClient('owner',async()=>{if(++calls===1)return Response.json(receipt());throw new TypeError('lost reply')})
 await assert.rejects(client.open(ready(),parts),/draft_outcome_unknown/)
 assert.equal(calls,2)
})

test('authentication and access denial stay typed and canceled replies cannot restore content',async()=>{
 for(const status of [401,403,410]){
  let calls=0
  const client=new DraftOpeningClient('owner',async()=>{calls++;return Response.json({error:'denied'},{status})})
  await assert.rejects(client.open(ready(),parts),status===401?/draft_authentication_required/:/draft_access_denied/)
  assert.equal(calls,1)
 }
 let finish
 const client=new DraftOpeningClient('owner',()=>new Promise(resolve=>{finish=()=>resolve(Response.json(receipt()))}))
 const late=client.open(ready(),parts)
 client.cancel();finish()
 await assert.rejects(late,/draft_superseded/)
})

test('a successful HTTP response cannot substitute another delivered Action',async()=>{
 let calls=0
 const client=new DraftOpeningClient('owner',async()=>{
  if(++calls===1)return Response.json(receipt())
  const changed=confirmed();changed.action={...changed.action,id:'different-action'};changed.bundle.binding.action=changed.action
  return Response.json(changed)
 })
 await assert.rejects(client.open(ready(),parts),/draft_invalid_result/)
})

test('review availability is rechecked after receipt response before ACK',async()=>{
 let available=[...parts],calls=0
 const client=new DraftOpeningClient('owner',async()=>{calls++;available=parts.slice(0,4);return Response.json(receipt())})
 await assert.rejects(client.open(ready(),()=>available),/draft_opening_incomplete/)
 assert.equal(calls,1,'the issued receipt must not become ACK authority after required review support disappears')
})
