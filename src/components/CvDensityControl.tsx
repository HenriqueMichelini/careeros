import { useId, type ReactNode } from "react"
import { CV_DENSITIES, type CvDensity } from "../lib/cvPreferences"
import { useI18n } from "../lib/store"

export default function CvDensityControl({
  density,
  onChange,
  appliedDensity,
  saveError,
  children,
}: {
  density: CvDensity
  onChange: (value: CvDensity) => void
  appliedDensity: CvDensity | null
  saveError: boolean
  children: ReactNode
}) {
  const { t } = useI18n()
  const id = useId()
  return (
    <div
      className="mb-4 border border-[var(--color-border)] bg-[var(--color-card)] p-5"
      data-cv-density
      data-cv-generation
    >
      <p className="mb-4 text-xs uppercase tracking-[0.2em] text-[var(--color-muted-fg)] [font-family:var(--font-mono)]">
        {t("cv.density")}
      </p>
      {CV_DENSITIES.map((value) => (
        <label
          key={value}
          className="mb-3 flex items-start gap-2 text-xs leading-5"
        >
          <input
            type="radio"
            name={id}
            value={value}
            checked={density === value}
            onChange={() => onChange(value)}
            aria-describedby={`${id}-${value}`}
            className="mt-1"
          />
          <span>
            <strong>{t(`cv.density.${value}`)}</strong>
            <span
              id={`${id}-${value}`}
              className="block text-[var(--color-muted-fg)]"
            >
              {t(`cv.density.${value}Help`)}
            </span>
          </span>
        </label>
      ))}
      {saveError && (
        <p role="alert" className="mb-3 text-xs">
          {t("cv.saveError")}
        </p>
      )}
      {children}
    </div>
  )
}
