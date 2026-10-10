export interface ExperienceEntry {
  id: string
  company: string
  title: string
  startDate: string
  endDate: string
  current: boolean
  location: string
  description: string
  responsibilities: string
  achievements: string
}

export interface ProjectEntry {
  id: string
  name: string
  description: string
  technologies: string
  url: string
  highlights: string
}

export interface EducationEntry {
  id: string
  degree: string
  institution: string
  location: string
  graduationDate: string
  details: string
}

export interface CertificationEntry {
  id: string
  name: string
  issuer: string
  date: string
  credentialId: string
  url: string
}

export interface LanguageEntry {
  id: string
  name: string
  proficiency: string
}

export interface ProfessionalRepository {
  fullName: string
  email: string
  phone: string
  location: string
  professionalLinks: string
  careerGoals: string
  skills: string
  competencies: string
  experience: ExperienceEntry[]
  tools: string
  projects: ProjectEntry[]
  education: EducationEntry[]
  certifications: CertificationEntry[]
  languages: LanguageEntry[]
  employmentStatus: string
  currentSalary: string
  desiredSalary: string
  additionalInfo: string
}

export interface GeneratedMaterials {
  contextSelection?: {
    version: "application-context-v1"
    sources: string[]
    budgetExcluded: number
    relevanceExcluded: number
    complete: boolean
    bytes: number
  }


  cvLanguage?: import("./i18n").Locale
  // null means the Job Posting did not supply this metadata.
  jobTitle: string | null
  company: string | null
  jobSummary: string
  resume: string
  coverLetter: string
  coverLetterHasSignature?: boolean
  applicationAnswers: string
}

export interface ProfileGap {
  kind: 'skill' | 'experience'
  requirement: string
  details: string
}

export interface ConfirmedQualification {
  kind: ProfileGap["kind"]
  requirement: string
  userContext: string
}

export type Page = 'landing' | 'home' | 'repository' | 'cv' | 'results'
