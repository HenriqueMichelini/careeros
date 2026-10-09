import test from "node:test"
import assert from "node:assert/strict"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"
const temp = mkdtempSync(join(tmpdir(), "profile-v2-"))
writeFileSync(
  join(temp, "contract.mjs"),
  `export default ${readFileSync(new URL("../internal/profiledocument/contract.json", import.meta.url), "utf8")}`,
)
for (const name of [
  "profileDocument",
  "profileStorage",
  "sectionReview",
  "ingestion",
  "ingestionDocument",
  "profile",
  "fieldDecision",
]) {
  const source = readFileSync(
    new URL(`../src/lib/${name}.ts`, import.meta.url),
    "utf8",
  )
  writeFileSync(
    join(temp, `${name}.mjs`),
    ts
      .transpileModule(source, {
        compilerOptions: {
          module: ts.ModuleKind.ESNext,
          target: ts.ScriptTarget.ES2022,
        },
      })
      .outputText.replaceAll('"./profileDocument"', '"./profileDocument.mjs"')
      .replaceAll('"./ingestion"', '"./ingestion.mjs"')
      .replaceAll('"./profile"', '"./profile.mjs"')
      .replaceAll('"./fieldDecision"', '"./fieldDecision.mjs"')
      .replaceAll(
        '"../../internal/profiledocument/contract.json"',
        '"./contract.mjs"',
      ),
  )
}
const {
  migrateProfile,
  profileView,
  validateProfileDocument,
  replaceProfileView,
  editProfile,
} = await import(join(temp, "profileDocument.mjs"))
const { sectionReviewRequest, validateSectionProposal, applySectionProposal } =
  await import(join(temp, "sectionReview.mjs"))
