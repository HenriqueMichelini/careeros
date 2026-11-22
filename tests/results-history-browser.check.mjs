import assert from 'node:assert/strict'
import {withAccountBrowser,pause} from './account-browser.fixture.mjs'

await withAccountBrowser(async({call,evaluate,wait,evidence})=>{
 await wait('window.historyFixture?.ready')
 await evaluate("document.querySelectorAll('.results-tabs button')[1].click()")
 await wait("[...document.querySelectorAll('button')].some(b=>b.textContent.trim()==='Copy text'&&!b.disabled)")
 const original=await evaluate('structuredClone(historyFixture.accepted)')
 await call('Runtime.evaluate',{expression:"window.historySecond=window.open(location.href+'?second=1','careeros-history-second');!!window.historySecond",userGesture:true,returnByValue:true})
 assert.equal(await evaluate('!!window.historySecond'),true,'synthetic second tab was blocked')
 try {
  await wait('window.historySecond?.historyFixture?.ready')
  const changed=await evaluate("historySecond.historyFixture.edit('Java, Go')")
  assert.equal(changed,true)
  await wait("historyFixture.error==='stale'")
  const after=await evaluate(`({retained:historyFixture.source,canonical:JSON.parse(localStorage.getItem('careeros_profile_v2')),copy:[...document.querySelectorAll('button')].find(b=>b.textContent.trim()==='Copy text').disabled,warning:!!document.querySelector('[data-results-history]')})`)
  assert.equal(after.retained.revision,original.sourceSnapshot.revision)
  assert.equal(after.canonical.revision,original.sourceSnapshot.revision+1)
  const blocked=await evaluate("historyFixture.edit('Unwanted stale write')")
  assert.equal(blocked,false)
  assert.deepEqual(await evaluate("JSON.parse(localStorage.getItem('careeros_profile_v2'))"),after.canonical)
  console.log(JSON.stringify({checkpoint:'issue135-reproduction',evidence,profileId:original.sourceSnapshot.id,originalRevision:original.sourceSnapshot.revision,canonicalRevision:after.canonical.revision,profileError:'stale',exportEnabled:!after.copy,historicalWarning:after.warning,canonicalStaleWriteBlocked:true,providerRequests:await evaluate('historyRequests.length')}))
  assert.equal(after.warning,true,'stale Results export has no historical disclosure')
  assert.equal(after.copy,false,'selected historical snapshot must remain exportable')
  await evaluate('historyFixture.refresh()')
  await wait('historyFixture.error===null')
  assert.equal(await evaluate("[...document.querySelectorAll('button')].find(b=>b.textContent.trim()==='Copy text').disabled"),false,'canonical refresh disabled approved historical export')
  assert.deepEqual(await evaluate('historyFixture.accepted'),original)
  assert.equal(await evaluate('historyRequests.length'),0)
  console.log(JSON.stringify({status:'passed',scenario:'issue135-history',evidence,checks:['two actual same-origin synthetic Profile tabs','storage-event stale Profile retains accepted snapshot','canonical stale writes independently rejected','historical disclosure and export preserved after canonical refresh','zero provider requests']}))
 } finally { await evaluate('historySecond?.close()') }
},{path:'/tests/results-history-harness.html',initialize:"localStorage.removeItem('careeros_profile_v2');localStorage.removeItem('careeros_repo')"})
