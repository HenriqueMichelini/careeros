declare global {
  interface Window {
    __LAB_SESSION__?: {
      authorization: string
      csrf: string
    }
  }
}
const session = window.__LAB_SESSION__
delete window.__LAB_SESSION__
export async function request<T>(path: string, body?: unknown): Promise<T> {
  if (!session)
    throw new Error("Launch session unavailable. Reload this local Lab.")
  const response = await fetch(`/lab/api/${path}`, {
    method: body === undefined ? "GET" : "POST",
    cache: "no-store",
    credentials: "omit",
    headers: {
      Authorization: session.authorization,
      "X-Lab-CSRF": session.csrf,
      "Content-Type": "application/json",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const value = await response.json()
  if (!response.ok)
    throw new Error(value.message ?? "Local request unavailable.")
  return value as T
}
export async function download(id: string, format: string) {
  if (!session) throw new Error("Launch session unavailable.")
  const response = await fetch(
    `/lab/api/runs/${encodeURIComponent(id)}/exports?format=${format}`,
    {
      cache: "no-store",
      credentials: "omit",
      headers: { Authorization: session.authorization },
    },
  )
  if (!response.ok) throw new Error("Approved synthetic export unavailable.")
  const url = URL.createObjectURL(await response.blob())
  const link = document.createElement("a")
  link.href = url
  link.download = `evaluation-${id}.${format === "markdown" ? "md" : "json"}`
  link.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
export async function bytes(file: File): Promise<string> {
  if (file.size > 8 * 1024 * 1024)
    throw new Error("Each import must be at most 8 MiB.")
  const raw = new Uint8Array(await file.arrayBuffer())
  let binary = ""
  for (let i = 0; i < raw.length; i += 8192)
    binary += String.fromCharCode(...raw.subarray(i, i + 8192))
  return btoa(binary)
}
