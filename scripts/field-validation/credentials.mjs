import { readFileSync, lstatSync } from "node:fs"
export function evaluationKey(provider) {
  if (!process.env.CAREEROS_EVAL_KEY_FILE)
    return process.env[
      provider === "jev" ? "TYPESAFE_API_KEY" : "OPENAI_API_KEY"
    ]
  const path = process.env.CAREEROS_EVAL_KEY_FILE
  const stat = lstatSync(path)
  if (
    !stat.isFile() ||
    (stat.mode & 0o077) !== 0 ||
    (process.getuid && stat.uid !== process.getuid())
  )
    throw new Error(
      "Credential file must be owned by you, private (0600), and a regular file",
    )
  let key
  try {
    key = JSON.parse(readFileSync(path, "utf8"))[provider]
  } catch {
    throw new Error(
      "Cannot read credential JSON; correct the private file locally",
    )
  }
  if (typeof key !== "string" || !key.trim())
    throw new Error("Provider key missing from private file")
  return key
}
