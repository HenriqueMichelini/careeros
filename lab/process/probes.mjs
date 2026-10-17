// Fixed local guard probes. No caller-supplied hosts, paths, environment or code.
import assert from "node:assert/strict"
import { existsSync, appendFileSync } from "node:fs"
import { spawn } from "node:child_process"
import { once } from "node:events"

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
  appendFileSync("/tmp/process-tree-heartbeat", "started\n")
  process.send({ ready: true })
  setInterval(
    () => appendFileSync("/tmp/process-tree-heartbeat", "alive\n"),
    50,
  )
} else if (mode === "assertion") {
  assert.equal("observed", "expected", "deliberate negative assertion fixture")
} else if (mode === "crash") {
  process.kill(process.pid, "SIGKILL")
} else if (mode === "output") {
  const chunk = "X".repeat(8192)
  while (true) {
    if (!process.stdout.write(chunk)) await once(process.stdout, "drain")
  }
} else if (mode === "isolation") {
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
    hostFileAbsent: !existsSync("/tmp/careeros-lab-host-sentinel"),
    hostNetworkUnreachable: !hostReachable,
    privateHome: process.env.HOME === "/tmp/home" && existsSync("/tmp/home"),
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
