import {
  emptyProfileView,
  migrateProfile,
  validateProfileDocument,
  type ProfileDocument,
} from "./profileDocument"

export const PROFILE_KEY = "careeros_profile_v2"
export const LEGACY_KEY = "careeros_repo"
export type ProfileStorageCode = "storage" | "validation" | "stale" | "lock"
export class ProfileStorageError extends Error {
  constructor(public code: ProfileStorageCode) {
    super(code)
  }
}
export type ProfileStorage = Pick<Storage, "getItem" | "setItem">
export type ProfileLock = <T>(operation: () => T | Promise<T>) => Promise<T>
// localStorage has no compare-and-swap. All v2 writers share this origin lock;
// environments without Web Locks fail safely instead of risking stale writes.
export const browserProfileLock: ProfileLock = async (operation) => {
  if (!navigator.locks) throw new ProfileStorageError("lock")
  return navigator.locks.request(PROFILE_KEY, { mode: "exclusive" }, operation)
}
function decode(raw: string): ProfileDocument {
  try {
    const candidate: unknown = JSON.parse(raw)
    if (!validateProfileDocument(candidate)) throw new Error()
    return candidate
  } catch {
    throw new ProfileStorageError("validation")
  }
}
function guarded<T>(operation: () => T): T {
  try {
    return operation()
  } catch (error) {
    if (error instanceof ProfileStorageError) throw error
    throw new ProfileStorageError("storage")
  }
}
export function readProfile(storage: ProfileStorage): ProfileDocument {
  return guarded(() => {
    const raw = storage.getItem(PROFILE_KEY)
    if (raw === null) throw new ProfileStorageError("stale")
    return decode(raw)
  })
}
export async function openProfile(
  storage: ProfileStorage,
  lock: ProfileLock = browserProfileLock,
): Promise<ProfileDocument> {
  return lock(() =>
    guarded(() => {
      const current = storage.getItem(PROFILE_KEY)
      if (current !== null) return decode(current)
      const legacy = storage.getItem(LEGACY_KEY)
      let candidate: ProfileDocument
      try {
        candidate = migrateProfile(
          legacy === null ? emptyProfileView() : JSON.parse(legacy),
          crypto.randomUUID(),
        )
      } catch {
        throw new ProfileStorageError("validation")
      }
      if (
        storage.getItem(LEGACY_KEY) !== legacy ||
        storage.getItem(PROFILE_KEY) !== null
      )
        throw new ProfileStorageError("stale")
      // setItem is atomic: authority is the presence of the validated v2 value.
      // The untouched legacy key is the recovery snapshot, never a second writer.
      storage.setItem(PROFILE_KEY, JSON.stringify(candidate))
      return candidate
    }),
  )
}
export async function saveProfile(
  storage: ProfileStorage,
  expected: ProfileDocument,
  candidate: ProfileDocument,
  lock: ProfileLock = browserProfileLock,
): Promise<void> {
  await lock(() =>
    guarded(() => {
      if (
        !validateProfileDocument(expected) ||
        !validateProfileDocument(candidate) ||
        candidate.id !== expected.id ||
        candidate.revision !== expected.revision + 1
      )
        throw new ProfileStorageError("validation")
      const current = readProfile(storage)
      if (JSON.stringify(current) !== JSON.stringify(expected))
        throw new ProfileStorageError("stale")
      storage.setItem(PROFILE_KEY, JSON.stringify(candidate))
    }),
  )
}
