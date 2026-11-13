import test from 'node:test'
import assert from 'node:assert/strict'
import {mkdtempSync,readFileSync,writeFileSync} from 'node:fs'
import {tmpdir} from 'node:os'
import {join} from 'node:path'
import ts from 'typescript'
const dir=mkdtempSync(join(tmpdir(),'draft-recovery-client-'))
for(const name of ['draftRecovery','workflowRequest','cover-letter']){
 const source=readFileSync(new URL(`../src/lib/${name}.ts`,import.meta.url),'utf8')
 writeFileSync(join(dir,`${name}.mjs`),ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText.replaceAll('"./workflowRequest"','"./workflowRequest.mjs"'))
}
const {DraftRecoveryClient}=await import(join(dir,'draftRecovery.mjs'))
function storage(){const values=new Map();return{values,getItem:k=>values.get(k)??null,setItem:(k,v)=>values.set(k,v),removeItem:k=>values.delete(k)}}
const input={profileSource:{id:'p',revision:1},job:{posting:'Java confidential fixture.'},answers:[],confirmations:[],selectedFactIds:[],language:'en'}
function ready(key,owner='a'){
 const action={namespace:'controlled-draft',id:'shared-action',owner,workflow:'application_draft'}
 const reference={kind:'profile',profileId:'p',id:'skill',revision:1}
 return{action,creationKey:key,state:'ready',artifactVersion:1,admittedAt:'2026-11-07T00:00:00Z',deadline:'2026-11-07T00:30:00Z',historical:false,cleanupQueued:false,job:input.job,bundle:{schema:'application-draft-bundle-v1',binding:{action,profileSource:input.profileSource,sourceDigest:'a'.repeat(64),inputDigest:'b'.repeat(64),language:'en',generationVersion:'controlled-complete-draft-v1',reviewVersion:'application-draft-review-snapshot-v1'},identity:{fullName:'Original Person',email:'',phone:'',location:'',professionalLinks:''},jobTitle:null,company:null,roleSummary:'Java role.',resume:'Java',coverLetter:'Dear hiring team,\n\nJava.\n\nSincerely,\nOriginal Person',employerAnswers:{state:'none_requested',text:'',requests:[]},support:{version:'application-draft-review-snapshot-v1',check:'complete',facts:[{reference,snapshot:{value:'Java'}}],resume:[{text:'Java',state:'supported',nonfactual:false,sources:[reference],jobSources:[],concerns:[]}],artifacts:[{field:'roleSummary',text:'Java role.',state:'supported',nonfactual:false,sources:[],jobSources:[{start:0,end:input.job.posting.length,quote:input.job.posting}],concerns:[]}],concerns:[],completion:{version:'complete-draft-eligibility-v1',state:'complete',candidateDigest:'c'.repeat(64),inputDigest:'b'.repeat(64),answerRequests:[],concerns:[]}}}}
}

test('lost creation response and reload recover the exact Action/version using GET only',async()=>{
 const saved=storage(),calls=[];globalThis.document={cookie:'__Host-careeros_csrf=synthetic'}
 let key,expected
 const transport=async(url,init={})=>{calls.push({url,init});if(init.method==='POST'){key=init.headers['X-CareerOS-Action-Key'];expected=ready(key);throw new TypeError('lost response')};return new Response(JSON.stringify(expected))}
 const client=new DraftRecoveryClient('a',saved,transport)
 await assert.rejects(client.create(input),/draft_outcome_unknown/)
 await assert.rejects(client.create(input),/draft_recover_first/)
 const restored=new DraftRecoveryClient('a',saved,transport)
 const result=await restored.recover();assert.deepEqual(result,expected);assert.equal(result.action.id,'shared-action');assert.equal(result.artifactVersion,1)
 assert.deepEqual(calls.map(x=>x.init.method??'GET'),['POST','GET'])
 assert.equal(calls[0].init.headers['X-OpenAI-Api-Key'],undefined)
 assert.equal(calls[0].init.headers['X-CareerOS-Account'],'a')
 assert.ok(!JSON.stringify([...saved.values]).includes('confidential'))
 assert.ok(!JSON.stringify([...saved.values]).includes('Original Person'))
 const another=new DraftRecoveryClient('b',saved,transport);assert.equal(another.hasKey(),false)
})

test('a late ready response cannot overwrite a later closed result and foreign bundles are rejected',async()=>{
 const saved=storage();let key,resolveLate;let reads=0
 const transport=async(url,init={})=>{if(init.method==='POST'){key=init.headers['X-CareerOS-Action-Key'];return new Response(JSON.stringify(ready(key)),{status:201})};reads++;if(reads===1)return new Promise(resolve=>{resolveLate=()=>resolve(new Response(JSON.stringify(ready(key))))});return new Response(JSON.stringify({...ready(key),state:'closed',bundle:undefined,job:undefined,cleanupQueued:true}),{status:410})}
 const client=new DraftRecoveryClient('a',saved,transport);await client.create(input)
 const late=client.recover();const closed=await client.recover();assert.equal(closed.state,'closed');resolveLate();await assert.rejects(late,/draft_superseded/);assert.equal(client.state,'closed')
 const foreign=new DraftRecoveryClient('a',saved,async()=>new Response(JSON.stringify(ready(key,'b'))));await assert.rejects(foreign.recover(),/draft_invalid_result/)
})


