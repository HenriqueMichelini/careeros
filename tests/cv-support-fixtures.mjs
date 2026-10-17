// ADR 0003 / issue #128 coordinated activation fixtures.
//
// After activation the production client opts into the v3 statement-support
// contract (`statementSupport: true`) and rejects a response without the
// requested assessment. Controlled fixtures must therefore answer in the same
// v3 shape the production handler emits: the generated logical content plus one
// judgment per review unit. These helpers keep the scripted judgments explicit
// and are shared by the unit and browser checks. They establish controlled
// enforcement only, never live provider quality.

export const CV_SUPPORT_POLICY = "cv-statement-support-v1"

// Empty per-section entry shapes shared by the tests that materialize a
// ProfessionalRepository from handler facts. Keeping one copy prevents the
// default field sets from drifting between the join and browser checks.
export const repositoryDefaults = {
  experience: {
    title: "",
    company: "",
    startDate: "",
    endDate: "",
    current: false,
    location: "",
    description: "",
    responsibilities: "",
    achievements: "",
  },
  projects: {
    name: "",
    description: "",
    technologies: "",
    url: "",
    highlights: "",
  },
  education: {
    degree: "",
    institution: "",
    location: "",
    graduationDate: "",
    details: "",
  },
  certifications: { name: "", issuer: "", date: "", credentialId: "", url: "" },
  languages: { name: "", proficiency: "" },
}

// Server-assigned review units: one summary item per sentence, then one rewrite
// per cited source id in ascending order. `summary[].sourceId` is normalized to
// the current `sourceIds` citation form.
function v3Units(core) {
  const wording = core.wording ?? {}
  return [
    ...core.summary.map((item, index) => ({
      id: `summary:${index}`,
      text: item.text,
      sourceIds: item.sourceIds ?? [item.sourceId],
    })),
    ...Object.keys(wording)
      .sort()
      .map((id) => ({
        id: `wording:${id}`,
        text: wording[id],
        sourceIds: [id],
      })),
  ]
}

// Build a complete v3 response. `statuses` maps a review-unit id to a
// `supported`/`uncertain`/`unsupported` judgment; unspecified units default to
// `supported` so a benign fixture stays acceptable.
export function v3Response(core, statuses = {}) {
  const wording = core.wording ?? {}
  return {
    schemaVersion: 3,
    summary: core.summary,
    selected: core.selected,
    wording,
    support: {
      policyVersion: CV_SUPPORT_POLICY,
      assessment: "complete",
      statements: v3Units(core).map((unit) => ({
        ...unit,
        status: statuses[unit.id] ?? "supported",
        reason: "controlled fixture judgment",
      })),
    },
  }
}

// The same wrapper as an expression a browser page can evaluate. Services the
// browser check, which injects controlled `fetch` mocks as strings.
export const v3ResponseExpression = `((core,statuses)=>{const wording=core.wording||{};const statements=core.summary.map((item,index)=>({id:'summary:'+index,text:item.text,sourceIds:item.sourceIds||[item.sourceId],status:(statuses||{})['summary:'+index]||'supported',reason:'controlled fixture judgment'})).concat(Object.keys(wording).sort().map(id=>({id:'wording:'+id,text:wording[id],sourceIds:[id],status:(statuses||{})['wording:'+id]||'supported',reason:'controlled fixture judgment'})));return JSON.stringify({schemaVersion:3,summary:core.summary,selected:core.selected,wording,support:{policyVersion:'cv-statement-support-v1',assessment:'complete',statements}})})`
