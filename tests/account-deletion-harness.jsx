import {createRoot} from 'react-dom/client'
import App from '../src/App'
import '../src/index.css'
const nativeFetch=window.fetch.bind(window)
await nativeFetch('/api/pilot/deletion-test/bootstrap?token='+encodeURIComponent(window.deletionToken))
window.deletionWire={calls:[]}
window.fetch=async(input,init={})=>{
 const response=await nativeFetch(input,init)
 deletionWire.calls.push({path:String(input),method:init.method??'GET',status:response.status})
 if(String(input)==='/api/auth/session')deletionWire.session=await response.clone().json()
 if(String(input)==='/api/profile/export'&&window.holdExport){window.holdExport=false;await new Promise(resolve=>window.releaseExport=resolve)}
 if(String(input)==='/api/deletion/operations'&&window.dropDeletionResponse){window.dropDeletionResponse=false;throw Error('synthetic lost committed response')}
 return response
}
createRoot(document.getElementById('root')).render(<App/> )
