import { profileFieldKey } from "../lib/profileLabels"
import { useState } from "react"
import { useI18n, useStore } from "../lib/store"
import {
  entityFields,
  profileFields,
  type ProfileFact,
  type ProfileEdit,
} from "../lib/profileDocument"
import type { TranslationKey } from "../lib/i18n"

const sectionFields: Record<string, readonly string[]> = {
  profile: ["fullName", "email", "phone", "location", "professionalLinks"],
  goals: ["careerGoals"],
  skills: ["skills", "competencies", "tools"],
  compensation: ["employmentStatus", "currentSalary", "desiredSalary"],
  other: ["additionalInfo"],
}

export default function FactDetails({ section }: { section: string }) {
  const { profileDocument: doc, editCanonicalProfile } = useStore()
  const { t } = useI18n()
  const [error, setError] = useState(false)
  if (!doc) return null
  const label = (key: string) => t(`profile.facts.${key}` as TranslationKey)
  const entities = doc.entities
    .filter((e) => e.kind === section)
    .sort((a, b) => a.order - b.order)
  const entityLabel = (id: string) => {
    if (id === doc.id) return t("nav.profile")
    const e = doc.entities.find((e) => e.id === id)
    return (
      doc.facts
        .filter(
          (f) =>
            f.owner.id === id &&
            [
              "company",
              "title",
              "name",
              "degree",
              "institution",
              "startDate",
              "endDate",
            ].includes(f.field),
        )
        .map((f) => String(f.value))
        .filter(Boolean)
        .join(" / ") || (e ? t(`repo.section.${e.kind}` as TranslationKey) : "")
    )
  }
  const fieldLabel = (fact: ProfileFact) => {
    const e = doc.entities.find((e) => e.id === fact.owner.id)
    return t(profileFieldKey(e?.kind || "", fact.field))
  }
  const facts = doc.facts
    .filter(
      (f) =>
        entities.some((e) => e.id === f.owner.id) ||
        (f.owner.id === doc.id && sectionFields[section]?.includes(f.field)),
    )
    .sort(
      (a, b) =>
        (doc.entities.find((e) => e.id === a.owner.id)?.order || 0) -
          (doc.entities.find((e) => e.id === b.owner.id)?.order || 0) ||
        a.order - b.order,
    )
  const save = async (edit: ProfileEdit) => {
    setError(!(await editCanonicalProfile(edit)))
  }
  const selectClass =
    "block w-full min-w-0 border border-[var(--color-border)] bg-[var(--color-card)] p-2 text-sm"
  return (
    <details
      className="mt-6 min-w-0 border border-[var(--color-border)] p-4"
      data-fact-details
    >
      <summary className="cursor-pointer text-sm font-semibold">
        {label("title")}
      </summary>
      <p className="my-3 text-xs text-[var(--color-muted-fg)]">
        {label("guide")}
      </p>
      {error && (
        <p role="alert" className="mb-3 text-sm">
          {label("error")}
        </p>
      )}
      {entities.length > 1 && (
        <div className="mb-4 space-y-2">
          {entities.map((entity, index) => (
            <div
              key={entity.id}
              className="flex items-center justify-between gap-2 text-sm"
            >
              <span className="min-w-0 break-words">
                {entityLabel(entity.id)}
              </span>
              <span className="flex shrink-0 gap-2">
                {[-1, 1].map((direction) => (
                  <button
                    key={direction}
                    type="button"
                    disabled={
                      index + direction < 0 ||
                      index + direction >= entities.length
                    }
                    aria-label={`${label(direction < 0 ? "up" : "down")}: ${entityLabel(entity.id)}`}
                    className="border px-2 disabled:opacity-30"
                    onClick={() => {
                      const ids = entities.map((e) => e.id)
                      ;[ids[index], ids[index + direction]] = [
                        ids[index + direction],
                        ids[index],
                      ]
                      void save({ type: "reorder", kind: entity.kind, ids })
                    }}
                  >
                    {direction < 0 ? "↑" : "↓"}
                  </button>
                ))}
              </span>
            </div>
          ))}
        </div>
      )}
      <div className="space-y-3">
        {facts.map((fact) => (
          <details
            key={fact.id}
            data-fact-id={fact.id}
            className="min-w-0 border border-[var(--color-border)] p-3"
          >
            <summary className="cursor-pointer break-words text-sm">
              {fieldLabel(fact)} · {entityLabel(fact.owner.id)}
            </summary>
            <p className="my-2 whitespace-pre-wrap break-words text-sm">
              {typeof fact.value === "string"
                ? fact.value
                : JSON.stringify(fact.value)}
            </p>
            <p className="mb-3 text-xs text-[var(--color-muted-fg)]">
              {label(fact.kind)} · {label(fact.origin.kind)} ·{" "}
              {label(fact.support)}
            </p>
            {fact.context.length > 0 && (
              <ul className="mb-3 space-y-1 text-xs">
                {fact.context.map((context) => (
                  <li
                    key={context.id}
                    className="flex flex-wrap items-center gap-2 break-words"
                  >
                    <span>
                      {label("context")}: {entityLabel(context.id)}
                    </span>
                    <button
                      type="button"
                      className="underline"
                      onClick={() =>
                        void save({
                          type: "remove_context",
                          id: fact.id,
                          targetId: context.id,
                        })
                      }
                    >
                      {label("removeContext")}
                    </button>
                  </li>
                ))}
              </ul>
            )}
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              {([
                ["assertion", ["unknown", "affirmed", "negated"]],
                ["intent", ["unknown", "actual", "aspiration"]],
                ["certainty", ["unknown", "certain", "uncertain"]],
              ] as const).map(([key, values]) => (
                <label key={key} className="text-xs">
                  {label(key)}
                  <select
                    aria-label={label(key)}
                    className={selectClass}
                    value={fact[key]}
                    onChange={(e) =>
                      void save({
                        type: "fact",
                        id: fact.id,
                        patch: { [key]: e.target.value },
                      })
                    }
                  >
                    {values.map((v) => (
                      <option key={v} value={v}>
                        {label(v)}
                      </option>
                    ))}
                  </select>
                </label>
              ))}
              <label className="text-xs">
                {label("precision")}
                <select
                  aria-label={label("precision")}
                  className={selectClass}
                  value={fact.temporal.precision}
                  onChange={(e) =>
                    void save({
                      type: "fact",
                      id: fact.id,
                      patch: {
                        temporal: {
                          ...fact.temporal,
                          precision: e.target
                            .value as ProfileFact["temporal"]["precision"],
                        },
                      },
                    })
                  }
                >
                  {["unknown", "exact", "approximate"].map((v) => (
                    <option key={v} value={v}>
                      {label(v)}
                    </option>
                  ))}
                </select>
              </label>
              <label className="text-xs">
                {label("temporal")}
                <input
                  aria-label={label("temporal")}
                  className={selectClass}
                  value={fact.temporal.wording}
                  onChange={(e) =>
                    void save({
                      type: "fact",
                      id: fact.id,
                      patch: {
                        temporal: { ...fact.temporal, wording: e.target.value },
                      },
                    })
                  }
                />
              </label>
              {([
                "role_context",
                "project_context",
                "period_context",
              ] as const).map((kind) => (
                <label key={kind} className="text-xs">
                  {label(kind)}
                  <select
                    aria-label={label(kind)}
                    className={selectClass}
                    value={
                      doc.links.find(
                        (l) =>
                          l.from.id === fact.id &&
                          l.kind === kind &&
                          l.state === "active",
                      )?.to.id || ""
                    }
                    onChange={(e) =>
                      void save({
                        type: "context",
                        id: fact.id,
                        kind,
                        targetId: e.target.value || null,
                      })
                    }
                  >
                    <option value="">{label("none")}</option>
                    {doc.entities
                      .filter(
                        (e) =>
                          kind === "period_context" ||
                          e.kind ===
                            (kind === "role_context"
                              ? "experience"
                              : "projects"),
                      )
                      .map((e) => (
                        <option key={e.id} value={e.id}>
                          {entityLabel(e.id)}
                        </option>
                      ))}
                  </select>
                </label>
              ))}
              <label className="text-xs">
                {label("move")}
                <select
                  aria-label={label("move")}
                  className={selectClass}
                  value=""
                  onChange={(e) => {
                    if (e.target.value)
                      void save({
                        type: "move_fact",
                        id: fact.id,
                        ownerId: e.target.value,
                        field: fact.field,
                      })
                  }}
                >
                  <option value="">{entityLabel(fact.owner.id)}</option>
                  {[
                    { id: doc.id, fields: profileFields },
                    ...doc.entities.map((e) => ({
                      id: e.id,
                      fields: entityFields[e.kind],
                    })),
                  ]
                    .filter(
                      (owner) =>
                        owner.id !== fact.owner.id &&
                        (owner.fields as readonly string[]).includes(
                          fact.field,
                        ) &&
                        !doc.facts.some(
                          (f) =>
                            f.owner.id === owner.id && f.field === fact.field,
                        ),
                    )
                    .map((owner) => (
                      <option key={owner.id} value={owner.id}>
                        {entityLabel(owner.id)}
                      </option>
                    ))}
                </select>
              </label>
            </div>
            {doc.links
              .filter((l) => l.from.id === fact.id && l.kind === "supports")
              .map((link) => {
                const evidence = doc.evidence.find((e) => e.id === link.to.id)
                return (
                  evidence && (
                    <blockquote
                      key={link.id}
                      className="mt-3 break-words border-l-2 pl-3 text-xs"
                    >
                      {evidence.excerpt}
                      <br />
                      {evidence.origin} · {label(link.state)}
                    </blockquote>
                  )
                )
              })}
            <button
              type="button"
              className="mt-3 text-xs text-[var(--color-accent)] underline"
              onClick={() => void save({ type: "remove_fact", id: fact.id })}
            >
              {label("remove")}
            </button>
          </details>
        ))}
      </div>
    </details>
  )
}
