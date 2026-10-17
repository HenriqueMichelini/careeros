// Fixed harness diagnostics: preserve otherwise ignored child startup failures.
// This module never accepts commands or changes synthetic request interception.
import childProcess from "node:child_process"
import { syncBuiltinESMExports } from "node:module"
const spawn = childProcess.spawn
childProcess.spawn = function (command, args, options) {
  const monitored =
    typeof command === "string" &&
    (command.endsWith("/node_modules/.bin/vite") || command === "google-chrome")
  const capture = monitored && options?.stdio === "ignore"
  const child = spawn.call(
    this,
    command,
    args,
    capture ? { ...options, stdio: ["ignore", "pipe", "pipe"] } : options,
  )
  if (capture) {
    child.stdout.on("data", (bytes) => process.stdout.write(bytes))
    child.stderr.on("data", (bytes) => process.stderr.write(bytes))
  }
  if (monitored)
    child.on("error", (error) =>
      process.stderr.write(
        JSON.stringify({
          kind: "lab-child-spawn-error",
          code: error.code ?? "unknown",
        }) + "\n",
      ),
    )
  return child
}
syncBuiltinESMExports()
