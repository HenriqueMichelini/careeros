import { CSSProperties, ReactNode, Ref } from "react"

interface Props {
  label: string
  fontSize?: number
  scale: number
  overflows: boolean
  pageEndLabel: string
  boundaryRef?: Ref<HTMLDivElement>
  contentRef?: Ref<HTMLDivElement>
  children: ReactNode
  className?: string
}

export default function CvPaper({
  label,
  fontSize = 14,
  scale,
  overflows,
  pageEndLabel,
  boundaryRef,
  contentRef,
  children,
  className = "",
}: Props) {
  return (
    <article
      aria-label={label}
      className={`cv-paper relative mx-auto w-[210mm] border border-[var(--color-border)] bg-[var(--color-card)] shadow-sm ${className}`}
      style={{ zoom: scale }}
    >
      <div
        ref={boundaryRef}
        aria-hidden="true"
        className="cv-page-boundary pointer-events-none absolute left-0 top-0 w-full"
      >
        {overflows && (
          <span className="absolute bottom-0 right-0 bg-[var(--color-accent)] px-2 py-1 text-[10px] font-semibold text-white">
            {pageEndLabel}
          </span>
        )}
      </div>
      <div
        ref={contentRef}
        className="cv-paper-content relative px-12 py-11"
        style={{ "--cv-font-size": `${fontSize}px` } as CSSProperties}
      >
        {children}
      </div>
    </article>
  )
}
