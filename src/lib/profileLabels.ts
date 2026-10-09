import type { TranslationKey } from "./i18n"

// Shared by proposal rows and canonical fact details, including qualification fields.
export function profileFieldKey(
  section: string,
  field: string,
): TranslationKey {
  const prefixes: Record<string, string> = {
    education: "education.",
    certifications: "certification.",
    languages: "language.",
  }
  if (prefixes[section])
    return `repo.${prefixes[section]}${field}` as TranslationKey
  if (field === "current") return "common.current"
  const aliases: Record<string, string> = {
    title: "jobTitle",
    name: "projectName",
    technologies: "technologiesUsed",
    currentSalary: "currentCompensation",
    desiredSalary: "desiredCompensation",
  }
  return `repo.${aliases[field] || field}` as TranslationKey
}
