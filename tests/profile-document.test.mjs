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
for (const name of ["profileDocument", "profileStorage"]) {
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
