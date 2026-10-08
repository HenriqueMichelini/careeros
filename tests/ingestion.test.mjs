import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"

const compiled = ts.transpileModule(
  readFileSync(new URL("../src/lib/ingestion.ts", import.meta.url), "utf8"),
  {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
      verbatimModuleSyntax: false,
    },
  },
).outputText.replace(/from ['"]\.\/fieldDecision['"]/g, `from "./careeros-field-${process.pid}.mjs"`).replace(/from ['"]\.\/profile['"]/g, `from "./careeros-profile-${process.pid}.mjs"`)
writeFileSync(join(tmpdir(), `careeros-field-${process.pid}.mjs`), ts.transpileModule(
 readFileSync(new URL("../src/lib/fieldDecision.ts", import.meta.url), "utf8"),
 {compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}},
).outputText)
const profileCompiled = ts.transpileModule(
  readFileSync(new URL("../src/lib/profile.ts", import.meta.url), "utf8"),
  { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } },
).outputText
writeFileSync(join(tmpdir(), `careeros-profile-${process.pid}.mjs`), profileCompiled)
const output = join(tmpdir(), "careeros-ingestion-" + process.pid + ".mjs")
writeFileSync(output, compiled)
const { validateIngestionResult, applyIngestion, beforeValue, afterValue, previewValues, validProfile, ingestProfile } =
  await import(output)

const profile = () => ({
  careerGoals: "",
  skills: "React",
  competencies: "",
  experience: [
    {
      id: "e1",
      company: "Acme",
      title: "Engineer",
      startDate: "",
      endDate: "",
      current: false,
      location: "",
      description: "",
      responsibilities: "",
      achievements: "Built search",
    },
  ],
  tools: "",
  projects: [
    {
      id: "p1",
      name: "Dashboard",
      description: "",
      technologies: "",
      url: "",
      highlights: "",
    },
  ],
  employmentStatus: "",
  currentSalary: "",
  desiredSalary: "",
  additionalInfo: "",
})
const op = (overrides = {}) => ({
  claimId: "c1",
  target: "skills",
  entryId: "",
  field: "skills",
  action: "add",
  value: "TypeScript",
  finding: "addition",
  approved: true,
  ...overrides,
})
const claim = (overrides = {}) => ({
  id: "c1",
  source: "TypeScript",
  text: "TypeScript",
  targets: ["skills"],
  question: "",
  ...overrides,
})
const review = (data) => ({ unverifiedClaimCount: 0, unresolvedClaimIds: [], unplacedOperationCount: 0, ...data })

test("expanded Profile validates and Quick Add preserves structured qualifications", () => {
  const expanded = {
    ...profile(), fullName: "Ada", email: "", phone: "", location: "", professionalLinks: "",
    education: [{ id: "ed1", degree: "BSc", institution: "Example University", location: "", graduationDate: "2018", details: "" }],
    certifications: [], languages: [{ id: "lang1", name: "English", proficiency: "Fluent" }],
  }
  assert.equal(validProfile(expanded), true)
  const next = applyIngestion(expanded, JSON.stringify(expanded), [op()])
  assert.deepEqual(next.education, expanded.education)
  assert.deepEqual(next.languages, expanded.languages)
  assert.equal(next.skills, "React\nTypeScript")
  assert.equal(validProfile({ ...expanded, languages: [{ ...expanded.languages[0], id: "ed1" }] }), false)
})

test("maps one claim to linked sections and validates source and target", () => {
  const result = validateIngestionResult(
    review({
      claims: [claim({ targets: ["skills", "experience"] })],
      operations: [
        {
          claimId: "c1",
          target: "skills",
          entryId: "",
          field: "skills",
          action: "add",
          value: "TypeScript",
          finding: "addition",
        },
        {
          claimId: "c1",
          target: "experience",
          entryId: "e1",
          field: "achievements",
          action: "add",
          value: "Used TypeScript",
          finding: "in_place",
        },
      ],
    }),
    "TypeScript",
    profile(),
  )
  assert.equal(result.operations.length, 2)
  assert.ok(result.operations.every((o) => o.approved === false))
  assert.throws(() =>
    validateIngestionResult(
      review({ claims: [claim({ source: "invented" })], operations: [] }),
      "TypeScript",
      profile(),
    ),
  )
  assert.throws(() =>
    validateIngestionResult(
      review({
        claims: [claim()],
        operations: [
          { ...op(), target: "desiredSalary", field: "desiredSalary" },
        ],
      }),
      "TypeScript",
      profile(),
    ),
  )
})

