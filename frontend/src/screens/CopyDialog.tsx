import { useState } from "react"
import { ArrowRightIcon, CircleCheckIcon, CopyIcon, TriangleAlertIcon } from "lucide-react"

import type { ExistingPolicy, Session, TemplateInfo } from "@/api"
import type { CopyJob } from "@/screens/CopyScreen"
import { Button } from "@/components/ui/button"
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { plural } from "@/lib/format"
import { cn } from "@/lib/utils"
import { existingChoices, listNames, toggleItemClass } from "@/screens/CopyDialog.helpers"

type CopyDialogProps = {
  session: Session
  chosen: TemplateInfo[]
  destNames: Set<string>
  onCancel: () => void
  onCopy: (job: CopyJob) => void
}

/** Asks how to copy the chosen templates. */
export function CopyDialog({ session, chosen, destNames, onCancel, onCopy }: CopyDialogProps) {
  const [versions, setVersions] = useState<"all" | "active">("all")
  const [prefix, setPrefix] = useState("")
  const [suffix, setSuffix] = useState("")
  const [onExisting, setOnExisting] = useState<ExistingPolicy>("skip")

  // Clashes are checked against the new names, so renaming can avoid them.
  const newName = (name: string) => prefix + name + suffix
  const renamed = prefix !== "" || suffix !== ""
  const clashing = chosen.filter((t) => destNames.has(newName(t.name))).map((t) => newName(t.name))
  const avoided = renamed && clashing.length === 0 ? chosen.filter((t) => destNames.has(t.name)).length : 0
  const example = chosen[0]?.name ?? "Welcome"
  const choice = existingChoices.find((c) => c.value === onExisting)

  return (
    <Dialog open onOpenChange={(open) => !open && onCancel()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            Copy {plural(chosen.length, "template")} to {session.destination}
          </DialogTitle>
          <DialogDescription>Copies get new template IDs, so update any code that uses the old IDs.</DialogDescription>
        </DialogHeader>

        <FieldGroup className="gap-5">
          <Field>
            <FieldLabel>Versions</FieldLabel>
            <ToggleGroup
              type="single"
              variant="outline"
              value={versions}
              onValueChange={(v) => v && setVersions(v as "all" | "active")}
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

          <Field>
            <FieldLabel>Add to each name (optional)</FieldLabel>
            <div className="flex items-center gap-2">
              <Input aria-label="Text to add before each name" placeholder="Before" value={prefix} onChange={(e) => setPrefix(e.target.value)} />
              <span className="shrink-0 rounded-md bg-muted px-2 py-1 text-xs font-medium text-muted-foreground">Template name</span>
              <Input aria-label="Text to add after each name" placeholder="After" value={suffix} onChange={(e) => setSuffix(e.target.value)} />
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

          {clashing.length > 0 && (
            <Field>
              <FieldLabel className="flex items-start gap-2 text-amber-700 dark:text-amber-400">
                <TriangleAlertIcon className="mt-0.5 size-4 shrink-0" />
                {clashing.length === 1
                  ? `"${clashing[0]}" already exists in the destination`
                  : `${clashing.length} names already exist in the destination: ${listNames(clashing)}`}
              </FieldLabel>
              <ToggleGroup
                type="single"
                variant="outline"
                value={onExisting}
                onValueChange={(v) => v && setOnExisting(v as ExistingPolicy)}
                className="w-full flex-wrap"
                aria-label="When the name already exists"
              >
                {existingChoices.map((c) => (
                  <ToggleGroupItem key={c.value} value={c.value} className={cn("flex-1", toggleItemClass)}>
                    {c.label}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
              <FieldDescription>
                {choice?.description} Or add text to each name above to copy them under new names.
              </FieldDescription>
            </Field>
          )}

          {avoided > 0 && (
            <p className="flex items-center gap-2 text-sm text-emerald-700 dark:text-emerald-400">
              <CircleCheckIcon className="size-4 shrink-0" />
              {avoided === 1
                ? "The new names avoid the 1 name that already exists in the destination."
                : `The new names avoid the ${avoided} names that already exist in the destination.`}
            </p>
          )}
        </FieldGroup>

        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">Cancel</Button>
          </DialogClose>
          <Button
            onClick={() =>
              onCopy({
                ids: chosen.map((t) => t.id),
                names: chosen.map((t) => t.name),
                options: { prefix, suffix, activeOnly: versions === "active", onExisting },
              })
            }
          >
            <CopyIcon /> Copy
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
