import { useState } from "react"

import type { CopyOverride, ExistingPolicy, TemplateInfo } from "@/api"
import type { CopyJob } from "@/screens/CopyScreen"
import { nameError, suggestName } from "@/screens/CopyDialog/CopyDialog.helpers"
import type { Clash, ClashChoice, ClashPolicy } from "@/screens/CopyDialog/CopyDialog.types"
import type { VersionChoice } from "@/screens/CopyDialog/VersionsField"

const defaultChoice: ClashChoice = { policy: "skip", name: "" }

/** The copy dialog's state: versions, names, and a choice per name clash. */
export function useCopyDialog(chosen: TemplateInfo[], destNames: Set<string>) {
  const [versions, setVersions] = useState<VersionChoice>("all")
  const [prefix, setPrefix] = useState("")
  const [suffix, setSuffix] = useState("")
  const [choices, setChoices] = useState<Record<string, ClashChoice>>({})

  // Clashes are checked against the names after prefix and suffix, so adding
  // text to every name can avoid them.
  const newName = (name: string) => prefix + name + suffix
  const choiceFor = (id: string) => choices[id] ?? defaultChoice
  const clashing = chosen.filter((t) => destNames.has(newName(t.name)))
  const clashIDs = new Set(clashing.map((t) => t.id))

  // The names of the new templates each copy creates, to catch two copies
  // getting the same name.
  const createdName = (t: TemplateInfo) => {
    if (!clashIDs.has(t.id)) return newName(t.name)
    const c = choiceFor(t.id)
    return c.policy === "rename" ? c.name.trim() : ""
  }
  const usedByOthers = (id: string, name: string) => chosen.some((t) => t.id !== id && createdName(t) === name)

  const clashes: Clash[] = clashing.map((t) => {
    const choice = choiceFor(t.id)
    const name = choice.name.trim()
    const error = choice.policy === "rename" ? nameError(name, destNames.has(name), usedByOthers(t.id, name)) : ""
    return { template: t, newName: newName(t.name), choice, error }
  })
  const renamed = prefix !== "" || suffix !== ""
  const avoided = renamed && clashes.length === 0 ? chosen.filter((t) => destNames.has(t.name)).length : 0

  const setPolicy = (t: TemplateInfo, policy: ClashPolicy) => {
    const current = choiceFor(t.id)
    const name =
      policy === "rename" && !current.name
        ? suggestName(newName(t.name), (n) => destNames.has(n) || usedByOthers(t.id, n))
        : current.name
    setChoices((prev) => ({ ...prev, [t.id]: { policy, name } }))
  }
  const setName = (id: string, name: string) =>
    setChoices((prev) => ({ ...prev, [id]: { ...(prev[id] ?? defaultChoice), name } }))
  const setAll = (policy: ExistingPolicy) =>
    setChoices((prev) => Object.fromEntries(clashing.map((t) => [t.id, { ...(prev[t.id] ?? defaultChoice), policy }])))

  const job = (): CopyJob => {
    const overrides: Record<string, CopyOverride> = {}
    for (const { template, choice } of clashes) {
      overrides[template.id] = choice.policy === "rename" ? { name: choice.name.trim() } : { onExisting: choice.policy }
    }
    return {
      ids: chosen.map((t) => t.id),
      names: chosen.map((t) => t.name),
      // Templates that only clash with another copy in this batch are skipped.
      options: { prefix, suffix, activeOnly: versions === "active", onExisting: "skip", overrides },
    }
  }

  return {
    versions, setVersions, prefix, setPrefix, suffix, setSuffix, newName,
    clashes, avoided, setPolicy, setName, setAll,
    valid: clashes.every((c) => !c.error),
    job,
  }
}
