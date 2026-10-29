import { Field, FieldLabel } from "@/components/ui/field"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { cn } from "@/lib/utils"
import { toggleItemClass } from "@/screens/CopyDialog/CopyDialog.helpers"

export type VersionChoice = "all" | "active"

interface VersionsFieldProps {
  value: VersionChoice
  onChange: (value: VersionChoice) => void
}

/** Whether to copy every version or only the active one. */
export function VersionsField({ value, onChange }: VersionsFieldProps) {
  return (
    <Field>
      <FieldLabel>Versions</FieldLabel>
      <ToggleGroup
        type="single"
        variant="outline"
        value={value}
        onValueChange={(v) => v && onChange(v as VersionChoice)}
        className="w-full"
        aria-label="Versions to copy"
      >
        <ToggleGroupItem value="all" className={cn("flex-1", toggleItemClass)}>
          All versions
        </ToggleGroupItem>
        <ToggleGroupItem value="active" className={cn("flex-1", toggleItemClass)}>
          Active version only
        </ToggleGroupItem>
      </ToggleGroup>
    </Field>
  )
}