const legacy = {
  skills: "JS? No Java; hope to learn Go",
  experience: [
    {
      id: "role",
      company: "A",
      title: "Dev",
      startDate: "about 3 years ago",
      endDate: "",
      current: true,
      location: "",
      description: "Maybe led team",
      responsibilities: "",
      achievements: "",
    },
  ],
  projects: [],
}
test("migration preserves wording, entry IDs, unknown fields and order without interpreting narrative", () => {
  const doc = migrateProfile(
    { ...legacy, custom: { retained: true } },
    "profile",
  )
  assert.equal(validateProfileDocument(doc), true)
  const view = profileView(doc)
  assert.equal(view.skills, legacy.skills)
  assert.deepEqual(view.experience, legacy.experience)
  assert.deepEqual(view.custom, { retained: true })
  const block = doc.facts.find((f) => f.field === "skills")
  assert.equal(block.kind, "legacy_block")
  assert.equal(block.origin.kind, "existing_profile")
  assert.equal(block.origin.original, "unknown")
  assert.equal(block.approval, "unreviewed")
  assert.equal(block.support, "unsupported")
  assert.equal(block.assertion, "unknown")
  assert.equal(block.normalization.canonical, null)
})
const {
  openProfile,
  saveProfile,
  PROFILE_KEY,
  LEGACY_KEY,
  ProfileStorageError,
} = await import(join(temp, "profileStorage.mjs"))
const storage = (initial = {}) => {
  const data = new Map(Object.entries(initial))
  return {
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => {
      data.set(key, value)
    },
    data,
  }
}
const lock = async (operation) => operation()
test("migration persists one authority, retains recovery, reloads and refuses stale writes", async () => {
  const raw = JSON.stringify(legacy)
  const s = storage({ [LEGACY_KEY]: raw })
  const doc = await openProfile(s, lock)
  assert.equal(s.getItem(LEGACY_KEY), raw)
  assert.deepEqual(await openProfile(s, lock), doc)
  const next = replaceProfileView(doc, { ...profileView(doc), skills: "Go" })
  await saveProfile(s, doc, next, lock)
  assert.equal(s.getItem(LEGACY_KEY), raw)
  await assert.rejects(
    saveProfile(s, doc, next, lock),
    (e) => e instanceof ProfileStorageError && e.code === "stale",
  )
  assert.equal(profileView(await openProfile(s, lock)).skills, "Go")
})
test("absent and empty fields remain safe; entry ordering and identities survive edits", async () => {
  for (const input of [{}, { skills: "", experience: [] }]) {
    const s = storage({ [LEGACY_KEY]: JSON.stringify(input) })
    const doc = await openProfile(s, lock)
    assert.equal(profileView(doc).skills, "")
    assert.deepEqual(profileView(doc).education, [])
  }
  const doc = migrateProfile(
    {
      ...legacy,
      languages: [
        { id: "b", name: "English", proficiency: "unknown" },
        { id: "a", name: "Português", proficiency: "" },
      ],
    },
    "p",
  )
  const next = replaceProfileView(doc, {
    ...profileView(doc),
    languages: profileView(doc).languages.toReversed(),
  })
  assert.deepEqual(
    profileView(next).languages.map((l) => l.id),
    ["a", "b"],
  )
  assert.deepEqual(
    next.entities.map((e) => e.id).sort(),
    doc.entities.map((e) => e.id).sort(),
  )
})
test("quota and malformed storage preserve the original and never install an authority", async () => {
  const raw = JSON.stringify(legacy)
  const s = storage({ [LEGACY_KEY]: raw })
  s.setItem = () => {
    throw new Error("quota")
  }
  await assert.rejects(openProfile(s, lock), { code: "storage" })
  assert.equal(s.getItem(LEGACY_KEY), raw)
  assert.equal(s.getItem(PROFILE_KEY), null)
  for (const malformed of [
    "{",
    "null",
    "[]",
    '{"skills":123}',
    '{"experience":[{"id":"same"},{"id":"same"}]}',
  ]) {
    const broken = storage({ [LEGACY_KEY]: malformed })
    await assert.rejects(openProfile(broken, lock), { code: "validation" })
    assert.equal(broken.getItem(LEGACY_KEY), malformed)
    assert.equal(broken.getItem(PROFILE_KEY), null)
  }
  const brokenV2 = storage({ [LEGACY_KEY]: raw, [PROFILE_KEY]: "{}" })
  await assert.rejects(openProfile(brokenV2, lock), { code: "validation" })
  assert.equal(brokenV2.getItem(PROFILE_KEY), "{}")
})
test("whole-field corrections retain fact identity, increment revision and invalidate support", () => {
  const doc = migrateProfile(legacy, "p")
  const fact = doc.facts.find((f) => f.field === "skills")
  fact.approval = "approved"
  fact.support = "supported"
  doc.evidence.push({
    id: "excerpt",
    revision: 1,
    excerpt: legacy.skills,
    origin: "user-submitted excerpt",
    approval: "approved",
  })
  doc.links.push({
    id: "support",
    kind: "supports",
    from: { profileId: "p", id: fact.id, revision: 1 },
    to: { profileId: "p", id: "excerpt", revision: 1 },
    state: "active",
  })
  assert.equal(validateProfileDocument(doc), true)
  const unchanged = replaceProfileView(doc, {
    ...profileView(doc),
    tools: "Docker",
  })
  assert.equal(
    unchanged.facts.find((f) => f.id === fact.id).support,
    "supported",
  )
  const changed = replaceProfileView(doc, { ...profileView(doc), skills: "Go" })
  const replacement = changed.facts.find((f) => f.id === fact.id)
  assert.equal(replacement.revision, 2)
  assert.equal(replacement.support, "invalidated")
  assert.deepEqual(replacement.origin, {
    kind: "manual_edit",
    original: "user",
  })
  assert.equal(changed.links[0].state, "invalidated")
  assert.equal(changed.evidence[0].excerpt, legacy.skills)
})
test("references, aliases and semantic qualifiers are validated conservatively", () => {
  const doc = migrateProfile(legacy, "p")
  for (const mutate of [
    (d) => {
      d.facts[0].owner.profileId = "elsewhere"
    },
    (d) => {
      d.facts[0].owner.revision = 999
    },
    (d) => {
      d.facts[0].normalization.canonical = "JavaScript"
    },
    (d) => {
      d.facts[0].assertion = "confident"
    },
    (d) => {
      d.facts[0].support = "supported"
    },
    (d) => {
      d.entities.push(structuredClone(d.entities[0]))
    },
  ]) {
    const invalid = structuredClone(doc)
    mutate(invalid)
    assert.equal(validateProfileDocument(invalid), false)
  }
})
test("failed saves retain the previous canonical document and permit an explicit retry", async () => {
  const s = storage({ [LEGACY_KEY]: JSON.stringify(legacy) })
  const doc = await openProfile(s, lock)
  const raw = s.getItem(PROFILE_KEY)
  const candidate = replaceProfileView(doc, {
    ...profileView(doc),
    skills: "Go",
  })
  const write = s.setItem
  s.setItem = () => {
    throw new Error("quota")
  }
  await assert.rejects(saveProfile(s, doc, candidate, lock), {
    code: "storage",
  })
  assert.equal(s.getItem(PROFILE_KEY), raw)
  s.setItem = write
  await saveProfile(s, doc, candidate, lock)
  assert.equal(profileView(await openProfile(s, lock)).skills, "Go")
})
test("a changed role context invalidates dependent evidence instead of silently dropping context", () => {
  const doc = migrateProfile(legacy, "p")
  const fact = doc.facts.find((f) => f.field === "skills")
  const role = doc.entities[0]
  fact.context = [{ profileId: "p", id: role.id, revision: role.revision }]
  fact.approval = "approved"
  fact.support = "supported"
  doc.evidence.push({
    id: "excerpt",
    revision: 1,
    excerpt: legacy.skills,
    origin: "submitted excerpt",
    approval: "approved",
  })
  doc.links.push({
    id: "support",
    kind: "supports",
    from: { profileId: "p", id: fact.id, revision: fact.revision },
    to: { profileId: "p", id: "excerpt", revision: 1 },
    state: "active",
  })
  const updated = replaceProfileView(doc, {
    ...profileView(doc),
    experience: [{ ...profileView(doc).experience[0], title: "Designer" }],
  })
  assert.equal(
    updated.facts.find((f) => f.id === fact.id).support,
    "invalidated",
  )
  assert.equal(updated.links[0].state, "invalidated")
})
test("shared Go and TypeScript contract corpus rejects malformed and ungrounded documents", () => {
  const cases = JSON.parse(
    readFileSync(
      new URL("../internal/profiledocument/fixtures.json", import.meta.url),
      "utf8",
    ),
  )
  for (const fixture of cases)
    assert.equal(
      validateProfileDocument(fixture.document),
      fixture.valid,
      fixture.name,
    )
})
test("concurrent migrations agree on one authority and competing revisions cannot both save", async () => {
  let tail = Promise.resolve()
  const serialized = (operation) => {
    const result = tail.then(operation)
    tail = result.catch(() => {})
    return result
  }
  const s = storage({ [LEGACY_KEY]: JSON.stringify(legacy) })
  const [a, b] = await Promise.all([
    openProfile(s, serialized),
    openProfile(s, serialized),
  ])
  assert.deepEqual(a, b)
  const changes = ["TypeScript", "Python"].map((skills) =>
    saveProfile(
      s,
      a,
      replaceProfileView(a, { ...profileView(a), skills }),
      serialized,
    ),
  )
  const results = await Promise.allSettled(changes)
  assert.equal(
    results.filter((result) => result.status === "fulfilled").length,
    1,
  )
  assert.equal(
    results.find((result) => result.status === "rejected").reason.code,
    "stale",
  )
})
test("removal keeps approved excerpts and invalidates historical links to removed facts", () => {
  const doc = migrateProfile(legacy, "p")
  const f = doc.facts.find((f) => f.field === "title")
  f.approval = "approved"
  f.support = "supported"
  doc.evidence.push({
    id: "e",
    revision: 1,
    excerpt: "Dev at A",
    origin: "career notes",
    approval: "approved",
  })
  doc.links.push({
    id: "l",
    kind: "supports",
    from: { profileId: "p", id: f.id, revision: 1 },
    to: { profileId: "p", id: "e", revision: 1 },
    state: "active",
  })
  const next = replaceProfileView(doc, { ...profileView(doc), experience: [] })
  assert.equal(validateProfileDocument(next), true)
  assert.equal(next.links[0].state, "invalidated")
  assert.equal(next.evidence[0].excerpt, "Dev at A")
  assert.equal(
    next.facts.some((fact) => fact.id === f.id),
    false,
  )
})
test("old optional entry fields have safe views and original date wording is explicit", () => {
  const doc = migrateProfile(
    {
      experience: [{ id: "role", company: "A" }],
      certifications: [{ id: "certificate", date: "roughly 2020" }],
    },
    "p",
  )
  assert.equal(profileView(doc).experience[0].title, "")
  assert.equal(profileView(doc).experience[0].current, false)
  const date = doc.facts.find((f) => f.field === "date")
  assert.deepEqual(date.temporal, {
    wording: "roughly 2020",
    precision: "unknown",
  })
})
test("a typed context-link correction also invalidates support when no inline context is present", () => {
  const doc = migrateProfile(legacy, "p")
  const f = doc.facts.find((f) => f.field === "skills")
  f.approval = "approved"
  f.support = "supported"
  const ref = (id) => ({ profileId: "p", id, revision: 1 })
  doc.evidence.push({
    id: "e",
    revision: 1,
    excerpt: legacy.skills,
    origin: "career notes",
    approval: "approved",
  })
  doc.links.push(
    {
      id: "s",
      kind: "supports",
      from: ref(f.id),
      to: ref("e"),
      state: "active",
    },
    {
      id: "c",
      kind: "role_context",
      from: ref(f.id),
      to: ref(doc.entities[0].id),
      state: "active",
    },
  )
  const next = replaceProfileView(doc, { ...profileView(doc), experience: [] })
  assert.equal(
    next.facts.find((fact) => fact.id === f.id).support,
    "invalidated",
  )
  assert.ok(next.links.every((link) => link.state === "invalidated"))
})
test("compatibility field enumeration does not revise an unchanged career entity", () => {
  const doc = migrateProfile(
    {
      ...legacy,
      experience: [
        {
          id: "role",
          title: "Dev",
          company: "A",
          startDate: "about 3 years ago",
          endDate: "",
          current: true,
          location: "",
          description: "Maybe led team",
          responsibilities: "",
          achievements: "",
        },
      ],
    },
    "p",
  )
  const title = doc.facts.find((f) => f.field === "title")
  title.approval = "approved"
  title.support = "supported"
  doc.evidence.push({
    id: "title-evidence",
    revision: 1,
    excerpt: "Dev at A",
    origin: "career notes",
    approval: "approved",
  })
  doc.links.push({
    id: "title-support",
    kind: "supports",
    from: { profileId: "p", id: title.id, revision: 1 },
    to: { profileId: "p", id: "title-evidence", revision: 1 },
    state: "active",
  })
  const next = replaceProfileView(doc, {
    ...profileView(doc),
    skills: "Docker",
  })
  assert.equal(next.entities[0].revision, doc.entities[0].revision)
  assert.equal(next.facts.find((f) => f.id === title.id).support, "supported")
})

