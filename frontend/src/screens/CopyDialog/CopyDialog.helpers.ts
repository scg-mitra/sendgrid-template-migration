import type { ExistingPolicy } from "@/api"
import type { ClashPolicy } from "@/screens/CopyDialog/CopyDialog.types"

export const toggleItemClass = "data-[state=on]:border-primary data-[state=on]:bg-primary/10 data-[state=on]:text-primary"

/** The choices for a clashing template, in menu order. */
export const clashPolicies: { value: ClashPolicy; label: string }[] = [
  { value: "skip", label: "Skip" },
  { value: "add", label: "Add as new versions" },
  { value: "duplicate", label: "Create duplicate" },
  { value: "rename", label: "Copy as new name…" },
]

/** The policies "Set all to" offers. Renaming needs a name per template. */
export const bulkPolicies: { value: ExistingPolicy; label: string }[] = [
  { value: "skip", label: "Skip" },
  { value: "add", label: "Add versions" },
  { value: "duplicate", label: "Duplicate" },
]

/** Returns `name (copy)`, or `name (copy 2)` etc. if that is taken. */
export function suggestName(name: string, isTaken: (name: string) => boolean): string {
  for (let n = 1; ; n++) {
    const candidate = n === 1 ? `${name} (copy)` : `${name} (copy ${n})`
    if (!isTaken(candidate)) return candidate
  }
}

/** Explains why a new name can't be used, or returns "". */
export function nameError(name: string, inDestination: boolean, usedByAnotherCopy: boolean): string {
  if (!name.trim()) return "Enter a name."
  if (inDestination) return "This name also exists in the destination."
  if (usedByAnotherCopy) return "Another copy has this name."
  return ""
}
