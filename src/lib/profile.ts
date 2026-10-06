import type { ProfessionalRepository } from "./types"

export const emptyContact = {
  fullName: "",
  email: "",
  phone: "",
  location: "",
  professionalLinks: "",
}

// Older saved Profiles have no contact or qualification arrays. Keep every
// existing fact while giving the new editors controlled values.
export function withContactFields(saved: ProfessionalRepository): ProfessionalRepository {
  return {
    ...emptyContact,
    ...saved,
    education: Array.isArray(saved.education) ? saved.education : [],
    certifications: Array.isArray(saved.certifications) ? saved.certifications : [],
    languages: Array.isArray(saved.languages) ? saved.languages : [],
  }
}

export function cvQualifications(repo: ProfessionalRepository) {
  return {
    education: repo.education.filter(item => item.degree.trim() || item.institution.trim()),
    certifications: repo.certifications.filter(item => item.name.trim()),
    languages: repo.languages.filter(item => item.name.trim()),
  }
}

export function validQualifications(repo: ProfessionalRepository): boolean {
  const groups = [
    [repo.education, ["id", "degree", "institution", "location", "graduationDate", "details"]],
    [repo.certifications, ["id", "name", "issuer", "date", "credentialId", "url"]],
    [repo.languages, ["id", "name", "proficiency"]],
  ] as const
  const ids = new Set<string>()
  const encoder = new TextEncoder()
  return groups.every(([items, fields]) => Array.isArray(items) && items.length <= 40 &&
    items.every(item => {
      if (!item || typeof item !== "object") return false
      const values = item as unknown as Record<string, unknown>
      if (Object.keys(values).length !== fields.length ||
        !fields.every(field => typeof values[field] === "string" && encoder.encode(values[field] as string).length <= (field === "id" ? 100 : 2000)) ||
        !values.id || ids.has(values.id as string)) return false
      ids.add(values.id as string)
      return true
    }))
}

// Existing AI endpoints accept the original ten-field Profile schema. Contact
// details are for the CV header and are not needed by these workflows.
export function careerProfile(repo: ProfessionalRepository) {
  return {
    careerGoals: repo.careerGoals,
    skills: repo.skills,
    competencies: repo.competencies,
    experience: repo.experience,
    tools: repo.tools,
    projects: repo.projects,
    employmentStatus: repo.employmentStatus,
    currentSalary: repo.currentSalary,
    desiredSalary: repo.desiredSalary,
    additionalInfo: repo.additionalInfo,
  }
}
