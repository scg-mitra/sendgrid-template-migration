import { useEffect, useRef, useState } from "react"
import { CheckIcon, CircleCheckIcon, CircleXIcon, ClipboardCopyIcon, CopyIcon, ListRestartIcon } from "lucide-react"
import { toast } from "sonner"

import { Application, Backend, Clipboard, CopyProgressEvent, Events, errorMessage, isCancelled, type CopyResult } from "@/api"
import { Screen, ScreenHeading } from "@/screens/layout"
import { Button } from "@/components/ui/button"
import { Progress } from "@/components/ui/progress"
import { Spinner } from "@/components/ui/spinner"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

/** The templates the user chose to copy. */
export type CopyJob = {
  ids: string[]
  names: string[]
  activeOnly: boolean
}

type Props = {
  job: CopyJob
  /** Go back to the template list; copiedNames now exist in the destination. */
  onCopyMore: (copiedNames: Set<string>) => void
}

export function CopyScreen({ job, onCopyMore }: Props) {
  const [results, setResults] = useState<(CopyResult | undefined)[]>(() => job.ids.map(() => undefined))
  const [finished, setFinished] = useState(false)
  const [fatal, setFatal] = useState("")
  const started = useRef(false)

  useEffect(() => {
    const off = Events.On(CopyProgressEvent, (ev) => {
      const { done, result } = ev.data
      setResults((prev) => prev.map((r, i) => (i === done - 1 ? result : r)))
    })
    // Start once, even though React's strict mode runs effects twice in development.
    if (!started.current) {
      started.current = true
      Backend.CopyTemplates(job.ids, job.activeOnly)
        .then((all) => setResults(job.ids.map((_, i) => all[i])))
        .catch((err) => {
          if (!isCancelled(err)) setFatal(errorMessage(err))
        })
        .finally(() => setFinished(true))
    }
    return off
  }, [job])

  const done = results.filter(Boolean).length
  const copied = results.filter((r): r is CopyResult => !!r && !r.error)
  const failed = results.filter((r) => r?.error).length
  const current = finished ? -1 : done

  const copyText = async (text: string, what: string) => {
    await Clipboard.SetText(text)
    toast.success(`${what} copied to the clipboard`)
  }

  let title = `Copying ${job.ids.length} template${job.ids.length === 1 ? "" : "s"}…`
  if (finished) title = failed || fatal ? "Finished with errors" : "All done"

  return (
    <Screen
      actions={
        <>
          <Button
            variant="outline"
            disabled={!finished || copied.length === 0}
            onClick={() =>
              copyText(
                ["Name\tOld ID\tNew ID", ...copied.map((r) => `${r.name}\t${r.id}\t${r.newID}`)].join("\n"),
                "ID list",
              )
            }
          >
            <ClipboardCopyIcon /> Copy ID list
          </Button>
          <Button
            variant="outline"
            className="ml-auto"
            disabled={!finished}
            onClick={() => onCopyMore(new Set(copied.map((r) => r.name)))}
          >
            <ListRestartIcon /> Copy more templates
          </Button>
          <Button disabled={!finished} onClick={() => Application.Quit()}>
            <CheckIcon /> Done
          </Button>
        </>
      }
    >
      <ScreenHeading
        title={title}
        description="Copies get new template IDs in the destination, so update any code that uses the old IDs."
      />

      <div className="space-y-2">
        <Progress value={(done / job.ids.length) * 100} />
        <p className="text-sm text-muted-foreground">
          {done} of {job.ids.length} done
          {copied.length > 0 && <> · <span className="text-emerald-600 dark:text-emerald-400">{copied.length} copied</span></>}
          {failed > 0 && <> · <span className="text-destructive">{failed} failed</span></>}
        </p>
        {fatal && <p className="text-sm text-destructive">{fatal}</p>}
      </div>

      <ul className="divide-y rounded-xl border">
        {job.ids.map((id, i) => {
          const r = results[i]
          return (
            <li key={id} className="flex items-start gap-3 px-4 py-3">
              <span className="mt-0.5">
                {r && !r.error && <CircleCheckIcon className="size-5 text-emerald-600 dark:text-emerald-400" />}
                {r?.error && <CircleXIcon className="size-5 text-destructive" />}
                {!r && i === current && <Spinner className="size-5 text-primary" />}
                {!r && i !== current && <span className="block size-5 rounded-full border-2 border-dashed" />}
              </span>
              <div className="min-w-0 flex-1">
                <p className="truncate font-medium">{job.names[i]}</p>
                {!r && (
                  <p className="font-mono text-xs text-muted-foreground">{i === current ? "Copying…" : id}</p>
                )}
                {r && !r.error && (
                  <p className="flex flex-wrap items-center gap-x-2 text-sm">
                    <span className="text-muted-foreground">New ID</span>
                    <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{r.newID}</code>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          aria-label="Copy new ID"
                          onClick={() => copyText(r.newID ?? "", "New ID")}
                        >
                          <CopyIcon />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>Copy new ID</TooltipContent>
                    </Tooltip>
                    <span className="text-xs text-muted-foreground">
                      was {r.id} · {r.versions} version{r.versions === 1 ? "" : "s"}
                    </span>
                  </p>
                )}
                {r?.error && (
                  <div className="text-sm">
                    <p className="text-destructive">{r.error}</p>
                    {r.partialID && (
                      <p className="text-muted-foreground">
                        A partial copy was left in the destination as <code className="font-mono">{r.partialID}</code>.
                      </p>
                    )}
                  </div>
                )}
              </div>
            </li>
          )
        })}
      </ul>
    </Screen>
  )
}
