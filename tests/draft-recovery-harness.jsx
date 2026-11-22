import {createRoot} from 'react-dom/client'
import App from '../src/App'
import '../src/index.css'
const nativeFetch=window.fetch.bind(window)
const token=window.draftControlToken
const bootstrap=await nativeFetch('/api/drafts/test/bootstrap?token='+encodeURIComponent(token))
if(!bootstrap.ok)throw new Error('Private real-router bootstrap failed')
const wire={documentID:crypto.randomUUID(),requests:[],responses:[]}
window.draftWire=wire
window.fetch=async(input,init={})=>{
 const path=String(input)
 const method=init.method??'GET'
 const headers=new Headers(init.headers)
 if(path.startsWith('/api/'))wire.requests.push({path,method,actionKey:headers.get('X-CareerOS-Action-Key'),retryKey:headers.get('X-CareerOS-Retry-Key'),callerKey:headers.has('X-OpenAI-Api-Key')||headers.has('X-TypeSafe-Api-Key'),body:typeof init.body==='string'?JSON.parse(init.body):null})
 const response=await nativeFetch(input,init)
 if(path==='/api/qualification-gaps'||path.startsWith('/api/ai/qualification/actions/'))wire.responses.push({path,method,status:response.status,action:response.headers.get('X-CareerOS-Action'),proof:response.headers.get('X-CareerOS-Draft-Context-Proof')!==null,body:await response.clone().json()})
 if(path==='/api/drafts'||/^\/api\/drafts\/[^/]+(?:\/cancel|\/versions\/1(?:\/review|\/opening(?:\/confirm)?)?)?$/.test(path)){
  wire.responses.push({path,method,status:response.status,body:await response.clone().json()})
  if(method==='GET' && response.status===410 && wire.holdNextTerminal){wire.holdNextTerminal=false;await new Promise(resolve=>{wire.releaseTerminal=resolve})}
  if(method==='GET' && response.status===200 && wire.holdNextReady){
   wire.holdNextReady=false
   await new Promise(resolve=>{wire.releaseReady=resolve})
  }
  if(path==='/api/drafts'&&method==='POST'&&response.status===201&&window.dropDraftResponse){window.dropDraftResponse=false;throw new TypeError('Synthetic lost response AFTER actual server publication')}
  if(path.endsWith('/review')&&method==='POST'&&response.status===200&&window.dropReviewResponse){window.dropReviewResponse=false;throw new TypeError('Synthetic lost response AFTER actual review commit')}
  if(path.endsWith('/opening/confirm')&&method==='POST'&&response.status===200&&window.dropOpeningResponse){window.dropOpeningResponse=false;throw new TypeError('Synthetic lost response AFTER actual opening commit')}
  if(path.endsWith('/cancel')&&method==='POST'&&response.status===200&&window.dropCancellationResponse){window.dropCancellationResponse=false;throw new TypeError('Synthetic lost response AFTER actual cancellation commit')}
 }
 return response
}
createRoot(document.getElementById('root')).render(<App/> )
