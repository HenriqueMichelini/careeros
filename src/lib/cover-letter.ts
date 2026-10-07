// A standalone name (including common particles, titles, and credentials) is
// identity text even if it ends with sentence punctuation.
const signatureLine = /^(?:(?:Dr|Dra|Mr|Ms|Mrs|Sr|Sra)\.?\s+)?\p{Lu}[\p{L}'’.-]*(?:\s+(?:\p{Lu}[\p{L}'’.-]*|da|de|do|dos|das|van|von|der))+(?:,\s*\p{Lu}[\p{L}.'’-]*(?:\s+\p{Lu}[\p{L}.'’-]*)*)?$/u

// Only new drafts pass through this boundary. Previously completed letters stay intact.
export function completeCoverLetter(value: unknown, fullName: string): string | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const parts = value as Record<string, unknown>
  if (Object.keys(parts).sort().join(',') !== 'body,closing,greeting') return null
  if (['greeting', 'body', 'closing'].some(key => typeof parts[key] !== 'string' || !(parts[key] as string).trim())) return null
  const greeting = (parts.greeting as string).trim()
  const body = (parts.body as string).trim()
  const closing = (parts.closing as string).trim()
  const name = fullName.trim()
  if (/[\r\n]/.test(greeting) || !['Sincerely,', 'Kind regards,', 'Best regards,', 'Atenciosamente,', 'Cordialmente,'].includes(closing)) return null
  // Reject model signatures instead of guessing which identity should be removed.
  if (body.split('\n').some(line => line.trim() && (!/[.!?]$/.test(line.trim()) || signatureLine.test(line.trim())))) return null
  if (/(?:My name is|Meu nome é|Me chamo)\s+/u.test(body)) return null
  if (name && `${greeting}\n${body}\n${closing}`.includes(name)) return null
  const completed = [greeting, body, closing + (name ? `\n${name}` : '')].join('\n\n')
  if (completed.split(/\s+/).length >= 400) return null
  return completed
}
