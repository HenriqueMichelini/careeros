import { useEffect, useRef, useState } from "react"
import { useI18n } from "../lib/store"
import {
  ProfileExportError,
  requestProfileExport,
  type ProfileExportCode,
} from "../lib/profileExport"

// Export only server-retained data, never the editor or browser recovery copy.
export default function ProfileExport({ accountId }: { accountId: string }) {
  const { t } = useI18n()
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<ProfileExportCode | "started" | "">("")
  const [failed, setFailed] = useState(false)
  const generation = useRef(0)
  const request = useRef<AbortController | null>(null)
  useEffect(() => {
    generation.current++
    setBusy(false)
    setMessage("")
    return () => {
      generation.current++
      request.current?.abort()
    }
  }, [accountId])
  async function download() {
    const lease = ++generation.current
    request.current?.abort()
    const controller = new AbortController()
    request.current = controller
    setBusy(true)
    setMessage("")
    setFailed(false)
    let url: string | null = null
    try {
      const blob = await requestProfileExport(accountId, controller.signal)
      if (generation.current !== lease || controller.signal.aborted) return
      url = URL.createObjectURL(blob)
      const anchor = document.createElement("a")
      anchor.href = url
      anchor.download = "careeros-profile-v1.json"
      document.body.append(anchor)
      try {
        anchor.click()
      } finally {
        anchor.remove()
      }
      setMessage("started")
    } catch (error) {
      if (generation.current !== lease) return
      setFailed(true)
      setMessage(
        error instanceof ProfileExportError ? error.code : "unavailable",
      )
    } finally {
      if (url) {
        const release = url
        setTimeout(() => URL.revokeObjectURL(release), 0)
      }
      if (generation.current === lease) setBusy(false)
    }
  }
  return (
    <section className="mt-4 mb-4" data-profile-export>
      <button
        type="button"
        data-profile-export-download
        disabled={busy}
        onClick={download}
        className="underline"
      >
        {t(busy ? "profile.export.pending" : "profile.export.download")}
      </button>
      <p className="text-sm">{t("profile.export.description")}</p>
      {message && (
        <p data-profile-export-message role={failed ? "alert" : "status"}>
          {t(`profile.export.${message}`)}
        </p>
      )}
    </section>
  )
}
