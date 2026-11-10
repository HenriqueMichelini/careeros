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
 if(path.startsWith('/api/'))wire.requests.push({path,method,actionKey:headers.get('X-CareerOS-Action-Key'),callerKey:headers.has('X-OpenAI-Api-Key')||headers.has('X-TypeSafe-Api-Key'),body:typeof init.body==='string'?JSON.parse(init.body):null})
 const response=await nativeFetch(input,init)
 if(path==='/api/drafts'||/^\/api\/drafts\/[^/]+(?:\/versions\/1)?$/.test(path)){
  wire.responses.push({path,method,status:response.status,body:await response.clone().json()})
  if(path==='/api/drafts'&&method==='POST'&&window.dropDraftResponse){window.dropDraftResponse=false;throw new TypeError('Synthetic lost response AFTER actual server publication')}
 }
 return response
}
createRoot(document.getElementById('root')).render(<App/> )