test("targeted manual wording edits retain unrelated IDs, revisions and qualifiers", () => {
  const doc = migrateProfile(legacy, "profile")
  const fact = doc.facts.find((f) => f.field === "skills")
  fact.kind = "statement"
  fact.assertion = "negated"
  fact.intent = "aspiration"
  fact.certainty = "uncertain"
  fact.temporal = { wording: "about 3 years", precision: "approximate" }
  const next = editProfile(doc, {
    type: "field",
    ownerId: doc.id,
    field: "skills",
    value: "No Java; hope to learn Go later",
  })
  const edited = next.facts.find((f) => f.id === fact.id)
  assert.equal(edited.revision, 2)
  assert.equal(edited.assertion, "negated")
  assert.equal(edited.intent, "aspiration")
  assert.deepEqual(edited.temporal, fact.temporal)
  assert.deepEqual(edited.origin, { kind: "manual_edit", original: "user" })
  for (const untouched of doc.facts.filter((f) => f.id !== fact.id)) {
    const saved = next.facts.find((f) => f.id === untouched.id)
    assert.equal(saved.revision, untouched.revision)
    assert.equal(saved.value, untouched.value)
  }
  assert.equal(validateProfileDocument(next), true)
  assert.equal(profileView(next).skills, "No Java; hope to learn Go later")
})

test("explicit relationship corrections invalidate evidence, preserve other links and reject wrong target kinds", () => {
  const doc = migrateProfile(
    { ...legacy, projects: [{ id: "project", name: "Atlas" }] },
    "p",
  )
  const f = doc.facts.find((f) => f.field === "skills")
  f.support = "supported"
  f.approval = "approved"
  const ref = (id) => ({ profileId: "p", id, revision: 1 })
  doc.evidence.push({
    id: "excerpt",
    revision: 1,
    excerpt: "Go at A",
    origin: "notes",
    approval: "approved",
  })
  const role = doc.entities.find((e) => e.kind === "experience")
  const project = doc.entities.find((e) => e.kind === "projects")
  doc.links.push(
    {
      id: "support",
      kind: "supports",
      from: ref(f.id),
      to: ref("excerpt"),
      state: "active",
    },
    {
      id: "role",
      kind: "role_context",
      from: ref(f.id),
      to: ref(role.id),
      state: "active",
    },
  )
  const next = editProfile(doc, {
    type: "context",
    id: f.id,
    kind: "project_context",
    targetId: project.id,
  })
  assert.equal(next.facts.find((x) => x.id === f.id).support, "invalidated")
  assert.equal(next.links.find((x) => x.id === "support").state, "invalidated")
  assert.equal(next.links.find((x) => x.id === "role").state, "active")
  assert.equal(next.evidence[0].excerpt, "Go at A")
  assert.throws(() =>
    editProfile(doc, {
      type: "context",
      id: f.id,
      kind: "role_context",
      targetId: project.id,
    }),
  )
  const edited = editProfile(doc, {
    type: "field",
    ownerId: doc.id,
    field: "skills",
    value: "No Java",
  })
  assert.equal(edited.links.find((x) => x.id === "role").state, "active")
  assert.equal(
    edited.links.find((x) => x.id === "support").state,
    "invalidated",
  )
})

