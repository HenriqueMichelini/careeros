export const CV_STORAGE_KEY = "careeros_cv_v1"

export const cvSections = [
  "contact",
  "summary",
  "skills",
  "tools",
  "experience",
  "projects",
  "education",
  "certifications",
  "languages",
  "additional",
] as const

export type CvSection = typeof cvSections[number]

export interface CvChoices {
  hiddenSections: string[]
  hiddenEntries: string[]
  hiddenBullets: string[]
  summary: string | null
  bulletWording: Record<string, string>
}

export const emptyCvChoices = (): CvChoices => ({
  hiddenSections: [],
  hiddenEntries: [],
  hiddenBullets: [],
  summary: null,
  bulletWording: {},
})

export const entryKey = (section: CvSection, id: string) =>
  JSON.stringify([section, id])
// The source text is part of the identity so a changed Profile fact cannot
// inherit wording or visibility chosen for an older fact at the same position.
export const bulletKey = (
  section: CvSection,
  id: string,
  field: string,
  index: number,
  source: string,
) => JSON.stringify([section, id, field, index, source])

export function professionalLinkHref(value: string): string | null {
  const text = value.trim()
  if (!text || /\s/.test(text)) return null
  const candidate = /^https?:\/\//i.test(text) ? text : `https://${text}`
  try {
    const url = new URL(candidate)
    if (
      !["http:", "https:"].includes(url.protocol) ||
      !url.hostname.includes(".") ||
      url.username ||
      url.password
    )
      return null
    return url.href
  } catch {
    return null
  }
}

export function parseCvChoices(raw: string | null): CvChoices {
  if (!raw) return emptyCvChoices()
  try {
    const value: unknown = JSON.parse(raw)
    if (!value || typeof value !== "object" || Array.isArray(value))
      return emptyCvChoices()
    const saved = value as Record<string, unknown>
    const strings = (field: string) =>
      Array.isArray(saved[field])
        ? (saved[field] as unknown[]).filter(
            (item): item is string => typeof item === "string",
          )
        : []
    const wording =
      saved.bulletWording &&
      typeof saved.bulletWording === "object" &&
      !Array.isArray(saved.bulletWording)
        ? Object.fromEntries(
            Object.entries(saved.bulletWording).filter(
              ([, text]) => typeof text === "string",
            ),
          ) as Record<string, string>
        : {}
    return {
      hiddenSections: strings("hiddenSections").filter((item) =>
        cvSections.includes(item as CvSection),
      ),
      hiddenEntries: strings("hiddenEntries"),
      hiddenBullets: strings("hiddenBullets"),
      summary: typeof saved.summary === "string" ? saved.summary : null,
      bulletWording: wording,
    }
  } catch {
    return emptyCvChoices()
  }
}
