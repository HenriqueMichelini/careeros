// Actual App/account/grant/Profile proposal UI; synthetic browser responses.
import {createRoot} from 'react-dom/client'
import App from '../src/App'
import fixtures from '../internal/profiledocument/fixtures.json'
import {reviewableFact} from '../src/lib/sectionReview'
import '../src/index.css'
const scenario={mode:window.reviewMode??'normal',posts:[],saves:[],reads:0,document:structuredClone(fixtures[0].document),operations:{},loseSave:false}
const completedKeys=new Set(JSON.parse(sessionStorage.getItem('synthetic.review.completed.keys')??'[]'))
Object.assign(window,{managedReviewScenario:scenario})
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
 if(path.startsWith('/api/ai/profile-review/actions/')){scenario.reads++;const completed=completedKeys.has(path.split('/').at(-1));return response({actionId:'server',state:completed?'result_unavailable':scenario.mode==='known_failure'?'known_failure':'unknown',attempt:1,retryAllowed:!completed&&scenario.mode==='known_failure',resultAvailable:false})}
 if(path==='/api/profile/review'){
  const body=JSON.parse(String(init.body));scenario.posts.push({headers:Object.fromEntries(new Headers(init.headers).entries()),body})
  const key=new Headers(init.headers).get('X-CareerOS-Action-Key')
  if(completedKeys.has(key))return response({actionId:'server',state:'result_unavailable',attempt:1,retryAllowed:false,resultAvailable:false},202)
  if(scenario.mode==='unknown')return response({actionId:'server',state:'unknown',attempt:1,retryAllowed:false,resultAvailable:false},202)
  if(scenario.mode==='known_failure'&&!new Headers(init.headers).get('X-CareerOS-Retry-Key'))return response({error:'outage'},502,{'X-CareerOS-Retry-Allowed':'true'})
  if(scenario.mode==='paused')return response({error:'qualification_paused'},429)
  completedKeys.add(key);sessionStorage.setItem('synthetic.review.completed.keys',JSON.stringify([...completedKeys]))
  return response({profileId:body.document.id,revision:body.document.revision,section:body.section,summary:'Synthetic clearer wording',patches:body.document.facts.filter(f=>reviewableFact(body.document,body.section,f)).map(f=>({factId:f.id,revision:f.revision,wording:String(f.value)+'.',supporting:[{id:f.id,revision:f.revision}]}))})
 }
 throw new Error('Unexpected managed review fixture path '+path)
}
createRoot(document.getElementById('root')).render(<App/> )
