import { useEffect, useRef, useState } from "react"
import {
  CircleCheckIcon,
  CircleMinusIcon,
  CircleSlashIcon,
  CircleXIcon,
  ClipboardCopyIcon,
  CopyIcon,
  FileDownIcon,
  ListRestartIcon,
  RotateCcwIcon,
  RefreshCwIcon,
  XIcon,
} from "lucide-react"
import { toast } from "sonner"

import {
  Backend,
  Clipboard,
  CopyAction,
  CopyProgressEvent,
  Dialogs,
  Events,
  errorMessage,
  isCancelled,
  type CopyOptions,
  type CopyResult,
} from "@/api"
import { Screen, ScreenHeading } from "@/screens/layout"
import { Button } from "@/components/ui/button"
import { Progress } from "@/components/ui/progress"
import { Spinner } from "@/components/ui/spinner"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { plural } from "@/lib/format"

/** The templates the user chose to copy, and how. */
export type CopyJob = {
  ids: string[]
  names: string[]
  options: CopyOptions
}

/** How many templates the backend copies at once (copyConcurrency in copy.go). */
const concurrency = 4

/** The error the backend reports for a template it didn't copy because of cancelling. */
const cancelledError = "Cancelled"

type Props = {
  job: CopyJob
  /** Called once the copy has finished, with the names of the templates it created. */
  onFinished: (createdNames: string[]) => void
  /** Go back to the template list. */
  onCopyMore: () => void
  /** Copy these templates again, with the same options. */
  onRetry: (ids: string[]) => void
  onStartOver: () => void
  onRunningChange: (running: boolean) => void
}

