import { useMemo, useState } from "react"
import { ArrowLeftIcon, CopyIcon, SearchIcon, TriangleAlertIcon } from "lucide-react"

import type { Session } from "@/api"
import type { CopyJob } from "@/screens/CopyScreen"
import { Screen, ScreenHeading } from "@/screens/layout"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { cn } from "@/lib/utils"

type Props = {
  session: Session
  onBack: () => void
  onCopy: (job: CopyJob) => void
}

export function SelectScreen({ session, onBack, onCopy }: Props) {
  const [query, setQuery] = useState("")
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [versions, setVersions] = useState<"all" | "active">("all")
  const [confirming, setConfirming] = useState(false)

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return session.templates
    return session.templates.filter((t) => t.name.toLowerCase().includes(q) || t.id.toLowerCase().includes(q))
  }, [session.templates, query])

  const toggle = (id: string, on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (on) next.add(id)
      else next.delete(id)
      return next
    })
  const setMany = (on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev)
      for (const t of filtered) {
        if (on) next.add(t.id)
        else next.delete(t.id)
      }
      return next
    })

  // Keep the list order for copying.
  const chosen = session.templates.filter((t) => selected.has(t.id))
  const duplicates = chosen.filter((t) => t.existsInDest).length

  return (
    <Screen
      actions={
        <>
          <Button variant="outline" onClick={onBack}>
            <ArrowLeftIcon /> Change accounts
          </Button>
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={versions}
            onValueChange={(v) => v && setVersions(v as "all" | "active")}
            aria-label="Versions to copy"
          >
            <ToggleGroupItem value="all" className="data-[state=on]:border-primary data-[state=on]:bg-primary/10 data-[state=on]:text-primary">All versions</ToggleGroupItem>
            <ToggleGroupItem value="active" className="data-[state=on]:border-primary data-[state=on]:bg-primary/10 data-[state=on]:text-primary">Active version only</ToggleGroupItem>
          </ToggleGroup>
          <span className="ml-auto text-sm text-muted-foreground">
            {selected.size} of {session.templates.length} selected
          </span>
          <Button disabled={selected.size === 0} onClick={() => setConfirming(true)}>
            <CopyIcon /> Copy {selected.size || ""} template{selected.size === 1 ? "" : "s"}
          </Button>
        </>
      }
    >
      <ScreenHeading
        title="Choose templates to copy"
        description={
          <>
            The source has <strong className="text-foreground">{session.templates.length}</strong> template(s);{" "}
            {session.destination} has <strong className="text-foreground">{session.destCount}</strong>.
          </>
        }
      />

      <div className="flex gap-2">
        <InputGroup>
          <InputGroupAddon>
            <SearchIcon />
          </InputGroupAddon>
          <InputGroupInput
            placeholder="Search by name or ID"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            autoFocus
          />
        </InputGroup>
        <Button variant="outline" onClick={() => setMany(true)} disabled={filtered.length === 0}>
          Select all
        </Button>
        <Button variant="outline" onClick={() => setMany(false)} disabled={selected.size === 0}>
          Clear
        </Button>
      </div>

      <div className="min-h-40 flex-1 overflow-hidden rounded-xl border">
        {filtered.length === 0 ? (
          <p className="p-8 text-center text-sm text-muted-foreground">
            {session.templates.length === 0 ? "The source account has no templates." : "No templates match your search."}
          </p>
        ) : (
          <ul className="h-full divide-y overflow-y-auto">
            {filtered.map((t) => {
              const checked = selected.has(t.id)
              return (
                <li key={t.id}>
                  <label
                    className={cn(
                      "flex cursor-pointer items-center gap-3 px-4 py-3 transition-colors hover:bg-muted/50",
                      checked && "bg-primary/5",
                    )}
                  >
                    <Checkbox checked={checked} onCheckedChange={(v) => toggle(t.id, v === true)} />
                    <div className="min-w-0 flex-1">
                      <p className="truncate font-medium">{t.name}</p>
                      <p className="truncate font-mono text-xs text-muted-foreground">{t.id}</p>
                    </div>
                    <div className="flex shrink-0 items-center gap-1.5">
                      {t.existsInDest && (
                        <Badge
                          variant="outline"
                          className="border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400"
                          title="A template with this name already exists in the destination"
                        >
                          <TriangleAlertIcon /> Name exists in destination
                        </Badge>
                      )}
                      {t.generation === "legacy" && <Badge variant="secondary">Legacy</Badge>}
                      <Badge variant="outline">
                        {t.versions} version{t.versions === 1 ? "" : "s"}
                      </Badge>
                    </div>
                  </label>
                </li>
              )
            })}
          </ul>
        )}
      </div>

      <AlertDialog open={confirming} onOpenChange={setConfirming}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Copy {chosen.length} template{chosen.length === 1 ? "" : "s"} to {session.destination}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              {versions === "all" ? "All versions" : "Only the active version"} of each template will be copied. The
              copies get new template IDs, so update any code that uses the old IDs.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {duplicates > 0 && (
            <p className="flex gap-2 rounded-lg bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-400">
              <TriangleAlertIcon className="mt-0.5 size-4 shrink-0" />
              {duplicates === 1
                ? "One of them already has a template with the same name in the destination. Copying creates a second one alongside it."
                : `${duplicates} of them already have templates with the same names in the destination. Copying creates second ones alongside them.`}
            </p>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => onCopy({ ids: chosen.map((t) => t.id), names: chosen.map((t) => t.name), activeOnly: versions === "active" })}
            >
              Copy
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Screen>
  )
}
