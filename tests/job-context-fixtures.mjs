// Controlled UI fixtures only. Hashes identify a synthetic artifact; they are
// not authenticated or used as evidence of server-side source validation.
export function jobContextFixture(posting) {
  const source = (quote) => {
    const start = new TextEncoder().encode(
      posting.slice(0, posting.indexOf(quote)),
    ).length
    return {
      segmentId: "fixture-segment",
      quote,
      occurrence: 0,
      original: [
        {
          start,
          end: start + new TextEncoder().encode(quote).length,
          text: quote,
        },
      ],
    }
  }
  return {
    version:
      "job-context-v1/prompt-v1/schema-v1/gpt-6-luna/none/jev-rubric-v1/policy-v1",
    sourceId: "a".repeat(64),
    inputsId: "b".repeat(64),
    job: {
      jobTitle: posting.includes("Java developer")
        ? source("Java developer")
        : null,
      company: null,
      seniority: null,
      location: null,
      responsibilities: [],
      qualifications: posting.includes("AWS required.")
        ? [{ source: source("AWS required."), importance: "required" }]
        : [],
      applicationRequirements: posting.includes(
        "Send salary expectations and portfolio as PDF.",
      )
        ? [
            {
              source: source("Send salary expectations and portfolio as PDF."),
              importance: "unspecified",
            },
          ]
        : [],
    },
  }
}
