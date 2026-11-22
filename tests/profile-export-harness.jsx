import {createRoot} from 'react-dom/client'
import App from '../src/App'
import '../src/index.css'
const nativeFetch=window.fetch.bind(window)
await nativeFetch('/api/pilot/export-test/bootstrap?token='+encodeURIComponent(window.exportToken))
window.exportWire={statuses:[],methods:[]}
window.fetch=async(input,init={})=>{
 const response=await nativeFetch(input,init)
 if(String(input)==='/api/auth/session'){exportWire.owner=(await response.clone().json()).account?.id??null}
 if(String(input)==='/api/profile/export'){
  exportWire.statuses.push(response.status);exportWire.methods.push(init.method??'GET')
  if(window.holdExport){window.holdExport=false;await new Promise(resolve=>window.releaseExport=resolve)}
 }
 return response
}
createRoot(document.getElementById('root')).render(<App/>)