test("every section supports explicit create, edit, remove and durable reload without touching recovery", async () => {
  const s = storage({ [LEGACY_KEY]: JSON.stringify(legacy) })
  let doc = await openProfile(s, lock)
  const apply = async (edit) => {
    const next = editProfile(doc, edit)
    await saveProfile(s, doc, next, lock)
    doc = await openProfile(s, lock)
    assert.deepEqual(doc, next)
  }
  for (const field of [
    "fullName",
    "email",
    "phone",
    "location",
    "professionalLinks",
    "careerGoals",
    "skills",
    "competencies",
    "tools",
    "employmentStatus",
    "currentSalary",
    "desiredSalary",
    "additionalInfo",
  ]) {
    await apply({
      type: "field",
      ownerId: doc.id,
      field,
      value: "Unknown; no Java; aspire to Go",
    })
    const fact = doc.facts.find(
      (f) => f.owner.id === doc.id && f.field === field,
    )
    await apply({
      type: "field",
      ownerId: doc.id,
      field,
      value: "Approximately 2020; still uncertain",
    })
    assert.equal(
      doc.facts.find((f) => f.id === fact.id).revision,
      fact.revision + 1,
    )
    assert.equal(profileView(doc)[field], "Approximately 2020; still uncertain")
    await apply({ type: "remove_fact", id: fact.id })
    assert.equal(profileView(doc)[field], "")
  }
  for (const [kind, field] of [
    ["experience", "company"],
    ["projects", "name"],
    ["education", "degree"],
    ["certifications", "name"],
    ["languages", "name"],
  ]) {
    await apply({
      type: "add_entity",
      kind,
      legacyId: "new-" + kind,
      values: { [field]: "Unknown" },
    })
    const entity = doc.entities.find((e) => e.legacyId === "new-" + kind)
    await apply({
      type: "field",
      ownerId: entity.id,
      field,
      value: "Perhaps A",
    })
    assert.equal(profileView(doc)[kind][0][field], "Perhaps A")
    await apply({ type: "remove_entity", id: entity.id })
    assert.ok(!doc.entities.some((e) => e.id === entity.id))
  }
  assert.equal(s.getItem(LEGACY_KEY), JSON.stringify(legacy))
})

test("explicit regrouping preserves unrelated order and refuses occupied or foreign destinations atomically", () => {
  let doc = migrateProfile(
    {
      ...legacy,
      experience: [...legacy.experience, { id: "second", company: "B" }],
    },
    "p",
  )
  const [first, second] = doc.entities
  const fact = doc.facts.find(
    (f) => f.owner.id === first.id && f.field === "achievements",
  )
  doc = editProfile(doc, {
    type: "move_fact",
    id: fact.id,
    ownerId: second.id,
    field: "achievements",
  })
  assert.equal(doc.facts.find((f) => f.id === fact.id).owner.id, second.id)
  assert.deepEqual(
    profileView(doc).experience.map((e) => e.company),
    ["A", "B"],
  )
  const reordered = editProfile(doc, {
    type: "reorder",
    kind: "experience",
    ids: [second.id, first.id],
  })
  assert.deepEqual(
    profileView(reordered).experience.map((e) => e.company),
    ["B", "A"],
  )
  assert.equal(
    reordered.entities.find((e) => e.id === first.id).revision,
    first.revision,
  )
  const before = JSON.stringify(doc)
  assert.throws(() =>
    editProfile(doc, {
      type: "move_fact",
      id: fact.id,
      ownerId: first.id,
      field: "company",
    }),
  )
  assert.throws(() =>
    editProfile(doc, {
      type: "move_fact",
      id: fact.id,
      ownerId: "foreign",
      field: "achievements",
    }),
  )
  assert.equal(JSON.stringify(doc), before)
})

test("removing period wording invalidates dependent support without reassigning excerpts", () => {
  const doc = migrateProfile(legacy, "p")
  const period = doc.facts.find((f) => f.field === "startDate")
  const skill = doc.facts.find((f) => f.field === "skills")
  const owner = doc.entities[0]
  skill.approval = "approved"
  skill.support = "supported"
  const ref = (id, revision = 1) => ({ profileId: "p", id, revision })
  doc.evidence.push({
    id: "e",
    revision: 1,
    excerpt: "Used Go at A",
    origin: "notes",
    approval: "approved",
  })
  doc.links.push(
    {
      id: "s",
      kind: "supports",
      from: ref(skill.id),
      to: ref("e"),
      state: "active",
    },
    {
      id: "period",
      kind: "period_context",
      from: ref(skill.id),
      to: ref(owner.id),
      state: "active",
    },
  )
  const next = editProfile(doc, { type: "remove_fact", id: period.id })
  assert.equal(next.facts.find((f) => f.id === skill.id).support, "invalidated")
  assert.equal(next.links.find((l) => l.id === "s").state, "invalidated")
  assert.deepEqual(next.evidence, doc.evidence)
})

test("typed relationship changes preserve unrelated inline context", () => {
  const doc = migrateProfile(
    {
      ...legacy,
      education: [{ id: "school", degree: "BSc" }],
      projects: [{ id: "project", name: "Atlas" }],
    },
    "p",
  )
  const fact = doc.facts.find((f) => f.field === "skills")
  const school = doc.entities.find((e) => e.kind === "education")
  const project = doc.entities.find((e) => e.kind === "projects")
  fact.context = [{ profileId: "p", id: school.id, revision: 1 }]
  const next = editProfile(doc, {
    type: "context",
    id: fact.id,
    kind: "project_context",
    targetId: project.id,
  })
  assert.ok(
    next.facts
      .find((f) => f.id === fact.id)
      .context.some((r) => r.id === school.id),
  )
})

