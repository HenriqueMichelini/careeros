import { useEffect, useState } from "react"
import { getInitialLocale } from "../lib/i18n"

const copy = {
  en: {
    title: "Pilot support",
    contact: "Email support",
    feedback: "Share optional feedback",
    status: "Service status",
    unavailable: "Unavailable",
    private: "For account help, use private email. Include only the details needed for your request.",
  },
  "pt-BR": {
    title: "Suporte do piloto",
    contact: "Enviar e-mail ao suporte",
    feedback: "Enviar feedback opcional",
    status: "Status do serviço",
    unavailable: "Indisponível",
    private: "Para ajuda com sua conta, use e-mail privado. Inclua apenas os detalhes necessários para o pedido.",
  },
}

function localStatusOrigin(raw: string | undefined) {
  if (!raw) return null
  try {
    const url = new URL(raw)
    if (url.protocol === "http:" && ["127.0.0.1", "[::1]"].includes(url.hostname) &&
        !url.username && !url.password && url.pathname === "/" &&
        !url.search && !url.hash) return url.origin
  } catch {
    // An invalid configuration leaves the channel unavailable.
  }
  return null
}

export default function PilotSupport() {
  const [locale, setLocale] = useState(getInitialLocale)
  useEffect(() => {
    const observer = new MutationObserver(() => setLocale(getInitialLocale()))
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ["lang"] })
    return () => observer.disconnect()
  }, [])
  const text = copy[locale]
  const origin = localStatusOrigin(import.meta.env.VITE_CAREEROS_PILOT_STATUS_ORIGIN)
  const address: unknown = import.meta.env.VITE_CAREEROS_PILOT_FAKE_SUPPORT_EMAIL
  const email = typeof address === "string" && /^[a-z0-9][a-z0-9._+-]{0,63}@[a-z0-9.-]+\.invalid$/i.test(address)
    ? address
    : null

  return <footer className="mx-auto max-w-6xl border-t px-5 py-6 text-sm" aria-label={text.title}>
    <h2 className="font-semibold">{text.title}</h2>
    <p className="mt-2">{text.private}</p>
    <ul className="mt-3 flex flex-wrap gap-x-5 gap-y-2">
      <li>{email ? <a className="underline" href={`mailto:${email}`}>{text.contact}</a>
        : <span>{text.contact} · {text.unavailable}</span>}</li>
      <li>{email ? <a className="underline" href={`mailto:${email}?subject=${encodeURIComponent(text.feedback)}`}>{text.feedback}</a>
        : <span>{text.feedback} · {text.unavailable}</span>}</li>
      <li>{origin ? <a className="underline" href={`${origin}/status/${locale}`} referrerPolicy="no-referrer">{text.status}</a>
        : <span>{text.status} · {text.unavailable}</span>}</li>
    </ul>
  </footer>
}
