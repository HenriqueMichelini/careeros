import { useId } from "react"
import { CV_DENSITIES, type CvDensity } from "../lib/cvPreferences"
import { useI18n } from "../lib/store"

export default function CvDensityControl({
  density,
  onChange,
  appliedDensity,
  saveError,
}: {
  density: CvDensity
  onChange: (value: CvDensity) => void
  appliedDensity: CvDensity | null
  saveError: boolean
}) {
  const { t } = useI18n()
  const id = useId()
  return (
    <fieldset
      className="mb-4 border border-[var(--color-border)] p-3"
      data-cv-density
    >
      <legend className="px-1 text-xs font-semibold">{t("cv.density")}</legend>
      <p id={`${id}-help`} className="mb-3 text-xs leading-5">
        {t("cv.densityHelp")}
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
            aria-describedby={`${id}-${value} ${id}-help`}
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
      <p role="status" className="text-xs leading-5" data-cv-density-status>
        {appliedDensity
          ? t("cv.densityApplied", {
              density: t(`cv.density.${appliedDensity}`),
            })
          : t("cv.densityNoSnapshot")}
        {(!appliedDensity || density !== appliedDensity) &&
          ` ${t("cv.densityPending")}`}
      </p>
      {saveError && (
        <p role="alert" className="mt-2 text-xs">
          {t("cv.saveError")}
        </p>
      )}
    </fieldset>
  )
}