test("explicit inline context removal retains other typed links and their owners", () => {
  const doc = migrateProfile(
    {
      ...legacy,
      education: [{ id: "school", degree: "BSc" }],
      projects: [{ id: "project", name: "Atlas" }],
    },
    "p",
  )
  const f = doc.facts.find((f) => f.field === "skills")
  const school = doc.entities.find((e) => e.kind === "education")
  const project = doc.entities.find((e) => e.kind === "projects")
  f.context = [{ profileId: "p", id: school.id, revision: 1 }]
  const linked = editProfile(doc, {
    type: "context",
    id: f.id,
    kind: "project_context",
    targetId: project.id,
  })
  const next = editProfile(linked, {
    type: "remove_context",
    id: f.id,
    targetId: school.id,
  })
  assert.deepEqual(
    next.facts.find((fact) => fact.id === f.id).context.map((r) => r.id),
    [project.id],
  )
  assert.equal(
    next.links.find((l) => l.kind === "project_context").state,
    "active",
  )
  assert.equal(
    next.links.find((l) => l.kind === "project_context").to.id,
    project.id,
  )
  assert.equal(next.facts.find((fact) => fact.id === f.id).revision, 3)
})

test("section proposals remain transient, scoped and revision-bound until explicitly applied", () => {
  const doc = migrateProfile(
    { ...legacy, careerGoals: "Unrelated goal" },
    "review-profile",
  )
  const request = sectionReviewRequest(doc, "skills", "en")
  assert.equal(JSON.stringify(request).includes("Unrelated goal"), false)
  const fact = request.document.facts.find((f) => f.field === "skills")
  const proposal = {
    profileId: doc.id,
    revision: doc.revision,
    section: "skills",
    summary: "Clearer wording",
    patches: [
      {
        factId: fact.id,
        revision: fact.revision,
        wording: "JS? No Java; I hope to learn Go",
        supporting: [{ id: fact.id, revision: fact.revision }],
      },
    ],
  }
  assert.equal(validateSectionProposal(request, proposal), true)
  assert.equal(
    doc.facts.find((f) => f.id === fact.id).value,
    "JS? No Java; hope to learn Go",
  )
  const next = applySectionProposal(doc, request, proposal, {}, [])
  assert.equal(profileView(next).skills, "JS? No Java; I hope to learn Go")
  assert.equal(profileView(next).careerGoals, "Unrelated goal")
  assert.throws(() =>
    applySectionProposal(
      editProfile(doc, {
        type: "fact",
        id: fact.id,
        patch: { certainty: "uncertain" },
      }),
      request,
      proposal,
      {},
      [],
    ),
  )
})

test("section proposal validation rejects invalid references, protected metadata and silent fact loss", () => {
  const doc = migrateProfile({ skills: "Go", tools: "Docker" }, "p")
  const request = sectionReviewRequest(doc, "skills", "pt-BR")
  const proposal = {
    profileId: "p",
    revision: 1,
    section: "skills",
    summary: "Revisado",
    patches: request.document.facts.map((f) => ({
      factId: f.id,
      revision: f.revision,
      wording: String(f.value),
      supporting: [{ id: f.id, revision: f.revision }],
    })),
  }
  assert.equal(validateSectionProposal(request, proposal), true)
  for (const change of [
    (p) => p.patches.pop(),
    (p) => p.patches[0].supporting[0].revision++,
    (p) => (p.patches[0].owner = "invented"),
    (p) => (p.patches[0].supporting = [p.patches[1].supporting[0]]),
    (p) => p.patches.push(p.patches[0]),
  ]) {
    const bad = structuredClone(proposal)
    change(bad)
    assert.equal(validateSectionProposal(request, bad), false)
  }
})
test("acceptance preserves excerpt support, but edited meaning becomes user-authored and explicit removal retains other facts", () => {
  const doc = migrateProfile({ skills: "Go", tools: "Docker" }, "p")
  const fact = doc.facts.find((f) => f.field === "skills")
  fact.approval = "approved"
  fact.support = "supported"
  doc.evidence.push({
    id: "excerpt",
    revision: 1,
    excerpt: "Go",
    origin: "Accepted note",
    approval: "approved",
  })
  doc.links.push({
    id: "support",
    kind: "supports",
    from: { profileId: "p", id: fact.id, revision: 1 },
    to: { profileId: "p", id: "excerpt", revision: 1 },
    state: "active",
  })
  const request = sectionReviewRequest(doc, "skills", "en")
  const proposal = {
    profileId: "p",
    revision: 1,
    section: "skills",
    summary: "Clearer",
    patches: request.document.facts.map((f) => ({
      factId: f.id,
      revision: 1,
      wording: f.field === "skills" ? "I use Go" : "Docker",
      supporting: [{ id: f.id, revision: 1 }],
    })),
  }
  const accepted = applySectionProposal(doc, request, proposal, {}, [])
  assert.equal(
    accepted.facts.find((f) => f.id === fact.id).support,
    "supported",
  )
  assert.equal(accepted.links.find((l) => l.id === "support").from.revision, 2)
  const edited = applySectionProposal(
    doc,
    request,
    proposal,
    { [fact.id]: "I use Rust" },
    [],
  )
  assert.deepEqual(edited.facts.find((f) => f.id === fact.id).origin, {
    kind: "manual_edit",
    original: "user",
  })
  assert.equal(
    edited.facts.find((f) => f.id === fact.id).support,
    "invalidated",
  )
  assert.equal(edited.evidence[0].excerpt, "Go")
  const removed = applySectionProposal(doc, request, proposal, {}, [fact.id])
  assert.equal(profileView(removed).skills, "")
  assert.equal(profileView(removed).tools, "Docker")
})

