import assert from 'node:assert/strict'
import {withAccountBrowser} from './account-browser.fixture.mjs'
for(const pt of [false,true]){
 const text={profile:pt?'Perfil':'Profile',skills:pt?'Habilidades e tecnologias':'Skills, tools & tech',review:pt?'Revisar com IA':'Review with AI',accept:pt?'Aceitar e salvar alterações':'Accept and save changes',reject:pt?'Rejeitar proposta':'Reject proposal',panel:pt?'Revisar proposta do Perfil':'Review Profile Proposal',check:pt?'Verificar status do processamento':'Check processing status'}
 const by=t=>`[...document.querySelectorAll('button')].find(b=>b.textContent.trim()===${JSON.stringify(t)})`
 const open=async({evaluate,wait})=>{await wait(`document.querySelector('header nav')`);await evaluate(`${by(text.profile)}.click()`);await wait(`${by(text.skills)}`);await evaluate(`${by(text.skills)}.click()`);await wait(`${by(text.review)}`)}
 await withAccountBrowser(async({evaluate,wait,evidence})=>{
  await open({evaluate,wait});const original=await evaluate(`JSON.stringify(managedReviewScenario.document)`)
  await evaluate(`${by(text.review)}.click()`);await wait(`document.querySelector('section[aria-label="${text.panel}"]')`)
  let posts=await evaluate(`managedReviewScenario.posts`);assert.equal(posts.length,1);assert.equal(posts[0].headers['x-careeros-account'],'synthetic-a');assert.ok(posts[0].headers['x-careeros-action-key']);assert.equal(posts[0].headers['x-openai-api-key'],undefined);assert.equal(posts[0].headers['x-typesafe-api-key'],undefined)
  assert.equal(await evaluate(`managedReviewScenario.saves.length`),0);assert.equal(await evaluate(`JSON.stringify(managedReviewScenario.document)`),original)
  await evaluate(`${by(text.reject)}.click()`);assert.equal(await evaluate(`managedReviewScenario.saves.length`),0)
  await evaluate(`${by(text.review)}.click()`);await wait(`document.querySelector('section[aria-label="${text.panel}"]')`)
  posts=await evaluate(`managedReviewScenario.posts`);assert.notEqual(posts[1].headers['x-careeros-action-key'],posts[0].headers['x-careeros-action-key'])
  await evaluate(`(()=>{const i=document.querySelector('section[aria-label="${text.panel}"] textarea');Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(i,'User-authored Java wording');i.dispatchEvent(new Event('input',{bubbles:true}))})()`)
  assert.equal(await evaluate(`managedReviewScenario.saves.length`),0)
  await evaluate(`managedReviewScenario.loseSave=true;${by(text.accept)}.click()`);await wait(`managedReviewScenario.saves.length===1&&!document.querySelector('section[aria-label="${text.panel}"]')`)
  const saved=await evaluate(`managedReviewScenario.document`);assert.ok(saved.facts.some(f=>f.value==='User-authored Java wording'&&f.origin.original==='user'))
  await evaluate(`${by(text.check)}.click()`);await wait(`managedReviewScenario.reads===1`);assert.equal(await evaluate(`managedReviewScenario.posts.length`),2);assert.equal(await evaluate(`JSON.stringify(Object.entries(sessionStorage)).includes('User-authored Java wording')`),false)
  console.log(JSON.stringify({status:'passed',locale:pt?'pt-BR':'en',evidence,checks:['actual App/account/grant/Profile proposal','no caller keys','generation/reject0writes','successful new rewrite newAction','manual edit provenance','deliberate acknowledged Save with lost reply recovery','GET-only status']}))
 },{path:'/tests/managed-profile-review-harness.html',env:{VITE_CAREEROS_PILOT_MODE:'1'},initialize:`localStorage.setItem('careeros_locale',${JSON.stringify(pt?'pt-BR':'en')})`})
 await withAccountBrowser(async({evaluate,wait,evidence})=>{
  await open({evaluate,wait});await evaluate(`${by(text.review)}.click()`);await wait(`managedReviewScenario.posts.length===1&&${by(text.review)}?.disabled===false`)
  const first=(await evaluate(`managedReviewScenario.posts`))[0]
  await evaluate(`document.querySelector('header nav button').click()`);await wait(`!${by(text.review)}`);await open({evaluate,wait})
  await evaluate(`${by(text.check)}.click()`);await wait(`managedReviewScenario.reads===1`);assert.equal(await evaluate(`managedReviewScenario.posts.length`),1)
  await evaluate(`managedReviewScenario.mode='known_failure';${by(text.check)}.click()`);await wait(`managedReviewScenario.reads===2`);assert.equal(await evaluate(`managedReviewScenario.posts.length`),1)
  await evaluate(`${by(text.review)}.click()`);await wait(`document.querySelector('section[aria-label="${text.panel}"]')`)
  const posts=await evaluate(`managedReviewScenario.posts`);assert.equal(posts.length,2);assert.equal(posts[1].headers['x-careeros-action-key'],first.headers['x-careeros-action-key']);assert.ok(posts[1].headers['x-careeros-retry-key']);assert.deepEqual(posts[1].body,first.body);assert.equal(await evaluate(`managedReviewScenario.saves.length`),0)
  console.log(JSON.stringify({status:'passed',locale:pt?'pt-BR':'en',evidence,checks:['unknown preserves saved Profile','navigation retained scoped owner','status GET0sends0writes','knownfailure deliberate Retry unchangedroot','proposal not implicitly saved']}))
 },{path:'/tests/managed-profile-review-harness.html',env:{VITE_CAREEROS_PILOT_MODE:'1'},initialize:`localStorage.setItem('careeros_locale',${JSON.stringify(pt?'pt-BR':'en')});window.reviewMode='unknown'`})
}
