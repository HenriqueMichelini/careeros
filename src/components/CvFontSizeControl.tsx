import { useId } from "react"
import { CV_FONT_MAX, CV_FONT_MIN } from "../lib/cvPreferences"
import { useI18n } from "../lib/store"

export default function CvFontSizeControl({
  fontSize,
  onChange,
  saveError,
}: {
  fontSize: number
  onChange: (value: number) => void
  saveError: boolean
}) {
  const { t } = useI18n()
  const id = useId()
  return (
    <div className="cv-font-controls">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2 text-xs">
        <label
          htmlFor={id}
          className="text-[var(--color-muted-fg)]"
        >
          {t("cv.fontSize")}
        </label>
        <output
          htmlFor={id}
          className="text-[var(--color-muted-fg)]"
        >
          {fontSize} px
        </output>
      </div>
      <input
        id={id}
        type="range"
        min={CV_FONT_MIN}
        max={CV_FONT_MAX}
        step={1}
        value={fontSize}
        aria-valuetext={`${fontSize} px`}
        onChange={(event) => onChange(Number(event.target.value))}
        className="mb-2 w-full accent-[var(--color-accent)]"
      />
      {saveError && (
        <p role="alert" className="mt-2 text-xs text-[var(--color-accent)]">
          {t("cv.saveError")}
        </p>
      )}
    </div>
  )
}
