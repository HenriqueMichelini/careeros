// One local invocation co-builds owned manifests and owns one loopback process.
import { spawn } from "node:child_process"
import { createHash } from "node:crypto"
import { mkdirSync, readFileSync, writeFileSync, renameSync } from "node:fs"
import { fileURLToPath } from "node:url"
import { join } from "node:path"
const root = fileURLToPath(new URL("..", import.meta.url))
const buildEnv = { ...process.env }
// Prevent ambient Go flags/workspaces/toolchain selection from changing the
// owned recipe. Use installed/cached dependencies only, with CGO disabled.
for (const key of Object.keys(buildEnv))
  if (key.startsWith("GO") && key !== "GOCACHE" && key !== "GOPATH")
    delete buildEnv[key]
Object.assign(buildEnv, {
  GOENV: "off",
  GOWORK: "off",
  GOFLAGS: "",
  GOTOOLCHAIN: "local",
  CGO_ENABLED: "0",
  GOPROXY: "off",
})
const run = (command, args, capture = false) =>
  new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: root,
      env: buildEnv,
      stdio: capture ? ["ignore", "pipe", "inherit"] : "inherit",
    })
    let output = ""
    if (capture)
      child.stdout.on("data", (chunk) => {
        output += chunk
        if (output.length > 1024 * 1024) child.kill()
      })
    child.on("error", reject)
    child.on("exit", (code) =>
      code === 0
        ? resolve(output)
        : reject(new Error("Local Lab build failed.")),
    )
  })
// Inspection builds the real API binary without rebuilding unrelated UI assets.
if (!process.argv.slice(2).includes("--build-info"))
  await run(join(root, "node_modules/.bin/vite"), [
    "build",
    "--config",
    "vite.lab.config.ts",
  ])
mkdirSync(join(root, "lab/.bin"), { recursive: true })
const manifestJSON = await run("go", ["run", "./lab/buildmanifest"], true)
const manifest = JSON.parse(manifestJSON)
const encoded = Buffer.from(manifestJSON).toString("base64")
const command = [
  "build",
  "-trimpath",
  "-buildvcs=true",
  `-ldflags=-X=professional-information-repo/internal/evaluationlab.compiledBuildManifest=${encoded}`,
  "-o",
  "lab/.bin/lab-api",
  "./lab/api",
]
await run("go", command)
const after = JSON.parse(await run("go", ["run", "./lab/buildmanifest"], true))
if (after.hash !== manifest.hash)
  throw new Error("Lab source changed during build.")
const digest = (bytes) => createHash("sha256").update(bytes).digest("hex")
const receipt = {
  version: "lab-build-receipt-v1",
  manifestHash: manifest.hash,
  executableHash: digest(readFileSync(join(root, "lab/.bin/lab-api"))),
  launcherHash: digest(readFileSync(join(root, "scripts/lab.mjs"))),
  runtime: manifest.runtime,
  command: [
    "go",
    ...command.map((value) =>
      value.startsWith("-ldflags=")
        ? "-ldflags=-X=professional-information-repo/internal/evaluationlab.compiledBuildManifest=<base64-manifest>"
        : value,
    ),
  ],
}
const receiptPath = join(root, "lab/.bin/lab-build-receipt.json")
writeFileSync(`${receiptPath}.tmp`, JSON.stringify(receipt), { mode: 0o600 })
renameSync(`${receiptPath}.tmp`, receiptPath)
const child = spawn(join(root, "lab/.bin/lab-api"), process.argv.slice(2), {
  cwd: root,
  stdio: "inherit",
})
for (const signal of ["SIGINT", "SIGTERM"])
  process.on(signal, () => child.kill(signal))
child.on("error", () => {
  console.error("Local Lab could not launch.")
  process.exitCode = 1
})
child.on("exit", (code) => {
  process.exitCode = code ?? 1
})
