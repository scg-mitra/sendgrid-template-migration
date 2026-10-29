import type { ExistingPolicy, TemplateInfo } from "@/api"

/** What to do with one template whose name already exists in the destination. */
export type ClashPolicy = ExistingPolicy | "rename"

/** The choice for one clashing template. name is used when policy is "rename". */
export type ClashChoice = { policy: ClashPolicy; name: string }

/** A chosen template whose copy's name already exists in the destination. */
export type Clash = {
  template: TemplateInfo
  /** The copy's name before any per-template rename. */
  newName: string
  choice: ClashChoice
  /** Why choice.name can't be used, or "". */
  error: string
}
