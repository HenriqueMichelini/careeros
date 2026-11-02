import assert from "node:assert/strict"
import {withAccountBrowser} from "./account-browser.fixture.mjs"
import {jobContextFixture} from "./job-context-fixtures.mjs"
for(const pt of [false,true]){
 const analyze=pt?"Analisar qualificações":"Analyze qualifications"
 const unknown=pt?"Esta tentativa pode ter chegado":"This attempt may have reached"
 await withAccountBrowser(async({evaluate,wait,evidence})=>{
  await wait(`document.querySelector('textarea') && document.querySelector('[data-qualification-status]')`)
  assert.equal(await evaluate(`document.querySelector('[data-api-key-settings],#openai-key,#job-typesafe-key')`),null)
  assert.equal(await evaluate(`managedScenario.posts.length`),0)
  await evaluate(`(()=>{const input=document.querySelector('textarea');Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(input,'Java developer');input.dispatchEvent(new Event('input',{bubbles:true}))})()`)
  await wait(`[...document.querySelectorAll('button')].some(b=>b.textContent===${JSON.stringify(analyze)}&&!b.disabled)`)
  await evaluate(`[...document.querySelectorAll('button')].find(b=>b.textContent===${JSON.stringify(analyze)}).click()`)
  await wait(`managedScenario.posts.length===1 && document.body.innerText.includes(${JSON.stringify(unknown)})`)
  const first=await evaluate(`managedScenario.posts[0]`)
  assert.equal(first.headers['x-openai-api-key'],undefined);assert.equal(first.headers['x-typesafe-api-key'],undefined)
  assert.equal(first.headers['x-careeros-account'],'synthetic-a');assert.ok(first.headers['x-careeros-action-key'])
  await evaluate(`document.querySelector('[data-qualification-status]').click()`)
  await wait(`managedScenario.reads===1`)
  assert.equal(await evaluate(`managedScenario.posts.length`),1)
  // A status read supplies known-failure authority; only the next explicit
  // button click creates the separately bounded retry command.
  await evaluate(`managedScenario.mode='known_failure';document.querySelector('[data-qualification-status]').click()`)
  await wait(`managedScenario.reads===2`)
  assert.equal(await evaluate(`managedScenario.posts.length`),1)
  await evaluate(`managedScenario.mode='success';[...document.querySelectorAll('button')].find(b=>b.textContent===${JSON.stringify(pt?'Tentar gerar novamente':'Try generation again')}).click()`)
  await wait(`managedScenario.posts.length===2 && document.querySelector('[role=dialog]')`)
  const retry=await evaluate(`managedScenario.posts[1]`)
  assert.equal(retry.headers['x-careeros-action-key'],first.headers['x-careeros-action-key'])
  assert.ok(retry.headers['x-careeros-retry-key']);assert.deepEqual(retry.body,first.body)
  assert.equal(await evaluate(`document.querySelector('[data-api-key-settings],#openai-key,#job-typesafe-key')`),null)
  // Unsupported next workflow is visibly closed and produces no POST.
  await evaluate(`[...document.querySelectorAll('[role=dialog] button')].find(b=>b.textContent.includes(${JSON.stringify(pt?'Gerar':'Generate')})).click()`)
  assert.equal(await evaluate(`managedScenario.posts.length`),2)
  await evaluate(`document.querySelector('[data-qualification-status]').click()`)
  await wait(`document.body.innerText.includes(${JSON.stringify(pt?'seu resultado era temporário':'its result was transient')})`)
  assert.equal(await evaluate(`managedScenario.posts.length`),2)
  await wait(`document.querySelector('[data-qualification-new]:not(:disabled)')`)
  await evaluate(`managedScenario.mode='paused';document.querySelector('[data-qualification-new]').click()`)
  await wait(`managedScenario.posts.length===3 && document.body.innerText.includes(${JSON.stringify(pt?'A análise está pausada':'Qualification is paused')})`)
  const fresh=await evaluate(`managedScenario.posts[2]`)
  assert.notEqual(fresh.headers['x-careeros-action-key'],first.headers['x-careeros-action-key'])
  assert.equal(await evaluate(`[...document.querySelectorAll('details')].some(d=>d.textContent.includes('Java developer'))`),true)
  assert.equal(await evaluate(`JSON.stringify(Object.entries(sessionStorage)).includes('Java developer')`),false)
  console.log(JSON.stringify({status:'passed',locale:pt?'pt-BR':'en',evidence,checks:['actual App/account/grant/HomePage','no credential controls or credential headers','no automatic inference/status resend','known-failure explicit retry preserves exact input and creation key','unsupported Draft remains closed','honest transient result recovery preserves analysis']}))
 },{path:'/tests/managed-qualification-harness.html',env:{VITE_CAREEROS_PILOT_MODE:'1'},initialize:`localStorage.setItem('careeros_locale',${JSON.stringify(pt?'pt-BR':'en')});window.managedContext=${JSON.stringify(jobContextFixture('Java developer'))}`})
}