test("partial review identifies withheld claims and cannot carry their operations", () => {
  const { approved: _approved, ...rawOp } = op()
  const partial = review({
    claims: [claim(), claim({ id: "c2", text: "Project", source: "Project", targets: ["projects"] })],
    operations: [rawOp],
    unverifiedClaimCount: 1,
    unresolvedClaimIds: ["c2"],
  })
  const result = validateIngestionResult(partial, "TypeScript Project", profile())
  assert.equal(result.unverifiedClaimCount, 1)
  assert.deepEqual(result.unresolvedClaimIds, ["c2"])
  assert.throws(() => validateIngestionResult({ ...partial, operations: [{ ...rawOp, claimId: "c2", target: "projects", entryId: "p1", field: "name" }] }, "TypeScript Project", profile()))
})

test("applies edited in-place facts while preserving IDs and unrelated fields", () => {
  const before = profile()
  const operations = [
    op({ value: "TypeScript at Acme" }),
    op({
      target: "experience",
      entryId: "e1",
      field: "achievements",
      value: "Improved search",
      finding: "in_place",
    }),
    op({
      target: "projects",
      entryId: "p1",
      field: "highlights",
      value: "Rejected",
      approved: false,
    }),
  ]
  const next = applyIngestion(before, JSON.stringify(before), operations)
  assert.equal(next.skills, "React\nTypeScript at Acme")
  assert.equal(next.experience[0].achievements, "Built search\nImproved search")
  assert.equal(next.experience[0].id, "e1")
  assert.equal(next.projects[0].highlights, "")
  assert.equal(before.skills, "React")
  assert.equal(afterValue(before, operations[0]), next.skills)
  assert.equal(beforeValue(before, operations[0]), "React")
})

test("repeat adds, explicit removal, and new IDs", () => {
  const before = profile()
  assert.equal(
    applyIngestion(before, JSON.stringify(before), [op({ value: "React" })])
      .skills,
    "React",
  )
  assert.equal(
    applyIngestion({ ...before, skills: "Built payment APIs." }, JSON.stringify({ ...before, skills: "Built payment APIs." }), [op({ value: "Built payment APIs" })]).skills,
    "Built payment APIs.",
  )
  assert.equal(
    applyIngestion({ ...before, skills: "• Built payment APIs" }, JSON.stringify({ ...before, skills: "• Built payment APIs" }), [op({ value: "Built payment APIs." })]).skills,
    "• Built payment APIs",
  )
  assert.equal(
    applyIngestion({ ...before, skills: "Built payment APIs" }, JSON.stringify({ ...before, skills: "Built payment APIs" }), [op({ value: "Built payment API" })]).skills,
    "Built payment APIs\nBuilt payment API",
  )
  assert.equal(
    applyIngestion(before, JSON.stringify(before), [
      op({ action: "remove", value: "" }),
    ]).skills,
    "",
  )
  const operations = [
    op({
      target: "experience",
      entryId: "new:c1",
      field: "company",
      value: "Beta",
    }),
    op({
      target: "experience",
      entryId: "new:c1",
      field: "title",
      value: "Lead",
    }),
  ]
  const next = applyIngestion(before, JSON.stringify(before), operations)
  assert.equal(next.experience.length, 2)
  assert.notEqual(next.experience[1].id, "e1")
  assert.equal(next.experience[1].company, "Beta")
  assert.throws(
    () =>
      applyIngestion(before, JSON.stringify(before), operations.slice(0, 1)),
    /incomplete/,
  )
})