test("overview and unsupported subsections cannot create a section rewrite request", () => {
  const doc = migrateProfile(legacy, "p")
  for (const section of [
    "profile",
    "overview",
    "education",
    "certifications",
    "languages",
  ])
    assert.throws(() => sectionReviewRequest(doc, section, "en"))
})
test("correcting a source fact invalidates a composite rewrite that shares its accepted excerpt", () => {
  const doc = migrateProfile({ skills: "Go", tools: "Docker" }, "p")
  const skill = doc.facts.find((f) => f.field === "skills"),
    tool = doc.facts.find((f) => f.field === "tools")
  for (const f of [skill, tool]) {
    f.approval = "approved"
    f.support = "supported"
    doc.evidence.push({
      id: f.id + "-e",
      revision: 1,
      excerpt: String(f.value),
      origin: "Accepted note",
      approval: "approved",
    })
    doc.links.push({
      id: f.id + "-l",
      kind: "supports",
      state: "active",
      from: { profileId: "p", id: f.id, revision: 1 },
      to: { profileId: "p", id: f.id + "-e", revision: 1 },
    })
  }
  const request = sectionReviewRequest(doc, "skills", "en")
  const proposal = {
    profileId: "p",
    revision: 1,
    section: "skills",
    summary: "Combined context",
    patches: [
      {
        factId: skill.id,
        revision: 1,
        wording: "Go with Docker",
        supporting: [
          { id: skill.id, revision: 1 },
          { id: tool.id, revision: 1 },
        ],
      },
      {
        factId: tool.id,
        revision: 1,
        wording: "Docker",
        supporting: [{ id: tool.id, revision: 1 }],
      },
    ],
  }
  const accepted = applySectionProposal(doc, request, proposal, {}, [])
  const corrected = editProfile(accepted, {
    type: "fact",
    id: tool.id,
    patch: { value: "Podman" },
  })
  assert.equal(
    corrected.facts.find((f) => f.id === skill.id).support,
    "invalidated",
  )
  assert.equal(corrected.evidence.length, 2)
})
test("editing a composite source in the same acceptance never reactivates invalid support, regardless of patch order", () => {
  const doc = migrateProfile({ skills: "Go", tools: "Docker" }, "p")
  const skill = doc.facts.find((f) => f.field === "skills"),
    tool = doc.facts.find((f) => f.field === "tools")
  for (const f of [skill, tool]) {
    f.approval = "approved"
    f.support = "supported"
    doc.evidence.push({
      id: f.id + "-e",
      revision: 1,
      excerpt: String(f.value),
      origin: "Accepted note",
      approval: "approved",
    })
    doc.links.push({
      id: f.id + "-l",
      kind: "supports",
      state: "active",
      from: { profileId: "p", id: f.id, revision: 1 },
      to: { profileId: "p", id: f.id + "-e", revision: 1 },
    })
  }
  const request = sectionReviewRequest(doc, "skills", "en")
  const proposal = {
    profileId: "p",
    revision: 1,
    section: "skills",
    summary: "Combined",
    patches: [
      {
        factId: tool.id,
        revision: 1,
        wording: "Docker",
        supporting: [{ id: tool.id, revision: 1 }],
      },
      {
        factId: skill.id,
        revision: 1,
        wording: "Go with Docker",
        supporting: [
          { id: skill.id, revision: 1 },
          { id: tool.id, revision: 1 },
        ],
      },
    ],
  }
  const accepted = applySectionProposal(
    doc,
    request,
    proposal,
    { [tool.id]: "Podman" },
    [],
  )
  assert.equal(
    accepted.facts.find((f) => f.id === skill.id).support,
    "invalidated",
  )
})
test("deliberate removal accepts an emptied wording field and malformed supporting references fail validation safely", () => {
  const doc = migrateProfile({ skills: "Go" }, "p"),
    request = sectionReviewRequest(doc, "skills", "en"),
    fact = request.document.facts[0]
  const proposal = {
    profileId: "p",
    revision: 1,
    section: "skills",
    summary: "Clearer",
    patches: [
      {
        factId: fact.id,
        revision: 1,
        wording: "I use Go",
        supporting: [{ id: fact.id, revision: 1 }],
      },
    ],
  }
  assert.equal(
    profileView(
      applySectionProposal(doc, request, proposal, { [fact.id]: "" }, [
        fact.id,
      ]),
    ).skills,
    "",
  )
  const malformed = structuredClone(proposal)
  malformed.patches[0].supporting = [null]
  assert.equal(validateSectionProposal(request, malformed), false)
})

