import { readFileSync, writeFileSync, existsSync, renameSync } from "node:fs"
import { cases } from "./evaluate.mjs"
import { collectVariant, scoreExperiment, hash } from "./experiment.mjs"
import { evaluationKey } from "./credentials.mjs"

const [
  provider,
  variant,
  split,
  destination,
  low = "0.2",
  high = "0.8",
  order = "normal",
  phase = "followup",
] = process.argv.slice(2)
if (
  !["openai", "jev"].includes(provider) ||
  !["explicit", "atomic"].includes(variant) ||
  ![
    "development",
    "calibration",
    "heldout",
    "smoke",
    "repeat",
    "confirmatory",
  ].includes(split) ||
  !destination ||
  !["normal", "reverse"].includes(order)
)
  throw new Error(
    "Explicit provider, variant, split, destination and option order required",
  )
if (existsSync(destination) || existsSync(destination + ".inprogress"))
  throw new Error("Preserve prior runs; choose an unused destination")
if (
  !["followup", "confirmatory"].includes(phase) ||
  (phase === "confirmatory") !== (split === "confirmatory")
)
  throw new Error("Select the matching frozen experiment phase")
const followup = JSON.parse(
  readFileSync(
    new URL(
      `../../docs/evaluations/field-validation-${phase}-cases.json`,
      import.meta.url,
    ),
    "utf8",
  ),
)
let selected =
  split === "development"
    ? cases
    : split === "smoke"
      ? cases.filter((item) => ["en-fact", "pt-quoted-job"].includes(item.id))
      : split === "repeat"
        ? cases.filter((item) =>
            [
              "en-quoted-profile",
              "pt-quoted-job",
              "en-unusable-profile",
              "pt-applicant",
            ].includes(item.id),
          )
        : followup.filter((item) => item.split === split)
const key = evaluationKey(provider)
if (!key) throw new Error("Configure provider key privately")
const plan = JSON.parse(
  readFileSync(
    new URL(
      `../../docs/evaluations/field-validation-${phase}-plan.json`,
      import.meta.url,
    ),
    "utf8",
  ),
)
if (
  plan.corpusHash !== hash(followup) ||
  plan.runnerHash !==
    hash(readFileSync(new URL("./experiment.mjs", import.meta.url), "utf8"))
)
  throw new Error(
    "Frozen corpus or runner changed; record a new experiment plan before any calls",
  )
const checkpoint = []
const run = await collectVariant(provider, variant, selected, key, fetch, {
  band: [Number(low), Number(high)],
  reverse: order === "reverse",
  onRecord(record) {
    checkpoint.push(record)
    writeFileSync(
      destination + ".inprogress",
      JSON.stringify(
        {
          status: "in_progress",
          provider,
          variant,
          split,
          planHash: hash(plan),
          records: checkpoint,
        },
        null,
        2,
      ) + "\n",
      { mode: 0o600 },
    )
  },
})
run.split = split
run.planHash = hash(plan)
writeFileSync(
  destination + ".inprogress",
  JSON.stringify(run, null, 2) + "\n",
  { mode: 0o600 },
)
renameSync(destination + ".inprogress", destination)
const report = scoreExperiment(run, selected)
const { rows, ...summary } = report
process.stdout.write(
  JSON.stringify({ file: destination, split, ...summary }) + "\n",
)

if (report.failures) process.exitCode = 1
