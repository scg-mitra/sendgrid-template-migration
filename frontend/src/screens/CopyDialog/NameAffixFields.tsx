import { ArrowRightIcon } from "lucide-react"

import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"

interface NameAffixFieldsProps {
  prefix: string
  suffix: string
  onPrefixChange: (value: string) => void
  onSuffixChange: (value: string) => void
  example: string
}

/** Text to add before and after every template's name, with a preview. */
export function NameAffixFields({ prefix, suffix, onPrefixChange, onSuffixChange, example }: NameAffixFieldsProps) {
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
      {renamed ? (
        // Wraps rather than truncating, so long names and the added text stay
        // visible; pre-wrap shows any spaces typed around the name.
        <div className="space-y-1 rounded-lg bg-muted/50 px-3 py-2 text-sm break-words whitespace-pre-wrap">
          <p className="text-muted-foreground">{example}</p>
          <p className="flex gap-1.5">
            <ArrowRightIcon className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
            <span className="min-w-0 font-medium">
              <mark className="rounded-sm bg-primary/15 text-primary">{prefix}</mark>
              {example}
              <mark className="rounded-sm bg-primary/15 text-primary">{suffix}</mark>
            </span>
          </p>
        </div>
      ) : (
        <FieldDescription>Copies keep their original names. Include any spaces you want between the text and the name.</FieldDescription>
      )}
    </Field>
  )
}
