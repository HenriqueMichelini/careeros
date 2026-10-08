import { useEffect, useState } from "react"

export const CV_PREFERENCES_KEY = "careeros_cv_preferences_v1"
export const CV_FONT_MIN = 12
export const CV_FONT_DEFAULT = 14
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

export const CV_DENSITIES = ["compact", "balanced", "detailed"] as const
export type CvDensity = typeof CV_DENSITIES[number]
export function parseCvDensity(raw: string | null): CvDensity {
  try {
    const value = JSON.parse(raw || "null")
    return CV_DENSITIES.includes(value?.density) ? value.density : "balanced"
  } catch {
    return "balanced"
  }
}

export function useCvPreferences() {
  const [fontSize, setFontSize] = useState(() => {
    try {
      return parseCvFontSize(localStorage.getItem(CV_PREFERENCES_KEY))
    } catch {
      return CV_FONT_DEFAULT
    }
  })
  const [density, setDensity] = useState<CvDensity>(() => {
    try {
      return parseCvDensity(localStorage.getItem(CV_PREFERENCES_KEY))
    } catch {
      return "balanced"
    }
  })
  const [saveError, setSaveError] = useState(false)
  useEffect(() => {
    try {
      localStorage.setItem(
        CV_PREFERENCES_KEY,
        JSON.stringify({ fontSize, density }),
      )
      setSaveError(false)
    } catch {
      setSaveError(true)
    }
  }, [fontSize, density])
  return { fontSize, setFontSize, density, setDensity, saveError }
}
