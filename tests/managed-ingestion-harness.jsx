// Actual App/account/grant/Profile journey, synthetic browser transport only.
import {createRoot} from 'react-dom/client'
import App from '../src/App'
import fixtures from '../internal/profiledocument/fixtures.json'
import {journeyFetch} from './submission-browser.fixture.mjs'
import '../src/index.css'
const scenario={mode:window.managedIngestionMode??'normal',posts:[],saves:[],reads:0,document:structuredClone(fixtures[0].document),loseSave:false,operations:{}}
Object.assign(window,{managedIngestionScenario:scenario,__realFetch:window.fetch})
const response=(v,status=200,headers)=>new Response(JSON.stringify(v),{status,headers})
window.fetch=async(input,init={})=>{
 const path=String(input)
 if(path==='/api/auth/session')return response({authenticated:true,account:{id:'synthetic-a'},expiresAt:new Date(Date.now()+3600000).toISOString()})
 if(path==='/api/pilot/status')return response({accountId:'synthetic-a',state:'active',canActivate:false,canDraft:true,canStandalone:true,canRetrieve:true,draftAllowance:5,expiresAt:new Date(Date.now()+14*86400000).toISOString()})
 if(path==='/api/profile/current'){
  if(init.method==='PUT'){
   const command=JSON.parse(init.body);scenario.saves.push(command);scenario.document=command.document
   const outcome={operationId:command.operationId,profileId:command.document.id,committedRevision:command.document.revision};scenario.operations[command.operationId]=outcome
   if(scenario.loseSave){scenario.loseSave=false;throw new Error('synthetic lost Save acknowledgment')}
   return response({accountId:'synthetic-a',document:scenario.document,outcome})
  }
  return response({accountId:'synthetic-a',document:scenario.document})
 }
 if(path.startsWith('/api/profile/operations/')){const outcome=scenario.operations[path.split('/').at(-1)];return outcome?response({accountId:'synthetic-a',document:scenario.document,outcome}):response({error:'not_found'},404)}
 if(path.startsWith('/api/ai/ingestion/actions/')){scenario.reads++;return response({actionId:'server',state:scenario.mode==='known_failure'?'known_failure':'unknown',attempt:1,retryAllowed:scenario.mode==='known_failure',resultAvailable:false})}
 if(path==='/api/profile/ingest'){
  const body=JSON.parse(String(init.body));scenario.posts.push({headers:Object.fromEntries(new Headers(init.headers).entries()),body})
  if(body.stage.kind==='extract'&&scenario.mode==='unknown')return response({actionId:'server',state:'unknown',attempt:1,retryAllowed:false,resultAvailable:false},202)
  if(body.stage.kind==='extract'&&scenario.mode==='known_failure')return response({error:'outage'},502,{'X-CareerOS-Retry-Allowed':'true'})
  if(body.stage.kind==='extract'&&scenario.mode==='paused')return response({error:'qualification_paused'},429)
  return journeyFetch(path,init)
 }
 throw new Error('Unexpected managed ingestion fixture path '+path)
}
createRoot(document.getElementById('root')).render(<App/> )
