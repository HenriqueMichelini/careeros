// Actual App/AccountGate/PilotAccess/HomePage, synthetic browser transport only.
// Real qualification adapters and PostgreSQL authority are tested in Go.
import { createRoot } from "react-dom/client"
import App from "../src/App"
import fixtures from "../internal/profiledocument/fixtures.json"
import "../src/index.css"

const scenario = {
  mode: "unknown", posts: [] as {headers:Record<string,string>;body:unknown}[], reads: 0,
  context: (window as Window & {managedContext?:unknown}).managedContext,
}
Object.assign(window,{managedScenario:scenario})
const response=(value:unknown,status=200,headers?:HeadersInit)=>new Response(JSON.stringify(value),{status,headers})
window.fetch=async(input,init={})=>{
 const path=String(input)
 if(path==="/api/auth/session")return response({authenticated:true,account:{id:"synthetic-a"},expiresAt:new Date(Date.now()+3600000).toISOString()})
 if(path==="/api/pilot/status")return response({accountId:"synthetic-a",state:"active",canActivate:false,canDraft:true,canStandalone:true,canRetrieve:true,draftAllowance:5,expiresAt:new Date(Date.now()+14*86400000).toISOString()})
 if(path==="/api/profile/current")return response({accountId:"synthetic-a",document:fixtures[0].document})
 if(path.startsWith("/api/ai/qualification/actions/")){scenario.reads++;return response({actionId:"server-action",state:scenario.mode==="known_failure"?"known_failure":scenario.mode==="success"?"result_unavailable":"unknown",attempt:1,retryAllowed:scenario.mode==="known_failure",resultAvailable:false})}
 if(path==="/api/qualification-gaps"){
  const headers=Object.fromEntries(new Headers(init.headers).entries());scenario.posts.push({headers,body:JSON.parse(String(init.body))})
  if(scenario.mode==="unknown")return response({actionId:"server-action",state:"unknown",attempt:1,retryAllowed:false,resultAvailable:false},202)
  if(scenario.mode==="paused")return response({error:"qualification_paused"},429)
  if(scenario.mode==="known_failure")return response({error:"outage"},502,{"X-CareerOS-Retry-Allowed":"true"})
  return response({gaps:[],matches:[],jobContext:scenario.context})
 }
 throw new Error("Unexpected browser fixture path: "+path)
}
createRoot(document.getElementById("root")!).render(<App />)
