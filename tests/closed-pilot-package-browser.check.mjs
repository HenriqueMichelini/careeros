import assert from "node:assert/strict"
import { withAccountBrowser } from "./account-browser.fixture.mjs"

const origin = process.argv[2]
assert.match(origin ?? "", /^http:\/\/127\.0\.0\.1:[1-9][0-9]*$/)

await withAccountBrowser(async ({ evaluate, wait, localRequests, externalRequests }) => {
  await wait("document.querySelector('#root')?.childElementCount > 0")
  await evaluate("document.fonts.ready.then(() => true)")
  await wait("document.readyState === 'complete'")
  assert.ok(localRequests.some((p) => /^\/assets\/.*\.js$/.test(p)), "packaged JavaScript should load")
  assert.ok(localRequests.some((p) => /^\/assets\/.*\.css$/.test(p)), "packaged CSS should load")
  assert.deepEqual(externalRequests, [], "closed package should request only its own assets")
  console.log(JSON.stringify({ rendered: true, local_asset_requests: localRequests, external_requests: 0 }))
}, { origin, path: "/" })
