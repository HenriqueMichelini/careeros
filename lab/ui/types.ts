export interface FixtureCase {
  id: string
  workflow: string
  language: string
  split: string
  categories?: string[]
  requiredSteps: number
  maximumCalls: number
  exposureGroup: string
}
export interface Suite {
  id: string
  version: string
  modes: string[]
  configurations: string[]
  cases: FixtureCase[]
  requiredTools: string[]
  assertionSemantics: string
  fixtureHash: string
}
export interface Caps {
  maxCases: number
  maxHttpAttempts: number
  maxProviderCalls: number
  maxArtifactBytes: number
  deadlineMs: number
}
export interface Plan {
  id: string
  hash: string
  selection: {
    suite: string
    mode: string
    configuration: string
    caseIds: string[]
  }
  caseOrder: string[]
  requiredProcessAttempts?: number
  requiredHttpAttempts: number
  maximumPlannedProviderCalls: number
  caps: Caps
  provenance: Record<string, unknown>
  runtimeVersion: string
  inputHashes: Record<string, string>
}
export interface Metric {
  id: string
  measure: string
  caseId: string
  workflow: string
  language: string
  field: string
  value: number | null
  numerator: number | null
  denominator: number | null
  eligible: number
  evaluated: number
  missing: number
  unavailableReason?: string
  units: string
  estimator: string
  scope?: string
  mode: string
}
export interface Definition {
  id: string
  name: string
  definition: string
  source: string
  version: string
}
export interface Status {
  version: string
  storageReady: boolean
  usage: {
    bytes: number
    files: number
    softWarning: boolean
    softWarningBytes: number
  }
  suites: {
    id: string
    available: boolean
    reason?: string
  }[]
  processCapabilities?: {
    isolationAvailable: boolean
    runtimeVersions: Record<string, string>
    unavailableReasons: string[]
    selectors: {
      id: string
      requiredTools: string[]
      available: boolean
      unavailableReasons: string[]
    }[]
  }
  metricDefinitions: Definition[]
  build?: Record<string, unknown>
}
export interface CaseEvidence {
  identity: string
  id: string
  workflow: string
  language: string
  kind: string
  engineState: string
  assertionVerdict: string
  operationalOutcome: string
  unavailableReasons: string[]
  failureFamilies?: string[]
  children?: CaseEvidence[]
  source?: unknown
  expected?: unknown
  actual?: unknown
  review?: unknown
}
export interface View {
  startedAt?: string
  elapsedMs?: number
  id: string
  version: string
  engineState?: string
  assertionVerdict?: string
  durable: boolean
  quarantined?: boolean
  originMode?: string
  originalHash?: string
  originalVersion?: string
  selection?: Plan["selection"]
  progress?: {
    plannedProcesses?: number
    attemptedProcesses?: number
    finishedProcesses?: number
    unreachedProcesses?: number
    plannedCases: number
    startedCases: number
    finishedCases: number
    plannedHttpAttempts: number
    attemptedHttpAttempts: number
    finishedHttpAttempts: number
    unreachedHttpAttempts: number
    attemptedDispatches: number
  }
  context?: Record<string, unknown>
  provenance: Record<string, unknown>
  derivations?: unknown
  derivation?: unknown
  cases: CaseEvidence[]
  metrics: Metric[]
  unavailableReasons: string[]
  dispatches?: unknown[]
}
export interface Summary {
  id: string
  suite: string
  mode: string
  engineState: string
  assertionVerdict: string
  cases: number
  quarantined: boolean
  workflows?: string[]
  languages?: string[]
  categories?: string[]
  configuredModels?: string[]
  returnedModels?: string[]
  versions?: string[]
  configuration?: string
}
export interface Page {
  runs: Summary[]
  next: number
  problems: string[]
}
export interface EventPage {
  events: {
    sequence: number
    kind?: string
    stage?: string
    outcome?: string
    engineState?: string
  }[]
  next: number
  epoch: string
  gap: boolean
  kind: string
}
export interface Comparison {
  compatibility: {
    compatible: boolean
    reasons: string[]
    changedDimension: string
  }
  partial: boolean
  pooling: string
  deltas: {
    caseId: string
    language: string
    workflow: string
    kind: string
    curatedForbiddenAdditionsDelta: number | null
    curatedMissingLabelsDelta: number | null
    curatedRelationshipErrorsDelta: number | null
    operationalDisagreement: boolean
    assertionDisagreement: boolean
    unavailableReason?: string
  }[]
}
