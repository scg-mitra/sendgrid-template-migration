import { useMemo, useRef, useState, type MouseEvent } from "react"
import { ArrowLeftIcon, CopyIcon, DownloadIcon, EyeIcon, SearchIcon, TriangleAlertIcon } from "lucide-react"
import { toast } from "sonner"

import { Backend, Dialogs, backupFilter, errorMessage, isCancelled, type ExistingPolicy, type Session, type TemplateInfo } from "@/api"
import type { CopyJob } from "@/screens/CopyScreen"
import { PreviewDialog } from "@/screens/PreviewDialog"
import { Screen, ScreenHeading } from "@/screens/layout"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Toggle } from "@/components/ui/toggle"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { plural } from "@/lib/format"
import { cn } from "@/lib/utils"

type Props = {
  session: Session
  selected: Set<string>
  onSelectedChange: (selected: Set<string>) => void
  onBack: () => void
  onCopy: (job: CopyJob) => void
}

type Generation = "all" | "dynamic" | "legacy"

const toggleItemClass = "data-[state=on]:border-primary data-[state=on]:bg-primary/10 data-[state=on]:text-primary"

export function SelectScreen({ session, selected, onSelectedChange, onBack, onCopy }: Props) {
  const [query, setQuery] = useState("")
  const [hideExisting, setHideExisting] = useState(false)
  const [generation, setGeneration] = useState<Generation>("all")
  const [confirming, setConfirming] = useState(false)
  const [previewing, setPreviewing] = useState<TemplateInfo | null>(null)
  const [exporting, setExporting] = useState(false)
  const lastClicked = useRef<number | null>(null)

  const destNames = useMemo(() => new Set(session.destNames), [session.destNames])
  const hasLegacy = session.templates.some((t) => t.generation === "legacy")

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return session.templates.filter(
      (t) =>
        (!q || t.name.toLowerCase().includes(q) || t.id.toLowerCase().includes(q)) &&
        (!hideExisting || !destNames.has(t.name)) &&
        (generation === "all" || (t.generation || "dynamic") === generation),
    )
  }, [session.templates, query, hideExisting, generation, destNames])

  const shownSelected = filtered.filter((t) => selected.has(t.id)).length
  const allShownState = shownSelected === 0 ? false : shownSelected === filtered.length ? true : "indeterminate"

  const setMany = (ids: string[], on: boolean) => {
    const next = new Set(selected)
    for (const id of ids) {
      if (on) next.add(id)
      else next.delete(id)
    }
    onSelectedChange(next)
  }

  // Clicking a row toggles it; shift-clicking sets every row from the last
  // clicked one to match.
  const clickRow = (index: number, e: MouseEvent) => {
    const on = !selected.has(filtered[index].id)
    const from = e.shiftKey && lastClicked.current !== null ? Math.min(lastClicked.current, index) : index
    const to = e.shiftKey && lastClicked.current !== null ? Math.max(lastClicked.current, index) : index
    setMany(
      filtered.slice(from, to + 1).map((t) => t.id),
      on,
    )
    lastClicked.current = index
  }

  // Keep the list order for copying.
  const chosen = session.templates.filter((t) => selected.has(t.id))

  async function exportChosen() {
    let path: string
    try {
      path = await Dialogs.SaveFile({
        Title: "Save template backup",
        Filename: "sendgrid-templates.json",
        Filters: [backupFilter],
        CanCreateDirectories: true,
      })
    } catch (err) {
      toast.error(errorMessage(err))
      return
    }
    if (!path) return
    if (!/\.json$/i.test(path)) path += ".json"

    setExporting(true)
    try {
      const n = await Backend.ExportTemplates(
        chosen.map((t) => t.id),
        path,
      )
      toast.success(`Saved ${plural(n, "template")} to ${path}`)
    } catch (err) {
      if (!isCancelled(err)) toast.error(errorMessage(err))
    } finally {
      setExporting(false)
    }
  }

  const count = selected.size
  return (
    <Screen
      actions={
        <>
          <Button variant="outline" onClick={onBack}>
            <ArrowLeftIcon /> Change accounts
          </Button>
          <span className="ml-auto text-sm text-muted-foreground">
            {count} of {session.templates.length} selected
          </span>
          {session.exportOnly ? (
            <Button disabled={count === 0 || exporting} onClick={exportChosen}>
              {exporting ? <Spinner /> : <DownloadIcon />} Save {count || ""} to file…
            </Button>
          ) : (
            <>
              <Button variant="outline" disabled={count === 0 || exporting} onClick={exportChosen}>
                {exporting ? <Spinner /> : <DownloadIcon />} Back up…
              </Button>
              <Button disabled={count === 0} onClick={() => setConfirming(true)}>
                <CopyIcon /> Copy {count ? plural(count, "template") : "templates"}
              </Button>
            </>
          )}
        </>
      }
    >
      <ScreenHeading
        title={session.exportOnly ? "Choose templates to back up" : "Choose templates to copy"}
        description={
          <>
            {capitalize(session.source)} has <strong className="text-foreground">{plural(session.templates.length, "template")}</strong>
            {!session.exportOnly && (
              <>
                ; {session.destination} has <strong className="text-foreground">{session.destCount}</strong>
              </>
            )}
            .
          </>
        }
      />

      <div className="flex flex-wrap gap-2">
        <InputGroup className="min-w-48 flex-1">
          <InputGroupAddon>
            <SearchIcon />
          </InputGroupAddon>
          <InputGroupInput placeholder="Search by name or ID" value={query} onChange={(e) => setQuery(e.target.value)} autoFocus />
        </InputGroup>
        {hasLegacy && (
          <Select value={generation} onValueChange={(v) => setGeneration(v as Generation)}>
            <SelectTrigger className="w-36" aria-label="Template type">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All types</SelectItem>
              <SelectItem value="dynamic">Dynamic</SelectItem>
              <SelectItem value="legacy">Legacy</SelectItem>
            </SelectContent>
          </Select>
        )}
        {!session.exportOnly && session.destNames.length > 0 && (
          <Toggle variant="outline" pressed={hideExisting} onPressedChange={setHideExisting} className={toggleItemClass}>
            Hide ones already in destination
          </Toggle>
        )}
      </div>

      <div className="flex min-h-40 flex-1 flex-col overflow-hidden rounded-xl border">
        {filtered.length > 0 && (
          <div className="flex items-center gap-3 border-b bg-muted/40 px-4 py-2 text-sm">
            <Checkbox
              checked={allShownState}
              onCheckedChange={() =>
                setMany(
                  filtered.map((t) => t.id),
                  allShownState !== true,
                )
              }
              aria-label={allShownState === true ? "Deselect all shown templates" : "Select all shown templates"}
            />
            <span className="text-muted-foreground">
              {filtered.length === session.templates.length
                ? "Select all"
                : `Select all ${filtered.length} shown`}
            </span>
            <span className="ml-auto hidden text-xs text-muted-foreground sm:inline">Shift-click to select a range</span>
          </div>
        )}
        {filtered.length === 0 ? (
          <p className="p-8 text-center text-sm text-muted-foreground">
            {session.templates.length === 0 ? "The source has no templates." : "No templates match your search and filters."}
          </p>
        ) : (
          <ul className="min-h-0 flex-1 divide-y overflow-y-auto">
            {filtered.map((t, i) => {
              const checked = selected.has(t.id)
              const exists = !session.exportOnly && destNames.has(t.name)
              return (
                <li
                  key={t.id}
                  onClick={(e) => clickRow(i, e)}
                  className={cn(
                    "flex cursor-pointer items-center gap-3 px-4 py-3 transition-colors select-none hover:bg-muted/50",
                    checked && "bg-primary/5",
                  )}
                >
                  <Checkbox
                    checked={checked}
                    aria-label={`Select ${t.name}`}
                    // The click (or Space) bubbles to the row, which does the
                    // toggling, so shift-click works on the box too.
                    onClick={(e) => e.preventDefault()}
                  />
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-medium">{t.name}</p>
                    <p className="truncate font-mono text-xs text-muted-foreground">{t.id}</p>
                  </div>
                  <div className="flex shrink-0 items-center gap-1.5">
                    {exists && (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Badge
                            variant="outline"
                            tabIndex={0}
                            className="border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400"
                          >
                            <TriangleAlertIcon /> <span className="hidden md:inline">Name exists in destination</span>
                            <span className="md:hidden">Exists</span>
                          </Badge>
                        </TooltipTrigger>
                        <TooltipContent>
                          {capitalize(session.destination)} already has a template named "{t.name}". You can choose what to do
                          when you copy.
                        </TooltipContent>
                      </Tooltip>
                    )}
                    {t.generation === "legacy" && <Badge variant="secondary">Legacy</Badge>}
                    <Badge variant="outline">{plural(t.versions, "version")}</Badge>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={`Preview ${t.name}`}
                          disabled={t.versions === 0}
                          onClick={(e) => {
                            e.stopPropagation()
                            setPreviewing(t)
                          }}
                        >
                          <EyeIcon />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>Preview</TooltipContent>
                    </Tooltip>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </div>

      {previewing && <PreviewDialog key={previewing.id} template={previewing} onClose={() => setPreviewing(null)} />}

      {confirming && (
        <CopyDialog
          session={session}
          chosen={chosen}
          destNames={destNames}
          onCancel={() => setConfirming(false)}
          onCopy={(job) => {
            setConfirming(false)
            onCopy(job)
          }}
        />
      )}
    </Screen>
  )
}

type CopyDialogProps = {
  session: Session
  chosen: TemplateInfo[]
  destNames: Set<string>
  onCancel: () => void
  onCopy: (job: CopyJob) => void
}

const existingChoices: { value: ExistingPolicy; label: string; description: string }[] = [
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

/** Asks how to copy the chosen templates. */
function CopyDialog({ session, chosen, destNames, onCancel, onCopy }: CopyDialogProps) {
  const [versions, setVersions] = useState<"all" | "active">("all")
  const [prefix, setPrefix] = useState("")
  const [suffix, setSuffix] = useState("")
  const [onExisting, setOnExisting] = useState<ExistingPolicy>("skip")

  const newName = (name: string) => prefix + name + suffix
  const duplicates = chosen.filter((t) => destNames.has(newName(t.name))).length
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
            <FieldLabel>Rename copies (optional)</FieldLabel>
            <div className="flex items-center gap-2">
              <Input aria-label="Add before the name" placeholder="Before" value={prefix} onChange={(e) => setPrefix(e.target.value)} />
              <span className="shrink-0 text-sm text-muted-foreground">name</span>
              <Input aria-label="Add after the name" placeholder="After" value={suffix} onChange={(e) => setSuffix(e.target.value)} />
            </div>
            <FieldDescription className="truncate">
              e.g. "{newName(example)}"
            </FieldDescription>
          </Field>

          {duplicates > 0 && (
            <Field>
              <FieldLabel className="flex items-center gap-2 text-amber-700 dark:text-amber-400">
                <TriangleAlertIcon className="size-4" />
                {duplicates === 1
                  ? "1 name already exists in the destination"
                  : `${duplicates} names already exist in the destination`}
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
              <FieldDescription>{choice?.description}</FieldDescription>
            </Field>
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

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}
