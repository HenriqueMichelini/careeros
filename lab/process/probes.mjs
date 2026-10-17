// Fixed local guard probes. No caller-supplied hosts, paths, environment or code.
import assert from "node:assert/strict"
import { existsSync, statSync } from "node:fs"
import { spawn } from "node:child_process"
import { homedir, tmpdir } from "node:os"
import { dirname, join } from "node:path"

const mode = process.argv[2]
if (mode === "hang-tree" || mode === "child") {
  const child = spawn(
    process.execPath,
    [
      new URL(import.meta.url).pathname,
      mode === "hang-tree" ? "child" : "grandchild",
    ],
    { detached: true, stdio: ["ignore", "ignore", "ignore", "ipc"] },
  )
  child.on("message", () => {
    if (mode === "hang-tree")
      console.log(JSON.stringify({ kind: "lab-probe-tree-ready", children: 2 }))
    else process.send({ ready: true })
  })
  setInterval(() => {}, 100)
} else if (mode === "grandchild") {
  process.send({ ready: true })
  setInterval(() => {
    /* Keep the ready detached descendant alive until supervisor cancellation. */
  }, 50)
} else if (mode === "assertion") {
  assert.equal("observed", "expected", "deliberate negative assertion fixture")
} else if (mode === "crash") {
  process.kill(process.pid, "SIGKILL")
} else if (mode === "output") {
  // A finite 2 MiB flood exceeds the 128 KiB supervisor cap without an
  // invariant loop or unlimited writes if a regression disables the cap.
  await emitChunks(256)
} else if (mode === "isolation") {
  const home = homedir()
  assert.equal(home, join(tmpdir(), "home"), "Probe requires isolated HOME")
  assert.equal(statSync(home).mode & 0o777, 0o700, "Private HOME permissions")
  let hostReachable = false
  try {
    await fetch("http://127.0.0.1:54679", { signal: AbortSignal.timeout(200) })
    hostReachable = true
  } catch {}
  const evidence = {
    credentialEnvAbsent: [
      "OPENAI_API_KEY",
      "TYPESAFE_API_KEY",
      "AWS_ACCESS_KEY_ID",
      "AWS_SECRET_ACCESS_KEY",
      "GH_TOKEN",
    ].every((name) => process.env[name] === undefined),
    hostFileAbsent: !existsSync(
      join(dirname(home), "careeros-lab-host-sentinel"),
    ),
    hostNetworkUnreachable: !hostReachable,
    privateHome: process.env.HOME === home && existsSync(home),
  }
  await new Promise((resolve) =>
    process.stdout.write(
      JSON.stringify({ kind: "lab-isolation-probe-v1", evidence }) + "\n",
      resolve,
    ),
  )
  assert.ok(
    Object.values(evidence).every(Boolean),
    "Controlled isolation sentinel failed",
  )
} else throw new Error("Unknown fixed guard probe")

async function emitChunks(remaining) {
  if (remaining === 0) return
  await new Promise((resolve) =>
    process.stdout.write("X".repeat(8192), resolve),
  )
  return emitChunks(remaining - 1)
}
