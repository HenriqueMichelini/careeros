export type ProfileExportCode = "authentication" | "access" | "missing" | "account_changed" | "unavailable" | "format"
export class ProfileExportError extends Error {
  constructor(readonly code: ProfileExportCode) {
    super(code)
  }
}

// Parse only for version/shape compatibility; deliver the original Blob. In
// particular opaque numeric values must not pass through JSON.stringify.
export async function requestProfileExport(
  accountId: string,
  signal: AbortSignal,
  request: typeof fetch = fetch,
): Promise<Blob> {
  let response: Response
  try {
    response = await request("/api/profile/export", {
      method: "GET",
      credentials: "same-origin",
      cache: "no-store",
      headers: { "X-CareerOS-Account": accountId },
      signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
    })
  } catch {
    throw new ProfileExportError("unavailable")
  }
  if (!response.ok) {
    const codes: Record<number, ProfileExportCode> = {
      401: "authentication",
      403: "access",
      404: "missing",
      409: "account_changed",
    }
    throw new ProfileExportError(codes[response.status] ?? "unavailable")
  }
  try {
    const blob = await response.blob()
    const value = JSON.parse(await blob.text())
    if (
      response.headers.get("Content-Type")?.split(";")[0] !==
        "application/json" ||
      value.format !== "careeros-profile-export" ||
      value.version !== 1 ||
      value.profile?.version !== 2 ||
      typeof value.profile.id !== "string" ||
      !Number.isSafeInteger(value.profile.revision)
    )
      throw new Error()
    return blob
  } catch {
    throw new ProfileExportError("format")
  }
}
