// Browser assertions read the authoritative v2 document through its public view.
export const savedProfileExpression = `(async () => {
  const raw = localStorage.getItem('careeros_profile_v2');
  if (raw === null) return localStorage.getItem('careeros_repo');
  const { profileView } = await import('/src/lib/profileDocument.ts');
  return JSON.stringify(profileView(JSON.parse(raw)));
})()`
