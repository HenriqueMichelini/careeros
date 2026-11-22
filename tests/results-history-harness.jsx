import {createRoot} from 'react-dom/client'
import {useEffect,useRef} from 'react'
import {StoreProvider,useStore} from '../src/lib/store'
import {migrateProfile,emptyProfileView,validateProfileDocument} from '../src/lib/profileDocument'
import {PROFILE_KEY} from '../src/lib/profileStorage'
import {acceptResume} from '../src/lib/resumeReview'
import ResultsPage from '../src/pages/ResultsPage'
import '../src/index.css'

if (!localStorage.getItem(PROFILE_KEY)) {
 const initial=migrateProfile({...emptyProfileView(),fullName:'Original Person',email:'original@example.com',skills:'Java'},'history-profile')
 if(!validateProfileDocument(initial))throw new Error('invalid synthetic Profile')
 localStorage.setItem(PROFILE_KEY,JSON.stringify(initial))
}
window.historyRequests=[]
window.fetch=async (...args)=>{window.historyRequests.push(String(args[0]));throw new Error('This offline history fixture has no provider or account transport')}

function Fixture(){
 const store=useStore(), seeded=useRef(false)
 useEffect(()=>{
  const doc=store.profileDocument
  if(!doc||seeded.current)return
  seeded.current=true
  const fact=doc.facts.find(f=>f.field==='skills')
  const review={id:'history-review',version:'resume-review-v1',check:'complete',facts:[],sourceSnapshot:structuredClone(doc),identityProfile:structuredClone(store.state.repository),cvLanguage:'en',claims:[{section:'skills',kind:'bullet',text:'Java',sources:[{profileId:doc.id,id:fact.id,revision:fact.revision}],state:'supported',concerns:[]}]}
  const accepted=acceptResume(review,doc)
  store.dispatch({type:'SET_MATERIALS',payload:{jobTitle:'Java role',company:null,jobSummary:'Java role.',resume:accepted.markdown,coverLetter:'',applicationAnswers:'',resumeReview:review,acceptedResume:accepted,cvLanguage:'en'}})
 },[store.profileDocument])
 window.historyFixture={
  ready:!!store.state.generatedMaterials,
  source:store.profileDocument,
  error:store.state.profileError,
  accepted:store.state.generatedMaterials?.acceptedResume,
  edit:value=>store.editCanonicalProfile({type:'field',ownerId:store.profileDocument.id,field:'skills',value}),
  refresh:()=>store.refreshCanonicalProfile(),
 }
 return <ResultsPage setPage={()=>{}}/>
}
createRoot(document.getElementById('root')).render(<StoreProvider><Fixture/></StoreProvider>)
