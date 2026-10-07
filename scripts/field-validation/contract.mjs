import { readFileSync } from "node:fs"
import ts from "typescript"
async function sourceModule(path) {
  const source = readFileSync(new URL(path, import.meta.url), "utf8")
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText
  return import(
    `data:text/javascript;base64,${Buffer.from(compiled).toString("base64")}`
  )
}
export const { decideField, parseFieldSignals } = await sourceModule(
  "../../src/lib/fieldDecision.ts",
)
export const { careerProfile, cvQualifications } = await sourceModule(
  "../../src/lib/profile.ts",
)
