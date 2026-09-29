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

export interface ProfessionalRepository {
  careerGoals: string
  skills: string
  competencies: string
  experience: ExperienceEntry[]
  tools: string
  projects: ProjectEntry[]
  employmentStatus: string
  currentSalary: string
  desiredSalary: string
  additionalInfo: string
}

export interface GeneratedMaterials {
  jobTitle: string
  company: string
  jobSummary: string
  resume: string
  coverLetter: string
  applicationAnswers: string
}

export type Page = 'landing' | 'home' | 'repository' | 'cv' | 'results'
