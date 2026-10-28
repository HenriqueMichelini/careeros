// The existing unit harnesses emit individual TS modules into temporary dirs.
// Keep the new, dependency-free request helper resolvable in those harnesses.
import { readFileSync } from "node:fs"
import ts from "typescript"
export const workflowModuleURL =
  "data:text/javascript;base64," +
  Buffer.from(
    ts.transpileModule(
      readFileSync(
        new URL("../src/lib/workflowRequest.ts", import.meta.url),
        "utf8",
      ),
      {
        compilerOptions: {
          module: ts.ModuleKind.ESNext,
          target: ts.ScriptTarget.ES2022,
        },
      },
    ).outputText,
  ).toString("base64")
