import type { TranslationKey } from "./i18n"

// One label per review destination target, shared by the destination-based
// review and the manual Profile sections. Profile-level targets read as the
// section they change.
const destinationKeys: Record<string, TranslationKey> = {
  education: "repo.section.education",
  certifications: "repo.section.certifications",
  languages: "repo.section.languages",
  experience: "repo.section.experience",
  projects: "repo.section.projects",
  careerGoals: "repo.careerGoals",
  skills: "repo.skills",
  competencies: "repo.competencies",
  tools: "repo.tools",
  additionalInfo: "repo.additionalInfo",
  employmentStatus: "repo.employmentStatus",
  currentSalary: "repo.currentCompensation",
  desiredSalary: "repo.desiredCompensation",
  fullName: "repo.fullName",
  email: "repo.email",
  phone: "repo.phone",
  location: "repo.location",
  professionalLinks: "repo.professionalLinks",
}

export function destinationLabelKey(target: string): TranslationKey {
  return destinationKeys[target] ?? "repo.additionalInfo"
}

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
