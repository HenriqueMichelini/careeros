// Synthetic provider/check results for browser rendering only. These fixtures
// exercise the acceptance/export boundary; they do not establish factual support.
export function reviewedDraftFixture(body, input) {
  if (!input?.reviewResume || typeof body.resume !== "string") return body
  const aliases = {
    "professional summary": "summary",
    "resumo profissional": "summary",
    "technical skills": "skills",
    "competências técnicas": "skills",
    "tools & technology": "skills",
    "professional experience": "experience",
    "experiência profissional": "experience",
    projects: "experience",
    education: "education",
    educação: "education",
    certifications: "certifications",
    certificações: "certifications",
    languages: "languages",
    idiomas: "languages",
  }
  const sections = [
    "summary",
    "skills",
    "experience",
    "education",
    "certifications",
    "languages",
  ]
  const titles =
    input.cvLanguage === "pt-BR"
      ? [
          "Resumo Profissional",
          "Competências Técnicas",
          "Experiência Profissional",
          "Educação",
          "Certificações",
          "Idiomas",
        ]
      : [
          "Professional Summary",
          "Technical Skills",
          "Professional Experience",
          "Education",
          "Certifications",
          "Languages",
        ]
  const doc = input.profileEvidence
  const facts = doc.facts.map((f) => ({
    ...f,
    section: doc.entities.find((e) => e.id === f.owner.id)?.kind ?? "profile",
    evidence: doc.links
      .filter(
        (l) =>
          l.kind === "supports" && l.state === "active" && l.from.id === f.id,
      )
      .flatMap((l) => doc.evidence.filter((e) => e.id === l.to.id)),
  }))
  const claims = []
  let section = null
  for (const line of body.resume
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean)) {
    if (line.startsWith("## ")) {
      section = aliases[line.slice(3).toLowerCase()] ?? "summary"
      continue
    }
    if (!section) continue
    const kind = line.startsWith("### ")
      ? "subheading"
      : line.startsWith("- ")
        ? "bullet"
        : "paragraph"
    const text = line.replace(/^### |^- /, "")
    for (let i = 0; i < text.length; i += 1500) {
      const chunk = text.slice(i, i + 1500)
      const f =
        facts.find(
          (f) =>
            typeof f.value === "string" &&
            (f.value.includes(chunk) || chunk.includes(f.value)),
        ) ?? facts[0]
      if (!f) throw Error("Browser fixture requires source facts")
      claims.push({
        section,
        kind,
        text: chunk,
        sources: [{ profileId: doc.id, id: f.id, revision: f.revision }],
        state: "supported",
        concerns: [],
      })
    }
  }
  if (!claims.length) {
    const f = facts[0]
    claims.push({
      section: "skills",
      kind: "bullet",
      text: String(f.value),
      sources: [{ profileId: doc.id, id: f.id, revision: f.revision }],
      state: "supported",
      concerns: [],
    })
  }
  const resume = sections
    .flatMap((s, i) => {
      const lines = claims
        .filter((c) => c.section === s)
        .map(
          (c) =>
            (c.kind === "bullet"
              ? "- "
              : c.kind === "subheading"
                ? "### "
                : "") + c.text,
        )
      return lines.length ? ["## " + titles[i] + "\n" + lines.join("\n\n")] : []
    })
    .join("\n\n")
  return {
    ...body,
    resume,
    ...(input.reviewArtifacts
      ? {
          artifactReview: {
            version: "artifact-review-v1",
            check: "complete",
            facts,
            jobPosting: input.jobPosting,
            closing: body.coverLetter.closing,
            claims: [
              ["jobTitle", body.jobTitle],
              ["company", body.company],
              ["jobSummary", body.jobSummary],
              ["greeting", body.coverLetter.greeting],
              ["body", body.coverLetter.body],
              ["applicationAnswers", body.applicationAnswers],
            ].flatMap(([field, text]) =>
              text
                ? String(text)
                    .split("\n\n")
                    .filter(Boolean)
                    .map((text) => ({
                      field,
                      text,
                      state: "supported",
                      concerns: [],
                      nonfactual: field === "greeting",
                      scope:
                        field === "greeting"
                          ? "nonfactual"
                          : ["body", "applicationAnswers"].includes(field)
                            ? "career"
                            : "job",
                      sources: ["body", "applicationAnswers"].includes(field)
                        ? [
                            {
                              profileId: doc.id,
                              id: facts[0].id,
                              revision: facts[0].revision,
                            },
                          ]
                        : [],
                      jobSources: [
                        "jobTitle",
                        "company",
                        "jobSummary",
                      ].includes(field)
                        ? [
                            {
                              start: 0,
                              end: new TextEncoder().encode(input.jobPosting)
                                .length,
                              quote: input.jobPosting,
                            },
                          ]
                        : [],
                    }))
                : [],
            ),
          },
        }
      : {}),
    resumeReview: {
      version: "resume-review-v1",
      check: "complete",
      facts,
      claims,
    },
  }
}

// Install after the app loads. Preserve each test's own synthetic fetch function
// while adapting its legacy draft fixture to the structured review contract.
export const reviewedFixtureFetchWrapper = `(() => {
 const fixture = ${reviewedDraftFixture.toString()};
 let sourceFetch = window.fetch;
 Object.defineProperty(window, 'fetch', {configurable:true,
  get: () => async (url, options) => {
   const response = await sourceFetch(url, options);
   if (!String(url).includes('/api/application-draft') || !response.ok) return response;
   const body = await response.json();
   return new Response(JSON.stringify(fixture(body, JSON.parse(options.body))), {status:response.status,headers:{'Content-Type':'application/json'}});
  },set: value => {sourceFetch = value;}
 });
})()`
