import { CvSection } from "./cv"

export interface ResumeBlock {
  kind: "paragraph" | "subheading" | "bullet" | "numbered"
  text: string
}

export interface ResumeSection {
  title: string
  id: CvSection | null
  blocks: ResumeBlock[]
}

const sectionAliases: Record<string, CvSection> = {
  "professional profile": "summary",
  "professional summary": "summary",
  "technical skills": "skills",
  profile: "summary",
  summary: "summary",
  experience: "experience",
  "professional experience": "experience",
  "work experience": "experience",
  skills: "skills",
  "skills competencies": "skills",
  "core skills": "skills",
  "selected projects": "projects",
  projects: "projects",
  education: "education",
  certifications: "certifications",
  languages: "languages",
  "tools technology": "tools",
  "tools and technology": "tools",
  "additional information": "additional",
  additional: "additional",
  "perfil profissional": "summary",
  "resumo profissional": "summary",
  experiencia: "experience",
  "experiencia profissional": "experience",
  "habilidades e competencias": "skills",
  "habilidades competencias": "skills",
  "habilidades tecnicas": "skills",
  "projetos selecionados": "projects",
  projetos: "projects",
  formacao: "education",
  certificacoes: "certifications",
  idiomas: "languages",
  "ferramentas e tecnologias": "tools",
  "informacoes adicionais": "additional",
}

function sectionId(title: string): CvSection | null {
  const normalized = title
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/&/g, " ")
    .replace(/[^a-z0-9]+/g, " ")
    .trim()
  return sectionAliases[normalized] ?? null
}

export function parseResumeHeader(markdown: string): {
  name: string
  contacts: string[]
  body: string
} {
  const lines = markdown.replace(/\r\n?/g, "\n").split("\n")
  const first = lines.findIndex((line) => line.trim())
  const nameMatch = first >= 0 ? lines[first].trim().match(/^#(?!#)\s+(.+)$/) : null
  if (!nameMatch) return { name: "", contacts: [], body: markdown }

  const contacts: string[] = []
  const kept: string[] = []
  let index = first + 1
  for (; index < lines.length; index++) {
    const line = lines[index].trim()
    const boldHeading = line.match(/^\*\*(.+)\*\*$/)
    if (/^#{2,6}\s+/.test(line) || (boldHeading && sectionId(boldHeading[1]))) break
    if (line && /@|https?:\/\/|www\.|\+?\d[\d\s().-]{6,}\d/.test(line)) {
      contacts.push(...line.split(/\s*[|·]\s*/).map((part) => part.trim()).filter(Boolean))
    } else {
      kept.push(lines[index])
    }
  }
  return {
    name: nameMatch[1].replace(/[*_`]/g, "").trim(),
    contacts,
    body: [...kept, ...lines.slice(index)].join("\n"),
  }
}

export function parseResumeMarkdown(markdown: string): ResumeSection[] {
  const sections: ResumeSection[] = []
  let current: ResumeSection | null = null
  let paragraph: string[] = []
  const ensureSection = () => {
    if (!current) {
      current = { title: "", id: null, blocks: [] }
      sections.push(current)
    }
    return current
  }
  const flushParagraph = () => {
    if (paragraph.length) {
      ensureSection().blocks.push({
        kind: "paragraph",
        text: paragraph.join(" "),
      })
      paragraph = []
    }
  }
  for (const raw of markdown.replace(/\r\n?/g, "\n").split("\n")) {
    const line = raw.trim()
    if (!line) {
      flushParagraph()
      continue
    }
    const section = line.match(/^#{1,2}(?!#)\s+(.+)$/)
    const boldHeading = line.match(/^\*\*(.+)\*\*$/)
    const heading = section?.[1] ?? boldHeading?.[1]
    const headingId = heading ? sectionId(heading) : null
    if (
      heading &&
      ((section &&
        (section[0].startsWith("##") || headingId || sections.length > 0)) ||
        (boldHeading && headingId))
    ) {
      flushParagraph()
      const title = heading.replace(/[*_`]/g, "").trim()
      current = { title, id: sectionId(title), blocks: [] }
      sections.push(current)
      continue
    }
    if (/^#\s+/.test(line) && !sections.length) continue
    const subheading = line.match(/^#{3,6}\s+(.+)$/)
    if (subheading) {
      flushParagraph()
      ensureSection().blocks.push({ kind: "subheading", text: subheading[1] })
      continue
    }
    const bullet = line.match(/^[-*•]\s+(.+)$/)
    const numbered = line.match(/^\d+[.)]\s+(.+)$/)
    if (bullet || numbered) {
      flushParagraph()
      ensureSection().blocks.push({
        kind: bullet ? "bullet" : "numbered",
        text: bullet ? bullet[1] : line,
      })
      continue
    }
    paragraph.push(line)
  }
  flushParagraph()
  const orderedIds: CvSection[] = [
    "summary",
    "skills",
    "experience",
    "education",
    "certifications",
    "languages",
    "additional",
  ]
  const ordered = new Map<CvSection, ResumeSection>()
  const other: ResumeSection[] = []
  for (const section of sections.filter((item) => item.blocks.length > 0)) {
    const id =
      section.id === "tools"
        ? "skills"
        : section.id === "projects"
          ? "experience"
          : section.id
    if (id && orderedIds.includes(id)) {
      const existing = ordered.get(id)
      if (existing) existing.blocks.push(...section.blocks)
      else ordered.set(id, { ...section, id })
    } else {
      other.push(section)
    }
  }
  return [
    ...orderedIds.flatMap((id) => (ordered.get(id) ? [ordered.get(id)!] : [])),
    ...other,
  ]
}
