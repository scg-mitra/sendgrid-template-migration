import { ArrowRightIcon } from "lucide-react"

import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"

interface NameAffixFieldsProps {
  prefix: string
  suffix: string
  onPrefixChange: (value: string) => void
  onSuffixChange: (value: string) => void
  example: string
  newName: (name: string) => string
}

/** Text to add before and after every template's name, with a preview. */
export function NameAffixFields({ prefix, suffix, onPrefixChange, onSuffixChange, example, newName }: NameAffixFieldsProps) {
  const renamed = prefix !== "" || suffix !== ""
  return (
    <Field>
      <FieldLabel>Add to each name (optional)</FieldLabel>
      <div className="flex items-center gap-2">
        <Input
          aria-label="Text to add before each name"
          placeholder="Before"
          value={prefix}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => onPrefixChange(e.target.value)}
        />
        <span className="shrink-0 rounded-md bg-muted px-2 py-1 text-xs font-medium text-muted-foreground">Template name</span>
        <Input
          aria-label="Text to add after each name"
          placeholder="After"
          value={suffix}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => onSuffixChange(e.target.value)}
        />
      </div>
      <FieldDescription className="flex min-w-0 items-center gap-1.5">
        {renamed ? (
          <>
            <span className="truncate">{example}</span>
            <ArrowRightIcon className="size-3.5 shrink-0" />
            <span className="truncate font-medium text-foreground">{newName(example)}</span>
          </>
        ) : (
          "Copies keep their original names. Include any spaces you want between the text and the name."
        )}
      </FieldDescription>
    </Field>
  )
}