const { applyIngestionDocument } = await import(
  join(temp, "ingestionDocument.mjs")
)
const ingestionClaim = (id, source, targets = ["skills"]) => ({
  id,
  source,
  text: source,
  targets,
  question: "",
})
const ingestionOp = (patch = {}) => ({
  claimId: "c1",
  target: "skills",
  entryId: "",
  field: "skills",
  action: "add",
  value: "TypeScript",
  finding: "addition",
  approved: true,
  ...patch,
})
const ingestionResult = (claims, operations) => ({
  claims,
  operations,
  unverifiedClaimCount: 0,
  unresolvedClaimIds: [],
  unplacedOperationCount: 0,
})
test("accepted ingestion keeps only approved excerpts through canonical storage reload", async () => {
  const doc = migrateProfile({ skills: "" }, "ingested")
  const result = ingestionResult(
    [
      ingestionClaim("c1", "I use TypeScript."),
      ingestionClaim("c2", "I use Java."),
    ],
    [
      ingestionOp(),
      ingestionOp({ claimId: "c2", value: "Java", approved: false }),
    ],
  )
  const next = applyIngestionDocument(
    doc,
    doc,
    "I use TypeScript. I use Java. PRIVATE PASTE",
    result,
  )
  assert.equal(validateProfileDocument(next), true)
  assert.equal(profileView(next).skills, "TypeScript")
  assert.deepEqual(
    next.evidence.map((e) => e.excerpt),
    ["I use TypeScript."],
  )
  const fact = next.facts.find((f) => f.field === "skills")
  assert.equal(fact.kind, "statement")
  assert.equal(fact.approval, "approved")
  assert.equal(fact.support, "supported")
  assert.equal(fact.normalization.canonical, "TypeScript")
  const saved = storage({ careeros_profile_v2: JSON.stringify(doc) })
  await saveProfile(saved, doc, next, lock)
  const reloaded = await openProfile(saved, lock)
  assert.deepEqual(reloaded.evidence, next.evidence)
  assert.equal(JSON.stringify(reloaded).includes("PRIVATE PASTE"), false)
})
test("composite accepted facts keep every supporting excerpt and reject partial dependencies", () => {
  const doc = migrateProfile({ skills: "" }, "composite")
  const result = ingestionResult(
    [
      ingestionClaim("c1", "I use TypeScript."),
      ingestionClaim("c2", "I use JavaScript."),
    ],
    [
      ingestionOp({
        value: "TypeScript and JavaScript",
        supportingClaimIds: ["c1", "c2"],
      }),
      ingestionOp({
        claimId: "c2",
        target: "tools",
        field: "tools",
        value: "JavaScript",
      }),
    ],
  )
  result.claims[1].targets.push("tools")
  const next = applyIngestionDocument(
    doc,
    doc,
    "I use TypeScript. I use JavaScript.",
    result,
  )
  const fact = next.facts.find((f) => f.field === "skills")
  assert.equal(
    next.links.filter((l) => l.from.id === fact.id && l.state === "active")
      .length,
    2,
  )
  assert.deepEqual(
    next.evidence
      .filter((e) =>
        next.links.some((l) => l.from.id === fact.id && l.to.id === e.id),
      )
      .map((e) => e.excerpt),
    ["I use TypeScript.", "I use JavaScript."],
  )
  assert.throws(
    () =>
      applyIngestionDocument(doc, doc, "I use TypeScript. I use JavaScript.", {
        ...result,
        operations: [result.operations[0]],
      }),
    /invalid_output/,
  )
})