test("related claims create one detailed experience and one project", () => {
  const before = profile()
  const operations = [
    op({ target: "experience", entryId: "new:c1", field: "company", value: "Aster Labs" }),
    op({ target: "experience", entryId: "new:c1", field: "title", value: "Engineer" }),
    op({ claimId: "c2", target: "experience", entryId: "new:c1", field: "description", value: "Backend engineer for payment systems." }),
    op({ claimId: "c3", target: "experience", entryId: "new:c1", field: "responsibilities", value: "Built payment APIs." }),
    op({ claimId: "c4", target: "experience", entryId: "new:c1", field: "achievements", value: "Cut validation time to 20 seconds." }),
    op({ claimId: "c5", target: "projects", entryId: "new:c5", field: "name", value: "Harbor" }),
    op({ claimId: "c6", target: "projects", entryId: "new:c5", field: "description", value: "Inventory project for small shops." }),
    op({ claimId: "c7", target: "projects", entryId: "new:c5", field: "highlights", value: "Added audit trails with Go." }),
  ]
  const next = applyIngestion(before, JSON.stringify(before), operations)
  assert.equal(next.experience.length, 2)
  assert.equal(next.experience[1].description, "Backend engineer for payment systems.")
  assert.equal(next.experience[1].responsibilities, "Built payment APIs.")
  assert.equal(next.experience[1].achievements, "Cut validation time to 20 seconds.")
  assert.equal(next.projects.length, 2)
  assert.equal(next.projects[1].highlights, "Added audit trails with Go.")
})

test("new entry references require a reviewed anchor and identity fields", () => {
  const claims = [claim({ targets: ["experience"] }), claim({ id: "c2", source: "Built APIs", text: "Built APIs", targets: ["experience"] })]
  const { approved: _approved, ...company } = op({ target: "experience", entryId: "new:c1", field: "company", value: "Aster Labs" })
  const raw = review({ claims, operations: [company, { ...company, claimId: "c2", field: "responsibilities", value: "Built APIs" }] })
  assert.throws(() => validateIngestionResult(raw, "TypeScript Built APIs", profile()))
})

test("two approved new entries cannot save the same project twice", () => {
  const before = profile()
  const operations = [
    op({ target: "projects", entryId: "new:c1", field: "name", value: "Harbor" }),
    op({ claimId: "c2", target: "projects", entryId: "new:c2", field: "name", value: "Harbor" }),
  ]
  assert.throws(() => applyIngestion(before, JSON.stringify(before), operations), /incomplete/)
  assert.equal(before.projects.length, 1)
})

test("reviewed updates can enrich sparse saved entries without replacing unrelated fields", () => {
  const before = profile()
  before.experience[0].description = "Software engineer."
  before.projects[0].description = "Inventory project."
  const next = applyIngestion(before, JSON.stringify(before), [
    op({ target: "experience", entryId: before.experience[0].id, field: "description", action: "update", value: "Backend engineer developing payment APIs and integration tests." }),
    op({ target: "experience", entryId: before.experience[0].id, field: "responsibilities", value: "Built transactional APIs." }),
    op({ target: "projects", entryId: before.projects[0].id, field: "description", action: "update", value: "Inventory platform for small shops." }),
    op({ target: "projects", entryId: before.projects[0].id, field: "highlights", value: "Added audit trails with Go." }),
  ])
  assert.equal(next.experience[0].description, "Backend engineer developing payment APIs and integration tests.")
  assert.equal(next.experience[0].responsibilities, "Built transactional APIs.")
  assert.equal(next.experience[0].id, before.experience[0].id)
  assert.equal(next.projects[0].description, "Inventory platform for small shops.")
  assert.equal(next.projects[0].highlights, "Added audit trails with Go.")
  assert.equal(next.projects[0].id, before.projects[0].id)
  assert.equal(before.experience[0].description, "Software engineer.")
  assert.equal(before.projects[0].description, "Inventory project.")
})

test("stale or invalid patches do not mutate the Profile", () => {
  const before = profile()
  assert.throws(
    () =>
      applyIngestion({ ...before, skills: "Changed" }, JSON.stringify(before), [
        op(),
      ]),
    /stale/,
  )
  assert.throws(() =>
    applyIngestion(before, JSON.stringify(before), [
      op({
        target: "employmentStatus",
        field: "employmentStatus",
        value: "unknown",
      }),
    ]),
  )
  assert.equal(before.skills, "React")
})

