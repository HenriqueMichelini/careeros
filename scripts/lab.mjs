// One local invocation builds standalone assets, then owns one loopback process.
import { spawn } from "node:child_process"
import { mkdirSync } from "node:fs"
import { fileURLToPath } from "node:url"
import { join } from "node:path"
const root = fileURLToPath(new URL("..", import.meta.url))
const run = (command, args) =>
  new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd: root, stdio: "inherit" })
    child.on("error", reject)
    child.on("exit", (code) =>
      code === 0 ? resolve() : reject(new Error("Local Lab build failed.")),
    )
  })
await run(join(root, "node_modules/.bin/vite"), [
  "build",
  "--config",
  "vite.lab.config.ts",
])
mkdirSync(join(root, "lab/.bin"), { recursive: true })
await run("go", ["build", "-o", "lab/.bin/lab-api", "./lab/api"])
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
