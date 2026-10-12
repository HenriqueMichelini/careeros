// Compare parsed JSON values. Object key order is immaterial; array order and
// every key/value remain significant. This does not establish factual support.
export function equalJsonValue(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (
    a === null ||
    b === null ||
    typeof a !== "object" ||
    typeof b !== "object"
  )
    return false
  if (Array.isArray(a) || Array.isArray(b))
    return (
      Array.isArray(a) &&
      Array.isArray(b) &&
      a.length === b.length &&
      a.every((value, index) => equalJsonValue(value, b[index]))
    )
  const left = a as Record<string, unknown>
  const right = b as Record<string, unknown>
  const keys = Object.keys(left)
  return (
    keys.length === Object.keys(right).length &&
    keys.every(
      (key) =>
        Object.prototype.hasOwnProperty.call(right, key) &&
        equalJsonValue(left[key], right[key]),
    )
  )
}
