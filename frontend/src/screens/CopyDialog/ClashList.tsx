import { TriangleAlertIcon } from "lucide-react"

import type { ExistingPolicy, TemplateInfo } from "@/api"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { bulkPolicies } from "@/screens/CopyDialog/CopyDialog.helpers"
import type { Clash, ClashPolicy } from "@/screens/CopyDialog/CopyDialog.types"
import { ClashRow } from "@/screens/CopyDialog/ClashRow"

interface ClashListProps {
  clashes: Clash[]
  onPolicyChange: (template: TemplateInfo, policy: ClashPolicy) => void
  onNameChange: (id: string, name: string) => void
  onSetAll: (policy: ExistingPolicy) => void
}

/** The templates whose names already exist, each with its own choice. */
export function ClashList({ clashes, onPolicyChange, onNameChange, onSetAll }: ClashListProps) {
  return (
    <Field>
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
        <FieldLabel className="flex items-center gap-2 text-amber-700 dark:text-amber-400">
          <TriangleAlertIcon className="size-4 shrink-0" />
          {clashes.length === 1
            ? "1 name already exists in the destination"
            : `${clashes.length} names already exist in the destination`}
        </FieldLabel>
        {clashes.length > 1 && (
          <div className="flex items-center gap-1 text-xs text-muted-foreground">
            Set all to
            {bulkPolicies.map((p) => (
              <Button key={p.value} variant="ghost" size="xs" onClick={() => onSetAll(p.value)}>
                {p.label}
              </Button>
            ))}
          </div>
        )}
      </div>
      <ul className="max-h-52 divide-y overflow-y-auto rounded-lg border">
        {clashes.map((c) => (
          <ClashRow key={c.template.id} clash={c} onPolicyChange={onPolicyChange} onNameChange={onNameChange} />
        ))}
      </ul>
      <FieldDescription>
        Adding versions keeps the existing template's ID and makes the copied active version active. Or add text to each
        name above to copy them all under new names.
      </FieldDescription>
    </Field>
  )
}