test("preview includes preceding approved changes and matches boolean apply", () => {
  const before=profile()
  const operations=[op({value:"TypeScript"}),op({value:"Go"})]
  assert.deepEqual(previewValues(before,operations,1),{before:"React\nTypeScript",after:"React\nTypeScript\nGo"})
  assert.equal(applyIngestion(before,JSON.stringify(before),operations).skills,previewValues(before,operations,1).after)
  const current=op({target:"experience",entryId:"e1",field:"current",value:"true"})
  assert.deepEqual(previewValues(before,[current],0),{before:"false",after:"true"})
  assert.equal(applyIngestion(before,JSON.stringify(before),[current]).experience[0].current,true)
  assert.throws(() => applyIngestion(before,JSON.stringify(before),[{...current,value:"maybe"}]))
  const removal={...current,action:"remove",value:""}
  assert.equal(previewValues(before,[removal],0).after,"false")
})

test("legacy saved employment status is preserved by Quick Add", () => {
  const before={...profile(),employmentStatus:"Employed — Full-time"}
  assert.equal(validProfile(before),true)
  assert.equal(applyIngestion(before,JSON.stringify(before),[op()]).employmentStatus,"Employed — Full-time")
})

test("Quick Add supplies the complete Profile for server-side minimal comparison projection", async () => {
  const before = {
    ...profile(), fullName: "Ada Lovelace", email: "ada@example.test",
    phone: "+1 555 0100", location: "London", professionalLinks: "example.test/ada",
  }
  assert.equal(validProfile(before), true)
  const updated = applyIngestion(before, JSON.stringify(before), [op()])
  assert.equal(updated.email, before.email)
  assert.equal(updated.professionalLinks, before.professionalLinks)
  const originalFetch = globalThis.fetch
  let body
  globalThis.fetch = async (_url, options) => {
    body = JSON.parse(options.body)
    return { ok: true, json: async () => ({decision:{version:1,field:"professional_information",outcome:{kind:"accept"}},...review({ claims: [], operations: [] })}) }
  }
  try {
    await ingestProfile("TypeScript", before, "test-key")
    assert.deepEqual(body.profile, { ...before, education: [], certifications: [], languages: [] })
  } finally {
    globalThis.fetch = originalFetch
  }
})

test("structured destinations are reviewed, edited and applied atomically with stable identities", () => {
  const before = { ...profile(), fullName: "Ada", email: "old@example.test", phone: "", location: "", professionalLinks: "",
    education: [{id:"school",degree:"BSc",institution:"North",location:"",graduationDate:"2020",details:"Existing detail"}],
    certifications: [], languages: [{id:"english",name:"English",proficiency:"Intermediate"}] }
  const claims = [claim({targets:["email","education","certifications","languages"]})]
  const operations = [
    op({target:"email",field:"email",action:"update",value:"new@example.test"}),
    op({target:"education",entryId:"school",field:"graduationDate",action:"update",value:"2021"}),
    op({target:"certifications",entryId:"new:c1",field:"name",value:"Cloud Certificate"}),
    op({target:"certifications",entryId:"new:c1",field:"issuer",value:"Cloud Guild"}),
    op({target:"languages",entryId:"english",field:"proficiency",action:"update",value:"Fluent"}),
  ]
  const raw = review({claims, operations: operations.map(({approved,...operation})=>operation)})
  const result = validateIngestionResult(raw,"TypeScript",before)
  assert.ok(result.operations.every(operation=>!operation.approved))
  assert.deepEqual(applyIngestion(before,JSON.stringify(before),result.operations),before)
  const approved = result.operations.map(operation=>({...operation,approved:true}))
  approved[4].value="Advanced"
  const next=applyIngestion(before,JSON.stringify(before),approved)
  assert.equal(next.email,"new@example.test")
  assert.equal(next.education[0].graduationDate,"2021")
  assert.equal(next.education[0].details,"Existing detail")
  assert.equal(next.education[0].id,"school")
  assert.equal(next.languages[0].proficiency,"Advanced")
  assert.equal(next.languages[0].id,"english")
  assert.equal(next.certifications[0].name,"Cloud Certificate")
  assert.equal(before.email,"old@example.test")
  assert.throws(()=>applyIngestion(next,JSON.stringify(next),approved),/incomplete/)
  assert.throws(()=>applyIngestion(before,JSON.stringify(before),[...approved,op({target:"languages",entryId:"new:c1",field:"proficiency",value:"Native"})]),/incomplete/)
  assert.equal(before.certifications.length,0)
  assert.throws(()=>validateIngestionResult({...raw, operations:[{...raw.operations[0],finding:"conflict"}]},"TypeScript",before),/invalid_output/)
})

