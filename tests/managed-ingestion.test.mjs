import test from 'node:test'
import assert from 'node:assert/strict'
import {mkdtempSync,readFileSync,writeFileSync} from 'node:fs'
import {tmpdir} from 'node:os'
import {join} from 'node:path'
import ts from 'typescript'
const dir=mkdtempSync(join(tmpdir(),'managed-ingestion-'))
for(const name of ['managedQualification','managedIngestion','workflowRequest']){
 const source=readFileSync(new URL(`../src/lib/${name}.ts`,import.meta.url),'utf8')
 writeFileSync(join(dir,`${name}.mjs`),ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText.replaceAll('"./workflowRequest"','"./workflowRequest.mjs"').replaceAll('"./managedQualification"','"./managedQualification.mjs"'))
}
const {ManagedIngestion,managedIngestionOwner}=await import(join(dir,'managedIngestion.mjs'))
test('all stages share the root identity, changed answer is a stage not a new financial Action',()=>{
 const client=new ManagedIngestion('a')
 const root={input:'complete fictional career information',profile:{skills:'Java'}}
 const plan=client.stage({...root,stage:{version:2,kind:'plan'}})
 const extract=client.stage({...root,stage:{version:2,kind:'extract'},planReceipt:'signed',batchIndex:0})
 assert.deepEqual(extract,plan)
 assert.deepEqual(client.stage({...root,stage:{version:2,kind:'clarify'},answer:'Changed answer'}),plan)
 client.knownFailure(true)
 const retry=client.stage({...root,stage:{version:2,kind:'reconcile'}})
 assert.equal(retry.actionKey,plan.actionKey);assert.ok(retry.retryKey)
 assert.deepEqual(client.stage({...root,stage:{version:2,kind:'reconcile'}}),retry)
 assert.notEqual(client.stage({...root,input:'new complete submission'}).actionKey,plan.actionKey)
})
test('ingestion status recovery uses only its own account-scoped GET endpoint',async()=>{
 globalThis.document={cookie:''};const previous=globalThis.fetch;const calls=[]
 globalThis.fetch=async(url,init)=>{calls.push({url,init});return new Response(JSON.stringify({actionId:'server',state:'unknown',attempt:1,retryAllowed:false,resultAvailable:false}))}
 try{const client=new ManagedIngestion('a',{getItem:()=> 'opaque'});await client.status();assert.equal(calls[0].url,'/api/ai/ingestion/actions/opaque');assert.equal(calls[0].init.method,undefined);assert.equal(calls[0].init.headers['X-OpenAI-Api-Key'],undefined)}finally{globalThis.fetch=previous;delete globalThis.document}
})
test('changed clarification answer is a new child without allocating financial retry',()=>{
 const client=new ManagedIngestion('a');const root={input:'complete root',profile:{skills:'Java'}}
 const first=client.stage({...root,stage:{version:2,kind:'clarify'},claim:{id:'c'},answer:'first'})
 client.knownFailure(true)
 const changed=client.stage({...root,stage:{version:2,kind:'clarify'},claim:{id:'c'},answer:'changed'})
 assert.equal(changed.actionKey,first.actionKey);assert.equal(changed.retryKey,undefined)
 client.knownFailure(true)
 const retry=client.stage({...root,stage:{version:2,kind:'clarify'},claim:{id:'c'},answer:'changed'})
 assert.equal(retry.actionKey,first.actionKey);assert.ok(retry.retryKey)
 assert.deepEqual(client.stage({...root,stage:{version:2,kind:'clarify'},claim:{id:'c'},answer:'changed'}),retry)
})
test('retained submission transport and remounted status use one owner per account',()=>{
 const first=managedIngestionOwner('profile-field','remount-a');const body={input:'complete root',profile:{skills:'Java'},stage:{version:2,kind:'extract'}}
 const before=first.stage(body)
 const remounted=managedIngestionOwner('profile-field','remount-a');assert.equal(remounted,first)
 remounted.knownFailure(true)
 const retry=first.stage(body);assert.equal(retry.actionKey,before.actionKey);assert.ok(retry.retryKey)
 assert.notEqual(managedIngestionOwner('profile-field','remount-b'),first)
 assert.notEqual(managedIngestionOwner('independent-field','remount-a'),first)
 assert.notEqual(first.stage({...body,document:{id:'changed-document',revision:2}}).actionKey,before.actionKey)
 const values=new Map();const storage={getItem:key=>values.get(key)??null,setItem:(key,value)=>values.set(key,value),removeItem:key=>values.delete(key)}
 const fieldA=managedIngestionOwner('field-a','scope-storage-account',storage).stage(body)
 const fieldB=managedIngestionOwner('field-b','scope-storage-account',storage).stage(body)
 assert.notEqual(fieldA.actionKey,fieldB.actionKey);assert.equal(values.size,2)
})