export function CopyScreen({ job, onFinished, onCopyMore, onRetry, onStartOver, onRunningChange }: Props) {
  const [results, setResults] = useState<(CopyResult | undefined)[]>(() => job.ids.map(() => undefined))
  const [finished, setFinished] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const [fatal, setFatal] = useState("")
  const call = useRef<ReturnType<typeof Backend.CopyTemplates> | null>(null)
  const reported = useRef(false)

  useEffect(() => {
    const off = Events.On(CopyProgressEvent, (ev) => {
      const { index, result } = ev.data
      setResults((prev) => prev.map((r, i) => (i === index ? result : r)))
    })
    // Start once, even though React's strict mode runs effects twice in development.
    if (!call.current) {
      onRunningChange(true)
      call.current = Backend.CopyTemplates(job.ids, job.options)
      call.current
        .then((all) => setResults(job.ids.map((_, i) => all[i])))
        .catch((err) => {
          if (!isCancelled(err)) setFatal(errorMessage(err))
        })
        .finally(() => {
          setFinished(true)
          onRunningChange(false)
        })
    }
    return off
  }, [job, onRunningChange])

  const done = results.filter(Boolean).length
  const succeeded = results.filter((r): r is CopyResult => !!r && !r.error && r.action !== CopyAction.Skipped)
  const skipped = results.filter((r) => r?.action === CopyAction.Skipped).length
  const failed = results.filter((r) => r?.error && r.error !== cancelledError).length
  const unfinished = job.ids.filter((_, i) => !results[i] || results[i]?.error)
  const createdNames = succeeded.filter((r) => r.action === CopyAction.Created).map((r) => r.newName ?? r.name)

  useEffect(() => {
    if (finished && !reported.current) {
      reported.current = true
      onFinished(createdNames)
    }
  }, [finished, createdNames, onFinished])

  // Templates are started in list order, a few at a time, so the first few
  // without a result are the ones in progress.
  const inProgress = new Set<number>()
  if (!finished && !cancelling) {
    for (let i = 0; i < results.length && inProgress.size < concurrency; i++) {
      if (!results[i]) inProgress.add(i)
    }
  }

  const copyText = async (text: string, what: string) => {
    try {
      await Clipboard.SetText(text)
      toast.success(`${what} copied to the clipboard`)
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }

  const saveCSV = async () => {
    try {
      let path = await Dialogs.SaveFile({
        Title: "Save results",
        Filename: "template-ids.csv",
        Filters: [{ DisplayName: "CSV files (*.csv)", Pattern: "*.csv" }],
        CanCreateDirectories: true,
      })
      if (!path) return
      if (!/\.csv$/i.test(path)) path += ".csv"
      await Backend.SaveResultsCSV(
        results.filter((r): r is CopyResult => !!r),
        path,
      )
      toast.success(`Saved to ${path}`)
    } catch (err) {
      if (!isCancelled(err)) toast.error(errorMessage(err))
    }
  }

  let title = `Copying ${plural(job.ids.length, "template")}…`
  if (cancelling && !finished) title = "Cancelling…"
  if (finished) {
    if (cancelling) title = "Copy cancelled"
    else if (failed || fatal) title = "Finished with errors"
    else title = "All done"
  }

  return (
    <Screen
      actions={
        <>
          <Button
            variant="outline"
            title="Copy name, old ID, new name and new ID for each copied template"
            disabled={succeeded.length === 0}
            onClick={() =>
              copyText(
                ["Name\tOld ID\tNew name\tNew ID", ...succeeded.map((r) => `${r.name}\t${r.id}\t${r.newName}\t${r.newID}`)].join("\n"),
                "ID list",
              )
            }
          >
            <ClipboardCopyIcon /> Copy IDs
          </Button>
          <Button variant="outline" disabled={!finished || done === 0} onClick={saveCSV}>
            <FileDownIcon /> Save CSV…
          </Button>
          <div className="ml-auto flex gap-3">
            {!finished ? (
              <Button
                variant="outline"
                disabled={cancelling}
                onClick={() => {
                  setCancelling(true)
                  call.current?.cancel()
                }}
              >
                {cancelling ? <Spinner /> : <XIcon />} Cancel
              </Button>
            ) : (
              <>
                {unfinished.length > 0 && !fatal && (
                  <Button
                    variant="outline"
                    title={`Copy the ${plural(unfinished.length, "template")} that didn't finish again, with the same options`}
                    onClick={() => onRetry(unfinished)}
                  >
                    <RefreshCwIcon /> Retry {unfinished.length}
                  </Button>
                )}
                <Button variant="outline" onClick={onCopyMore}>
                  <ListRestartIcon /> Choose more
                </Button>
                <Button onClick={onStartOver}>
                  <RotateCcwIcon /> Start over
                </Button>
              </>
            )}
          </div>
        </>
      }
    >
      <ScreenHeading
        title={title}
        description="Copies get new template IDs in the destination, so update any code that uses the old IDs."
      />

      <div className="space-y-2">
        <Progress value={(done / job.ids.length) * 100} />
        <p className="text-sm text-muted-foreground" aria-live="polite">
          {done} of {job.ids.length} done
          {succeeded.length > 0 && (
            <>
              {" · "}
              <span className="text-emerald-600 dark:text-emerald-400">{succeeded.length} copied</span>
            </>
          )}
          {skipped > 0 && <> · {skipped} skipped</>}
          {failed > 0 && (
            <>
              {" · "}
              <span className="text-destructive">{failed} failed</span>
            </>
          )}
        </p>
        {fatal && <p className="text-sm text-destructive">{fatal}</p>}
      </div>

      <ul className="min-h-16 divide-y overflow-y-auto rounded-xl border">
        {job.ids.map((id, i) => (
          <ResultRow
            key={id}
            id={id}
            name={job.names[i]}
            result={results[i]}
            state={results[i] ? "done" : inProgress.has(i) ? "copying" : finished ? "not-run" : "waiting"}
            onCopyID={(newID) => copyText(newID, "New ID")}
          />
        ))}
      </ul>
    </Screen>
  )
}

type ResultRowProps = {
  id: string
  name: string
  result: CopyResult | undefined
  state: "waiting" | "copying" | "done" | "not-run"
  onCopyID: (id: string) => void
}

function ResultRow({ id, name, result: r, state, onCopyID }: ResultRowProps) {
  const cancelled = r?.error === cancelledError || state === "not-run"
  const ok = r && !r.error && r.action !== CopyAction.Skipped

  let icon = <span className="block size-5 rounded-full border-2 border-dashed" />
  if (state === "copying") icon = <Spinner className="size-5 text-primary" />
  else if (cancelled) icon = <CircleMinusIcon className="size-5 text-muted-foreground" />
  else if (r?.error) icon = <CircleXIcon className="size-5 text-destructive" />
  else if (r?.action === CopyAction.Skipped) icon = <CircleSlashIcon className="size-5 text-muted-foreground" />
  else if (ok) icon = <CircleCheckIcon className="size-5 text-emerald-600 dark:text-emerald-400" />

  const newID = r?.newID ?? ""
  const idButton = (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="ghost" size="icon-xs" aria-label="Copy new ID" onClick={() => onCopyID(newID)}>
          <CopyIcon />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Copy new ID</TooltipContent>
    </Tooltip>
  )

  return (
    <li className="flex items-start gap-3 px-4 py-3">
      <span className="mt-0.5">{icon}</span>
      <div className="min-w-0 flex-1">
        <p className="truncate font-medium">
          {name}
          {r?.newName && r.newName !== name && <span className="font-normal text-muted-foreground"> → {r.newName}</span>}
        </p>
        {state === "waiting" && <p className="font-mono text-xs text-muted-foreground">{id}</p>}
        {state === "copying" && <p className="text-xs text-muted-foreground">Copying…</p>}
        {cancelled && <p className="text-sm text-muted-foreground">Not copied (cancelled).</p>}
        {r && !cancelled && r.error && (
          <div className="text-sm">
            <p className="text-destructive">{r.error}</p>
            {r.partialID && (
              <p className="text-muted-foreground">
                A partial copy couldn't be removed and was left in the destination as{" "}
                <code className="font-mono">{r.partialID}</code>.
              </p>
            )}
          </div>
        )}
        {r?.action === CopyAction.Skipped && (
          <p className="text-sm text-muted-foreground">
            Skipped: already exists as <code className="font-mono text-xs">{newID}</code>
          </p>
        )}
        {ok && (
          <p className="flex flex-wrap items-center gap-x-2 text-sm">
            <span className="text-muted-foreground">{r.action === CopyAction.Added ? "Added to" : "New ID"}</span>
            <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{newID}</code>
            {idButton}
            <span className="text-xs text-muted-foreground">
              was {r.id} · {plural(r.versions, "version")}
              {r.action === CopyAction.Added && " added"}
            </span>
          </p>
        )}
      </div>
    </li>
  )
}