test("new education and languages require reviewed identities and repeat pastes do not duplicate entries", () => {
  const before={...profile(),fullName:"",email:"",phone:"",location:"",professionalLinks:"",education:[],certifications:[],languages:[]}
  const operations=[op({target:"education",entryId:"new:c1",field:"degree",value:"MSc"}),op({target:"education",entryId:"new:c1",field:"institution",value:"South"}),op({target:"languages",entryId:"new:c1",field:"name",value:"Portuguese"}),op({target:"languages",entryId:"new:c1",field:"proficiency",value:"Native"})]
  const next=applyIngestion(before,JSON.stringify(before),operations)
  assert.equal(next.education[0].degree,"MSc")
  assert.equal(next.languages[0].name,"Portuguese")
  assert.throws(()=>applyIngestion(next,JSON.stringify(next),operations),/incomplete/)
  assert.equal(next.education.length,1)
})

test("client accepts exactly 30,000 UTF-8 bytes and rejects oversized mixed Unicode without a request", async () => {
  const originalFetch=globalThis.fetch
  let calls=0
  globalThis.fetch=async()=>{calls++;return {ok:true,json:async()=>({decision:{version:1,field:"professional_information",outcome:{kind:"accept"}},...review({claims:[],operations:[]})})}}
  try {
    const input="ação🙂 \n".repeat(2500) // 12 bytes per line
    assert.equal(new TextEncoder().encode(input).length,30000)
    await ingestProfile(input,profile(),"sk-synthetic")
    assert.equal(calls,1)
    await assert.rejects(ingestProfile(input+"x",profile(),"sk-synthetic"),/input/)
    assert.equal(calls,1)
  } finally {globalThis.fetch=originalFetch}
})

test("professional link additions preserve existing and multiple approved links", () => {
  const before={...profile(),fullName:"",email:"",phone:"",location:"",professionalLinks:"https://linkedin.test/ada",education:[],certifications:[],languages:[]}
  const ops=[op({target:"professionalLinks",field:"professionalLinks",value:"https://github.test/ada"}),op({target:"professionalLinks",field:"professionalLinks",value:"https://portfolio.test/ada"}),op({target:"professionalLinks",field:"professionalLinks",value:"https://github.test/ada"})]
  assert.equal(applyIngestion(before,JSON.stringify(before),ops).professionalLinks,"https://linkedin.test/ada\nhttps://github.test/ada\nhttps://portfolio.test/ada")
  assert.equal(applyIngestion(before,JSON.stringify(before),[{...ops[0],action:"update"}]).professionalLinks,"https://github.test/ada")
})

test("same-name certifications from distinct issuers are preserved and exact repeats are rejected", () => {
  const before={...profile(),fullName:"",email:"",phone:"",location:"",professionalLinks:"",education:[],certifications:[{id:"a",name:"Cloud Fundamentals",issuer:"Issuer A",date:"",credentialId:"",url:""}],languages:[]}
  const ops=[op({target:"certifications",entryId:"new:c1",field:"name",value:"Cloud Fundamentals"}),op({target:"certifications",entryId:"new:c1",field:"issuer",value:"Issuer B"})]
  const next=applyIngestion(before,JSON.stringify(before),ops)
  assert.equal(next.certifications.length,2)
  assert.equal(next.certifications[0].id,"a")
  assert.equal(next.certifications[1].issuer,"Issuer B")
  assert.throws(()=>applyIngestion(next,JSON.stringify(next),ops),/incomplete/)
})

