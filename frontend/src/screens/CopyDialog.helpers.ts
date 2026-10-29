import type { ExistingPolicy } from "@/api"

export const toggleItemClass = "data-[state=on]:border-primary data-[state=on]:bg-primary/10 data-[state=on]:text-primary"

/** What to do when a copy's name already exists in the destination. */
export const existingChoices: { value: ExistingPolicy; label: string; description: string }[] = [
  {
    value: "skip",
    label: "Skip them",
    description: "Leave the existing templates as they are.",
  },
  {
    value: "add",
    label: "Add as new versions",
    description: "Add the versions to the existing templates and make the copied active version active. Their IDs stay the same.",
  },
  {
    value: "duplicate",
    label: "Create duplicates",
    description: "Create another template with the same name alongside each one.",
  },
]

/** Quotes up to three names, e.g. `"A", "B", "C" and 2 more`. */
export function listNames(names: string[]): string {
  const shown = names.slice(0, 3).map((n) => `"${n}"`).join(", ")
  return names.length > 3 ? `${shown} and ${names.length - 3} more` : shown
}
