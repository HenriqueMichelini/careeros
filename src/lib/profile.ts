import type { ProfessionalRepository } from "./types"

export const emptyContact = {
  fullName: "",
  email: "",
  phone: "",
  location: "",
  professionalLinks: "",
}

// Older saved Profiles have no contact fields. Keep every existing fact while
// giving the new editors controlled string values.
export function withContactFields(saved: ProfessionalRepository): ProfessionalRepository {
  return { ...emptyContact, ...saved }
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
