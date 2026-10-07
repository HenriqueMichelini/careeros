import { readFileSync } from "node:fs"
import ts from "typescript"
const source = readFileSync(
  new URL("../../src/lib/fieldDecision.ts", import.meta.url),
  "utf8",
)
const compiled = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText
export const { decideField, parseFieldSignals } = await import(
  `data:text/javascript;base64,${Buffer.from(compiled).toString("base64")}`
)