test("edited ingestion is user authored and does not persist the proposed evidence", () => {
  const doc = migrateProfile({ skills: "" }, "edit-ingestion")
  const result = ingestionResult([ingestionClaim("c1", "I use TypeScript.")], [
    ingestionOp({ value: "I hope to learn Go", proposedValue: "TypeScript" }),
  ])
  const next = applyIngestionDocument(doc, doc, "I use TypeScript.", result)
  const fact = next.facts.find((f) => f.field === "skills")
  assert.deepEqual(fact.origin, { kind: "manual_edit", original: "user" })
  assert.equal(fact.support, "unsupported")
  assert.equal(next.evidence.length, 0)
  assert.equal(fact.intent, "unknown")
})
test("generated ingestion refuses protected numbers and technologies absent from its sources", () => {
  const doc = migrateProfile({ skills: "" }, "protected-ingestion")
  for (const value of [
    "TypeScript and Kubernetes",
    "TypeScript for 20 years",
    "JavaScript",
  ]) {
    assert.throws(
      () =>
        applyIngestionDocument(
          doc,
          doc,
          "I use TypeScript.",
          ingestionResult([ingestionClaim("c1", "I use TypeScript.")], [
            ingestionOp({ value }),
          ]),
        ),
      /invalid_output/,
    )
  }
})
test("distinct accepted role stints keep ownership, approximate dates and negation without cross-role support", () => {
  const doc = migrateProfile({ skills: "" }, "role-ingestion")
  const input =
    "Acme Engineer around 2020; did not use Java.\nAcme Engineer 2024; used TypeScript."
  const claims = [
    ingestionClaim("c1", "Acme Engineer around 2020; did not use Java.", [
      "experience",
    ]),
    ingestionClaim("c2", "Acme Engineer 2024; used TypeScript.", [
      "experience",
    ]),
  ]
  claims[0].meaning = {
    assertion: "negated",
    intent: "actual",
    certainty: "certain",
    temporal: { wording: "around 2020", precision: "approximate" },
  }
  claims[1].meaning = {
    assertion: "affirmed",
    intent: "actual",
    certainty: "certain",
    temporal: { wording: "2024", precision: "exact" },
  }
  const operations = claims.flatMap((c, index) =>
    Object.entries({
      company: "Acme",
      title: "Engineer",
      startDate: index ? "2024" : "around 2020",
      responsibilities: index ? "Used TypeScript" : "Did not use Java",
    }).map(([field, value]) =>
      ingestionOp({
        claimId: c.id,
        target: "experience",
        entryId: "new:" + c.id,
        field,
        value,
      }),
    ),
  )
  const result = ingestionResult(claims, operations)
  const next = applyIngestionDocument(doc, doc, input, result)
  assert.equal(profileView(next).experience.length, 2)
  for (const [i, entity] of next.entities.entries()) {
    const fact = next.facts.find(
      (f) => f.owner.id === entity.id && f.field === "responsibilities",
    )
    assert.equal(fact.assertion, i ? "affirmed" : "negated")
    assert.equal(fact.temporal.precision, i ? "exact" : "approximate")
    assert.ok(
      next.links.some(
        (l) =>
          l.from.id === fact.id &&
          l.kind === "role_context" &&
          l.to.id === entity.id,
      ),
    )
    const evidenceIds = next.links
      .filter((l) => l.from.id === fact.id && l.kind === "supports")
      .map((l) => l.to.id)
    assert.deepEqual(
      next.evidence
        .filter((e) => evidenceIds.includes(e.id))
        .map((e) => e.excerpt),
      [claims[i].source],
    )
  }
  // Even matching employers and role titles cannot justify borrowing another stint's evidence.
  result.operations[3].supportingClaimIds = ["c1", "c2"]
  claims[1].meaning = claims[0].meaning
  assert.throws(
    () => applyIngestionDocument(doc, doc, input, result),
    /invalid_output/,
  )
})
test("all structured ingestion destinations commit together", () => {
  const doc = migrateProfile({ skills: "" }, "all-destinations")
  const source =
    "Ada ada@example.test +55 11 5555 São Paulo https://example.test/ada Go BSc North 2021 Cloud Guild Portuguese Fluent Aster Engineer Harbor"
  const claims = [
    ingestionClaim("c1", source, [
      "fullName",
      "email",
      "phone",
      "location",
      "professionalLinks",
      "education",
      "certifications",
      "languages",
      "experience",
      "projects",
      "skills",
    ]),
  ]
  const ops = [
    ...Object.entries({
      fullName: "Ada",
      email: "ada@example.test",
      phone: "+55 11 5555",
      location: "São Paulo",
      professionalLinks: "https://example.test/ada",
      skills: "Go",
    }).map(([target, value]) =>
      ingestionOp({
        target,
        field: target,
        action: target === "skills" ? "add" : "update",
        value,
        proposedValue: value,
      }),
    ),
    ...Object.entries({
      education: {
        degree: "BSc",
        institution: "North",
        graduationDate: "2021",
      },
      certifications: { name: "Cloud", issuer: "Guild" },
      languages: { name: "Portuguese", proficiency: "Fluent" },
      experience: { company: "Aster", title: "Engineer" },
      projects: { name: "Harbor" },
    }).flatMap(([target, fields]) =>
      Object.entries(fields).map(([field, value]) =>
        ingestionOp({
          target,
          entryId: "new:c1",
          field,
          value,
          proposedValue: value,
        }),
      ),
    ),
  ]
  ops[0].value = "Ada Reviewed"
  const next = applyIngestionDocument(
    doc,
    doc,
    source,
    ingestionResult(claims, ops),
  )
  assert.equal(validateProfileDocument(next), true)
  assert.equal(profileView(next).education[0].institution, "North")
  assert.equal(profileView(next).experience[0].company, "Aster")
  assert.equal(profileView(next).languages[0].proficiency, "Fluent")
})
test("accepted alias normalization retains observed spelling and remains conservative", () => {
  const doc = migrateProfile({ skills: "" }, "alias-ingestion")
  const next = applyIngestionDocument(
    doc,
    doc,
    "I use Javascript.",
    ingestionResult([ingestionClaim("c1", "I use Javascript.")], [
      ingestionOp({ value: "JavaScript" }),
    ]),
  )
  assert.deepEqual(next.facts.find((f) => f.field === "skills").normalization, {
    observed: "Javascript",
    canonical: "JavaScript",
    policy: "exact-alias-v1",
  })
})
test("ingestion preserves aspirations and rejects metadata-only stale acceptance", () => {
  const doc = migrateProfile({skills:""},"aspiration-ingestion")
  const source = "I hope to learn Go, perhaps around 2027."
  const claim = ingestionClaim("c1",source)
  claim.meaning = {assertion:"affirmed",intent:"aspiration",certainty:"uncertain",temporal:{wording:"around 2027",precision:"approximate"}}
  const result = ingestionResult([claim],[ingestionOp({value:"Hope to learn Go, perhaps around 2027"})])
  const next = applyIngestionDocument(doc,doc,source,result)
  assert.equal(next.facts.find(f => f.field === "skills").intent,"aspiration")
  assert.equal(next.facts.find(f => f.field === "skills").certainty,"uncertain")
  const changed = editProfile(doc,{type:"fact",id:doc.facts.find(f => f.field === "skills").id,patch:{intent:"actual"}})
  assert.throws(() => applyIngestionDocument(changed,doc,source,result), /stale/)
})
test("complete replacements use their own accepted evidence and invalidate obsolete excerpts", () => {
 const source = "I use Python."
 const result = ingestionResult([ingestionClaim("c1",source)],[ingestionOp({action:"update",value:"Python"})])
 const migrated = migrateProfile({skills:"Java"},"replace-ingestion")
 const fresh = applyIngestionDocument(migrated,migrated,source,result)
 assert.equal(fresh.facts.find(f => f.field === "skills").support,"supported")
 const empty = migrateProfile({skills:""},"old-evidence")
 const doc = applyIngestionDocument(empty,empty,"I use Java.",ingestionResult([ingestionClaim("c1","I use Java.")],[ingestionOp({value:"Java"})]))
 const next = applyIngestionDocument(doc,doc,source,result)
 const fact = next.facts.find(f => f.field === "skills")
 const active = next.links.filter(l => l.from.id === fact.id && l.kind === "supports" && l.state === "active").map(l => l.to.id)
 assert.deepEqual(next.evidence.filter(e => active.includes(e.id)).map(e => e.excerpt),["I use Python."])
 assert.ok(next.evidence.some(e => e.excerpt === "I use Java."))
 assert.equal(fact.support,"supported")
})
test("identity support can accompany an approximate dated narrative without inheriting its qualifiers", () => {
 const doc = migrateProfile({skills:""},"composite-period")
 const input = "At Acme I was an Engineer.\nI used Java around 2020."
 const c1=ingestionClaim("c1","At Acme I was an Engineer.",["experience"])
 const c2=ingestionClaim("c2","I used Java around 2020.",["experience"])
 c1.meaning={assertion:"affirmed",intent:"actual",certainty:"certain",temporal:{wording:"",precision:"unknown"}}
 c2.meaning={assertion:"affirmed",intent:"actual",certainty:"certain",temporal:{wording:"around 2020",precision:"approximate"}}
 const ops=[ingestionOp({target:"experience",entryId:"new:c1",field:"company",value:"Acme"}),ingestionOp({target:"experience",entryId:"new:c1",field:"title",value:"Engineer"}),ingestionOp({claimId:"c2",target:"experience",entryId:"new:c1",field:"responsibilities",value:"Used Java around 2020",supportingClaimIds:["c2","c1"]})]
 const next=applyIngestionDocument(doc,doc,input,ingestionResult([c1,c2],ops))
 const fact=next.facts.find(f=>f.field==="responsibilities")
 assert.equal(fact.temporal.precision,"approximate")
 assert.equal(fact.temporal.wording,"around 2020")
 assert.equal(next.links.filter(l=>l.kind==="supports" && l.from.id===fact.id && l.state==="active").length,2)
})