test('completed recovery letter limits preserve canonical JS whitespace at 399/400 words',async()=>{
 const {completeCoverLetter}=await import(join(dir,'cover-letter.mjs'))
 const cases=JSON.parse(readFileSync(new URL('../backend/application-draft/recovery_letter_fixtures.json',import.meta.url),'utf8'))
 for(const row of cases){const body=Array(row.words).fill('word').join(row.separator)+'.';const completed=`Dear hiring team,\n\n${body}\n\nSincerely,\nOriginal Person`;assert.equal(completed.split(/\s+/).length,row.count,row.name);assert.equal(completeCoverLetter({greeting:'Dear hiring team,',body,closing:'Sincerely,'},'Original Person')!==null,row.valid,row.name)}
})

test('authenticated absence allows one deliberate corrected submit on the preserved key', async () => {
 const saved = storage()
 const requests = []
 let key
 const send = async (url, options = {}) => {
   requests.push([options.method ?? 'GET', url, options.headers['X-CareerOS-Action-Key']])
   if (options.method === 'POST') {
     key ??= options.headers['X-CareerOS-Action-Key']
     if (requests.length === 1) return Response.json({error:'draft_stale'}, {status:409})
     return Response.json(ready(key), {status:201})
   }
   return Response.json({error:'draft_not_found'}, {status:404})
 }
 const original = new DraftRecoveryClient('a', saved, send)
 await assert.rejects(original.create(input))
 const resumed = new DraftRecoveryClient('a', saved, send)
 await assert.rejects(resumed.recover(), /draft_not_found/)
 assert.equal(requests.length, 2)
 assert.deepEqual([...saved.values.values()], [key])
 const corrected = {...input, profileSource:{id:'p', revision:2}}
 const result = await resumed.create(corrected)
 assert.equal(result.creationKey, key)
 assert.deepEqual(requests.map(([method,,header]) => [method, header]), [
   ['POST', key], ['GET', undefined], ['POST', key]
 ])
})

test('generic 404 and an established Action cannot unlock a creation key', async () => {
 const saved = storage()
 let key
 const original = new DraftRecoveryClient('a', saved, async (_, options) => {
   key = options.headers['X-CareerOS-Action-Key']
   return Response.json(ready(key), {status:201})
 })
 await original.create(input)
 const generic = new DraftRecoveryClient('a', saved, async () => new Response('Not found', {status:404}))
 await assert.rejects(generic.recover(), /draft_outcome_unknown/)
 await assert.rejects(generic.create(input), /draft_recover_first/)
 let requestCount = 0
 const existing = new DraftRecoveryClient('a', saved, async () => {
   requestCount++
   return requestCount === 1 ? Response.json(ready(key)) : Response.json({error:'draft_not_found'}, {status:404})
 })
 await existing.recover()
 await assert.rejects(existing.recover(), /draft_outcome_unknown/)
 await assert.rejects(existing.create(input), /draft_recover_first/)
 assert.deepEqual([...saved.values.values()], [key])
})

test('corrected submit honors an existing binding conflict on the same key', async () => {
 const saved = storage()
 saved.setItem('careeros_draft_recovery:a','original-request')
 const posts = []
 const client = new DraftRecoveryClient('a', saved, async (_, options) => {
   if (options.method !== 'POST') return Response.json({error:'draft_not_found'}, {status:404})
   posts.push(options.headers['X-CareerOS-Action-Key'])
   return Response.json({error:'draft_stale'}, {status:409})
 })
 await assert.rejects(client.recover(), /draft_not_found/)
 await assert.rejects(client.create(input), /draft_outcome_unknown/)
 await assert.rejects(client.create(input), /draft_recover_first/)
 assert.deepEqual(posts, ['original-request'])
 assert.deepEqual([...saved.values.values()], ['original-request'])
})

test('managed Retry needs authenticated known failure and reuses the original operation identities', async () => {
 const saved = storage()
 const posts = []
 let key
 const client = new DraftRecoveryClient('a', saved, async (_, options) => {
   if (options.method !== 'POST') {
     const {bundle, job, ...prior} = ready(key)
     return Response.json({...prior, state:'incomplete', artifactVersion:0, attempt:1, retryAllowed:true}, {status:202})
   }
   const retry = options.headers['X-CareerOS-Retry-Key']
   const creation = options.headers['X-CareerOS-Action-Key']
   key ??= creation
   posts.push({creation, retry, body:JSON.parse(options.body)})
   if (posts.length === 1) return Response.json({error:'draft_unavailable'}, {status:503})
   if (posts.length === 2) throw new TypeError('lost Retry request before acceptance')
   const value = ready(key)
   value.bundle.binding.generationVersion = 'application-draft-local-20261104-v1'
   return Response.json({...value, attempt:2}, {status:201})
 })
 await assert.rejects(client.create(input), /draft_outcome_unknown/)
 await assert.rejects(client.retry(input), /draft_recover_first/)
 assert.equal(posts.length, 1)
 assert.equal((await client.recover()).retryAllowed, true)
 await assert.rejects(client.retry(input), /draft_outcome_unknown/)
 await assert.rejects(client.retry(input), /draft_recover_first/)
 assert.equal(posts.length, 2)
 assert.equal((await client.recover()).retryAllowed, true)
 assert.equal((await client.retry(input)).attempt, 2)
 assert.equal(posts[0].retry, undefined)
 assert.ok(posts[1].retry)
 assert.equal(posts[1].retry, posts[2].retry)
 assert.deepEqual(posts.map(p => p.creation), [key, key, key])
 assert.deepEqual(posts.map(p => p.body), [input, input, input])
 assert.deepEqual([...saved.values.values()], [key])
})
