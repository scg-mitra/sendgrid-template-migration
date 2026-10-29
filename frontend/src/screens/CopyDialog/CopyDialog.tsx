import { CopyIcon } from "lucide-react"

import type { Session, TemplateInfo } from "@/api"
import type { CopyJob } from "@/screens/CopyScreen"
import { Button } from "@/components/ui/button"
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { FieldGroup } from "@/components/ui/field"
import { plural } from "@/lib/format"
import { AvoidedNote } from "@/screens/CopyDialog/AvoidedNote"
import { ClashList } from "@/screens/CopyDialog/ClashList"
import { NameAffixFields } from "@/screens/CopyDialog/NameAffixFields"
import { useCopyDialog } from "@/screens/CopyDialog/useCopyDialog"
import { VersionsField } from "@/screens/CopyDialog/VersionsField"

interface CopyDialogProps {
  session: Session
  chosen: TemplateInfo[]
  destNames: Set<string>
  onCancel: () => void
  onCopy: (job: CopyJob) => void
}

/** Asks how to copy the chosen templates. */
export function CopyDialog({ session, chosen, destNames, onCancel, onCopy }: CopyDialogProps) {
  const d = useCopyDialog(chosen, destNames)

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
          <VersionsField value={d.versions} onChange={d.setVersions} />

          <NameAffixFields
            prefix={d.prefix}
            suffix={d.suffix}
            onPrefixChange={d.setPrefix}
            onSuffixChange={d.setSuffix}
            example={chosen[0]?.name ?? "Welcome"}
            newName={d.newName}
          />

          {d.clashes.length > 0 && (
            <ClashList clashes={d.clashes} onPolicyChange={d.setPolicy} onNameChange={d.setName} onSetAll={d.setAll} />
          )}

          {d.avoided > 0 && <AvoidedNote count={d.avoided} />}
        </FieldGroup>

        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">Cancel</Button>
          </DialogClose>
          <Button disabled={!d.valid} onClick={() => onCopy(d.job())}>
            <CopyIcon /> Copy
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
