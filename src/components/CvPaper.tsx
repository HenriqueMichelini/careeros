import { ReactNode, Ref } from "react"

interface Props {
  label: string
  scale: number
  overflows: boolean
  pageEndLabel: string
  boundaryRef?: Ref<HTMLDivElement>
  contentRef?: Ref<HTMLDivElement>
  children: ReactNode
}

export default function CvPaper({
  label,
  scale,
  overflows,
  pageEndLabel,
  boundaryRef,
  contentRef,
  children,
}: Props) {
  return (
    <article
      aria-label={label}
      className="cv-paper relative mx-auto w-[210mm] border border-[var(--color-border)] bg-[var(--color-card)] shadow-sm"
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
      <div ref={contentRef} className="cv-paper-content relative px-12 py-11">
        {children}
      </div>
    </article>
  )
}