test("separate stints at the same employer and title apply without losing the batch", () => {
  const before = profile()
  before.experience[0].startDate = "2018-01"
  before.experience[0].endDate = "2020-12"
  const snapshot = JSON.stringify(before)
  const operations = [
    op(),
    ...Object.entries({ company: "Acme", title: "Engineer", startDate: "2024-01", endDate: "2025-12" })
      .map(([field, value]) => op({ target: "experience", entryId: "new:c1", field, value })),
  ]
  const next = applyIngestion(before, snapshot, operations)
  assert.equal(next.skills, "React\nTypeScript")
  assert.equal(next.experience.length, 2)
  assert.deepEqual(next.experience[0], before.experience[0])
  assert.notEqual(next.experience[1].id, before.experience[0].id)
  assert.equal(next.experience[1].startDate, "2024-01")
  assert.equal(next.experience[1].endDate, "2025-12")
  assert.equal(JSON.stringify(before), snapshot)
  assert.throws(() => applyIngestion(next, JSON.stringify(next), operations), /incomplete/)
  assert.equal(next.experience.length, 2)
})

test("Experience duplicate protection distinguishes dates and still rejects matching or undated stints atomically", () => {
  for (const [startDate, endDate] of [["2018-01", "2020-12"], ["", ""]]) {
    const before = profile()
    Object.assign(before.experience[0], { startDate, endDate })
    const snapshot = JSON.stringify(before)
    const operations = [
      op(),
      ...Object.entries({ company: " ACME ", title: "engineer", startDate, endDate })
        .map(([field, value]) => op({ target: "experience", entryId: "new:c1", field, value })),
    ]
    assert.throws(() => applyIngestion(before, snapshot, operations), /incomplete/)
    assert.equal(JSON.stringify(before), snapshot)
    for (const [field, value] of [["startDate", "2024-01"], ["endDate", "2025-12"]]) {
      const changedPeriod = operations.map(item => item.field === field ? { ...item, value } : item)
      assert.equal(applyIngestion(before, snapshot, changedPeriod).experience.length, 2)
    }
  }
})

test("original source references use UTF-8 byte ranges and reject invalid evidence", () => {
  const input = "é\r\nFirst TypeScript\r\nSecond TypeScript"
  const sourceReference = {version:1,sourceId:'a'.repeat(64),preparationVersion:'structure-v1',segmentId:'b'.repeat(64),occurrenceId:'c'.repeat(64),originalStart:29,originalEnd:39}
  const raw = review({claims:[claim({sourceReference})],operations:[]})
  assert.equal(validateIngestionResult(raw,input,profile()).claims[0].sourceReference.originalStart,29)
  for (const patch of [{version:2},{originalStart:1},{originalEnd:999},{originalStart:8},{segmentId:'invalid'},{extra:true}]) {
    assert.throws(()=>validateIngestionResult({...raw,claims:[claim({sourceReference:{...sourceReference,...patch}})]},input,profile()))
  }
})

test("ingestion binds traceable evidence to the complete raw submission", async () => {
  const {createHash} = await import('node:crypto')
  const input = 'I  use Java\r\n<&>\u2028'
  const sourceId = createHash('sha256').update('["normalization-v1","structure-v1","professional_information","I  use Java\\r\\n\\u003c\\u0026\\u003e\\u2028"]').digest('hex')
  const sourceReference = {version:1,sourceId,preparationVersion:'structure-v1',segmentId:'b'.repeat(64),occurrenceId:'c'.repeat(64),originalStart:0,originalEnd:11}
  const payload = {decision:{version:1,field:'professional_information',outcome:{kind:'accept'}},...review({claims:[claim({source:'I  use Java',text:'Java',sourceReference})],operations:[]})}
  const originalFetch = globalThis.fetch
  try {
    globalThis.fetch = async (_url, options) => {
      assert.equal(JSON.parse(options.body).input,input)
      return new Response(JSON.stringify(payload),{status:200})
    }
    assert.equal((await ingestProfile(input,profile(),'sk-test')).claims[0].source,input.slice(0,11))
    // The excerpt is still exact, but a different complete paste invalidates its identity.
    globalThis.fetch = async()=>new Response(JSON.stringify(payload),{status:200})
    await assert.rejects(ingestProfile(input+' changed',profile(),'sk-test'),{code:'invalid_output'})
  } finally { globalThis.fetch = originalFetch }
})
