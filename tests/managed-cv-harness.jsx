import {createRoot} from 'react-dom/client'
import App from '../src/App'
import fixtures from '../internal/profiledocument/fixtures.json'
import '../src/index.css'
const doc=structuredClone(fixtures[0].document)
doc.entities=[];doc.facts[0].context=[];doc.facts[0].value='Java';doc.facts[0].assertion='affirmed';doc.facts[0].intent='actual';doc.facts[0].certainty='certain';doc.facts[0].support='unsupported';doc.evidence=[];doc.links=[]
if(window.cvSparse){doc.entities=[{id:'ed',revision:1,kind:'education',legacyId:'old-ed',order:0}];doc.facts[0].owner={profileId:'p',id:'ed',revision:1};doc.facts[0].field='degree';doc.facts[0].value='Computer science'}
const scenario={document:doc,mode:window.cvMode??'normal',posts:[],reads:0,profileWrites:0}
Object.assign(window,{managedCVScenario:scenario})
const response=(v,status=200,headers)=>new Response(JSON.stringify(v),{status,headers})
window.fetch=async(input,init={})=>{
 const path=String(input)
 if(path==='/api/auth/session')return response({authenticated:true,account:{id:'synthetic-a'},expiresAt:new Date(Date.now()+3600000).toISOString()})
 if(path==='/api/pilot/status')return response({accountId:'synthetic-a',state:'active',canActivate:false,canDraft:true,canStandalone:true,canRetrieve:true,draftAllowance:5,expiresAt:new Date(Date.now()+14*86400000).toISOString()})
 if(path==='/api/profile/current'){if(init.method==='PUT')scenario.profileWrites++;return response({accountId:'synthetic-a',document:doc})}
 if(path.startsWith('/api/ai/cv/actions/')){scenario.reads++;return response({actionId:'server',state:scenario.mode==='known_failure'?'known_failure':scenario.mode==='unknown'?'unknown':'result_unavailable',attempt:1,retryAllowed:scenario.mode==='known_failure',reviewRequired:scenario.mode==='uncertain',resultAvailable:false})}
 if(path==='/api/cv/generate'){
  const body=JSON.parse(String(init.body));const headers=Object.fromEntries(new Headers(init.headers).entries());scenario.posts.push({body,headers})
  if(scenario.mode==='unknown')return response({actionId:'server',state:'unknown',attempt:1,retryAllowed:false,resultAvailable:false},202)
  if(scenario.mode==='known_failure'&&!headers['x-careeros-retry-key'])return response({error:'outage'},502,{'X-CareerOS-Retry-Allowed':'true'})
  const f=body.facts[0];return response({schemaVersion:3,summary:[{sourceIds:[f.id],text:f.text}],selected:[f.id],wording:{},support:{policyVersion:'cv-statement-support-v1',assessment:scenario.mode==='uncertain'?'unavailable':'complete',statements:[{id:'summary:0',text:f.text,sourceIds:[f.id],status:scenario.mode==='uncertain'?'uncertain':'supported',reason:'Synthetic judgment'}]}})
 }
 throw new Error('Unexpected managed CV fixture '+path)
}
createRoot(document.getElementById('root')).render(<App/> )
