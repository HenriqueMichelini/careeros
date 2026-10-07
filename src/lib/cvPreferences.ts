import { useEffect, useState } from "react"

export const CV_PREFERENCES_KEY = "careeros_cv_preferences_v1"
export const CV_FONT_MIN = 12
export const CV_FONT_DEFAULT = 12
export const CV_FONT_MAX = 16

export function parseCvFontSize(raw: string | null): number {
  try {
    const value: unknown = JSON.parse(raw || "null")
    const size =
      value && typeof value === "object" && "fontSize" in value
        ? value.fontSize
        : null
    return typeof size === "number" &&
      Number.isInteger(size) &&
      size >= CV_FONT_MIN &&
      size <= CV_FONT_MAX
      ? size
      : CV_FONT_DEFAULT
  } catch {
    return CV_FONT_DEFAULT
  }
}

export function useCvFontSize() {
  const [fontSize, setFontSize] = useState(() => {
    try {
      return parseCvFontSize(localStorage.getItem(CV_PREFERENCES_KEY))
    } catch {
      return CV_FONT_DEFAULT
    }
  })
  const [saveError, setSaveError] = useState(false)
  useEffect(() => {
    try {
      localStorage.setItem(CV_PREFERENCES_KEY, JSON.stringify({ fontSize }))
      setSaveError(false)
    } catch {
      setSaveError(true)
    }
  }, [fontSize])
  return { fontSize, setFontSize, saveError }
}
