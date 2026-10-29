export function csrfHeaders(): Record<string, string> {
  const token = document.cookie
    .split(";")
    .map((v) => v.trim())
    .find((v) => v.startsWith("__Host-careeros_csrf="))
    ?.split("=")[1]
  return token ? { "X-CSRF-Token": decodeURIComponent(token) } : {}
}

// Capture the workspace identity in each caller. It is only a consistency
// precondition; the server obtains ownership from the authenticated session.
export function workflowHeaders(
  accountId?: string | null,
): Record<string, string> {
  return accountId ? { "X-CareerOS-Account": accountId, ...csrfHeaders() } : {}
}
