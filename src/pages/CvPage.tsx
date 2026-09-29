import { useMemo, useState } from 'react'
import { useStore } from '../lib/store'

type Template = 'editorial' | 'minimal' | 'modern'

const TEMPLATES: { id: Template; label: string; note: string }[] = [
  { id: 'editorial', label: 'Editorial', note: 'Structured · accent details' },
  { id: 'minimal', label: 'Minimal', note: 'Quiet · compact' },
  { id: 'modern', label: 'Modern', note: 'Sidebar · skills focus' },
]

function splitLines(value: string) {
  return value.split(/\n|,/).map(item => item.replace(/^[-•*]\s*/, '').trim()).filter(Boolean)
}

function SectionTitle({ children, muted = false }: { children: React.ReactNode; muted?: boolean }) {
  return (
    <h3 className="text-[10px] font-bold uppercase tracking-[0.2em] mb-3" style={{ color: muted ? 'var(--color-muted-fg)' : 'var(--color-accent)' }}>
      {children}
    </h3>
  )
}

export default function CvPage() {
  const { state } = useStore()
  const [template, setTemplate] = useState<Template>('editorial')
  const repo = state.repository
  const skills = useMemo(() => splitLines(repo.skills), [repo.skills])
  const competencies = useMemo(() => splitLines(repo.competencies), [repo.competencies])
  const tools = useMemo(() => splitLines(repo.tools), [repo.tools])
  const projects = repo.projects.filter(project => project.name || project.description)
  const experience = repo.experience.filter(item => item.title || item.company)
  const isModern = template === 'modern'
  const isMinimal = template === 'minimal'
  const name = repo.experience[0]?.title ? 'YOUR NAME' : 'YOUR NAME'

  return (
    <div className="max-w-6xl mx-auto px-6 py-10">
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-6 mb-9">
        <div>
          <p className="text-xs uppercase tracking-[0.25em] mb-3" style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}>
            Step 03 — CV Studio
          </p>
          <h1 className="text-6xl font-bold uppercase tracking-tight leading-[0.9]" style={{ fontFamily: 'var(--font-display)' }}>
            Your CV,<br /><span style={{ color: 'var(--color-accent)' }}>Your Template</span>
          </h1>
          <p className="text-sm max-w-xl mt-4 leading-relaxed" style={{ color: 'var(--color-muted-fg)' }}>
            Shape the master CV that future application templates can build on. This preview uses your Profile details and sample content where information is missing.
          </p>
        </div>
        <div className="flex gap-2 flex-wrap">
          {TEMPLATES.map(item => (
            <button key={item.id} onClick={() => setTemplate(item.id)} className="px-4 py-3 border text-left transition-colors" style={{ borderColor: template === item.id ? 'var(--color-fg)' : 'var(--color-border)', backgroundColor: template === item.id ? 'var(--color-fg)' : 'var(--color-card)', color: template === item.id ? 'var(--color-bg)' : 'var(--color-fg)' }}>
              <span className="block text-xs uppercase tracking-widest" style={{ fontFamily: 'var(--font-mono)' }}>{item.label}</span>
              <span className="block text-[10px] mt-1 opacity-70">{item.note}</span>
            </button>
          ))}
        </div>
      </div>

      <div className="grid lg:grid-cols-[minmax(0,1fr)_260px] gap-8 items-start">
        <article className="min-h-[900px] shadow-sm" style={{ backgroundColor: 'var(--color-card)', border: '1px solid var(--color-border)' }}>
          <div className={`p-8 md:p-12 ${isModern ? 'grid md:grid-cols-[200px_1fr] gap-10' : ''}`}>
            {isModern && (
              <aside className="md:border-r md:pr-7" style={{ borderColor: 'var(--color-border)' }}>
                <div className="w-24 h-24 mb-7 flex items-center justify-center text-3xl font-bold" style={{ backgroundColor: 'var(--color-muted)', color: 'var(--color-accent)', fontFamily: 'var(--font-display)' }}>YN</div>
                <SectionTitle>Contact</SectionTitle>
                <p className="text-xs leading-6" style={{ color: 'var(--color-muted-fg)' }}>name@email.com<br />+1 555 010 2024<br />New York, NY<br />linkedin.com/in/name</p>
                <div className="mt-8"><SectionTitle>Core Skills</SectionTitle><div className="flex flex-wrap gap-1.5">{(skills.length ? skills.slice(0, 8) : ['Leadership', 'Strategy', 'Communication']).map(skill => <span key={skill} className="text-[10px] px-2 py-1" style={{ backgroundColor: 'var(--color-muted)' }}>{skill}</span>)}</div></div>
              </aside>
            )}
            <div className={isModern ? '' : ''}>
              <header className={`pb-7 mb-8 ${isMinimal ? 'border-b' : 'border-b-2'}`} style={{ borderColor: isMinimal ? 'var(--color-border)' : 'var(--color-accent)' }}>
                <p className="text-[10px] uppercase tracking-[0.25em] mb-2" style={{ color: 'var(--color-muted-fg)', fontFamily: 'var(--font-mono)' }}>Curriculum Vitae</p>
                <h2 className="text-5xl md:text-6xl font-bold uppercase tracking-tight leading-none" style={{ fontFamily: 'var(--font-display)' }}>{name}</h2>
                <p className="text-sm mt-3" style={{ color: 'var(--color-muted-fg)' }}>{repo.careerGoals || 'Product-minded leader · Building teams and experiences that move business forward'}</p>
                {!isModern && <p className="text-[10px] mt-5 tracking-wide" style={{ color: 'var(--color-muted-fg)', fontFamily: 'var(--font-mono)' }}>name@email.com&nbsp;&nbsp;·&nbsp;&nbsp;+1 555 010 2024&nbsp;&nbsp;·&nbsp;&nbsp;New York, NY&nbsp;&nbsp;·&nbsp;&nbsp;linkedin.com/in/name</p>}
              </header>

              <section className="mb-8">
                <SectionTitle>Professional Profile</SectionTitle>
                <p className="text-sm leading-7" style={{ color: 'var(--color-fg)' }}>{repo.additionalInfo || repo.careerGoals || 'Strategic professional with a record of turning complex challenges into clear plans, strong partnerships, and measurable results. Known for combining thoughtful leadership with a hands-on approach to delivery.'}</p>
              </section>

              <section className="mb-8">
                <SectionTitle>Experience</SectionTitle>
                {experience.length ? experience.map(item => (
                  <div key={item.id} className="mb-6">
                    <div className="flex flex-wrap justify-between gap-2"><h4 className="text-sm font-semibold">{item.title} <span className="font-normal">· {item.company}</span></h4><span className="text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted-fg)', fontFamily: 'var(--font-mono)' }}>{item.startDate || '2021'} — {item.current ? 'Present' : item.endDate || '2024'}</span></div>
                    <p className="text-xs mt-1 mb-2" style={{ color: 'var(--color-muted-fg)' }}>{item.location || 'New York, NY'}</p>
                    <p className="text-xs leading-6">{item.description || 'Led cross-functional initiatives, aligning team priorities with customer needs and business outcomes.'}</p>
                    <ul className="mt-2 space-y-1">{splitLines(item.achievements || item.responsibilities).slice(0, 3).map((line, index) => <li key={index} className="text-xs leading-5 pl-4 relative before:content-['•'] before:absolute before:left-0" style={{ color: 'var(--color-muted-fg)' }}>{line}</li>)}</ul>
                  </div>
                )) : <div className="mb-6"><div className="flex justify-between gap-2"><h4 className="text-sm font-semibold">Senior Product Manager · Northstar Labs</h4><span className="text-[10px]" style={{ color: 'var(--color-muted-fg)', fontFamily: 'var(--font-mono)' }}>2021 — Present</span></div><p className="text-xs mt-1 mb-2" style={{ color: 'var(--color-muted-fg)' }}>New York, NY</p><p className="text-xs leading-6">Lead product strategy and delivery for a platform serving 2M+ users across 12 markets.</p><ul className="mt-2 space-y-1"><li className="text-xs leading-5 pl-4 relative before:content-['•'] before:absolute before:left-0" style={{ color: 'var(--color-muted-fg)' }}>Grew activation by 28% through onboarding research and iterative experimentation.</li><li className="text-xs leading-5 pl-4 relative before:content-['•'] before:absolute before:left-0" style={{ color: 'var(--color-muted-fg)' }}>Built a cross-functional roadmap that reduced delivery cycle time by 35%.</li></ul></div>}
                {experience.length > 1 && null}
                {!experience.length && <div className="pt-4 border-t" style={{ borderColor: 'var(--color-border)' }}><div className="flex justify-between gap-2"><h4 className="text-sm font-semibold">Product Manager · Fieldwork</h4><span className="text-[10px]" style={{ color: 'var(--color-muted-fg)', fontFamily: 'var(--font-mono)' }}>2018 — 2021</span></div><p className="text-xs leading-6 mt-2" style={{ color: 'var(--color-muted-fg)' }}>Launched a new customer insights program and helped grow annual recurring revenue by 18%.</p></div>}
              </section>

              <div className="grid md:grid-cols-2 gap-8">
                {!isModern && <section><SectionTitle>Skills & Competencies</SectionTitle><p className="text-xs leading-6">{[...skills, ...competencies].slice(0, 12).join(' · ') || 'Product strategy · Team leadership · Data analysis · Stakeholder management · Roadmapping'}</p></section>}
                <section><SectionTitle>Education</SectionTitle><h4 className="text-sm font-semibold">B.S. Business Administration</h4><p className="text-xs mt-1" style={{ color: 'var(--color-muted-fg)' }}>University of California, Berkeley · 2018</p></section>
                {tools.length > 0 && !isModern && <section><SectionTitle>Tools & Technology</SectionTitle><p className="text-xs leading-6">{tools.slice(0, 12).join(' · ')}</p></section>}
                {projects.length > 0 && <section><SectionTitle>Selected Projects</SectionTitle>{projects.slice(0, 2).map(project => <div key={project.id} className="mb-3"><h4 className="text-xs font-semibold">{project.name}</h4><p className="text-xs leading-5 mt-1" style={{ color: 'var(--color-muted-fg)' }}>{project.description || project.highlights}</p></div>)}</section>}
                <section><SectionTitle>Additional</SectionTitle><p className="text-xs leading-6" style={{ color: 'var(--color-muted-fg)' }}>English (Native) · Spanish (Professional)<br />Certified Scrum Product Owner</p></section>
              </div>
            </div>
          </div>
        </article>

        <aside className="lg:sticky lg:top-24">
          <div className="p-5 border" style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-card)' }}>
            <p className="text-xs uppercase tracking-[0.2em] mb-4" style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}>Template Setup</p>
            <div className="space-y-4">
              <div><label className="text-xs block mb-2">Template name</label><input value={`${TEMPLATES.find(item => item.id === template)?.label} CV`} readOnly className="w-full text-xs px-3 py-2 bg-transparent border" style={{ borderColor: 'var(--color-border)' }} /></div>
              <div><label className="text-xs block mb-2">Page format</label><select className="w-full text-xs px-3 py-2 bg-transparent border" style={{ borderColor: 'var(--color-border)', color: 'var(--color-fg)' }}><option>A4 · 2 pages</option><option>Letter · 2 pages</option><option>A4 · 1 page</option></select></div>
              <div><label className="text-xs block mb-2">Accent color</label><div className="flex gap-2">{['#C8102E', '#1D4ED8', '#0F766E', '#111110'].map(color => <button key={color} aria-label={`Accent ${color}`} className="w-7 h-7 border-2" style={{ backgroundColor: color, borderColor: color === '#C8102E' ? 'var(--color-fg)' : 'transparent' }} />)}</div></div>
            </div>
            <button className="w-full mt-6 py-3 text-xs uppercase tracking-[0.16em] border transition-colors hover:opacity-75" style={{ fontFamily: 'var(--font-mono)', borderColor: 'var(--color-fg)', color: 'var(--color-fg)' }}>Save as master template</button>
            <p className="text-[10px] leading-5 mt-3" style={{ color: 'var(--color-muted-fg)' }}>Template setup is a frontend preview. Saving and export will be connected in a later step.</p>
          </div>
          <div className="p-5 mt-4 border" style={{ borderColor: 'var(--color-border)' }}>
            <p className="text-xs uppercase tracking-[0.2em] mb-3" style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}>Profile Coverage</p>
            {[['Experience', experience.length > 0], ['Skills', skills.length > 0], ['Projects', projects.length > 0]].map(([label, complete]) => <div key={label as string} className="flex justify-between py-2 border-b text-xs" style={{ borderColor: 'var(--color-border)' }}><span>{label}</span><span style={{ color: complete ? 'var(--color-fg)' : 'var(--color-muted-fg)' }}>{complete ? 'Added' : 'Sample content'}</span></div>)}
            <p className="text-[10px] leading-5 mt-3" style={{ color: 'var(--color-muted-fg)' }}>Edit your Profile to replace the sample sections with your details.</p>
          </div>
        </aside>
      </div>
    </div>
  )
}
