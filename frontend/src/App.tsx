import { useCallback, useEffect, useState } from "react"
import { ArrowUpCircleIcon, CheckIcon } from "lucide-react"

import { Backend, Browser, errorMessage, type Connection, type Defaults, type Session, type UpdateInfo } from "@/api"
import { ConnectScreen, type DestMode, type SourceMode } from "@/screens/ConnectScreen"
import { CopyScreen, type CopyJob } from "@/screens/CopyScreen"
import { SelectScreen } from "@/screens/SelectScreen"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"

type Step = "connect" | "select" | "copy"

const steps: { id: Step; label: string }[] = [
  { id: "connect", label: "Connect" },
  { id: "select", label: "Choose templates" },
  { id: "copy", label: "Copy" },
]

const emptyConnection: Connection = {
  baseURL: "",
  sourceKey: "",
  sourceSubuser: "",
  sourceFile: "",
  destKey: "",
  subuser: "",
  exportOnly: false,
}

export default function App() {
  const [defaults, setDefaults] = useState<Defaults | null>(null)
  const [loadError, setLoadError] = useState("")
  const [update, setUpdate] = useState<UpdateInfo | null>(null)
  const [step, setStep] = useState<Step>("connect")
  const [conn, setConn] = useState<Connection>(emptyConnection)
  const [sourceMode, setSourceMode] = useState<SourceMode>("key")
  const [destMode, setDestMode] = useState<DestMode>("key")
  const [session, setSession] = useState<Session | null>(null)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [job, setJob] = useState<CopyJob | null>(null)
  const [jobRun, setJobRun] = useState(0)
  const [running, setRunning] = useState(false)

  useEffect(() => {
    Backend.Defaults()
      .then((d) => {
        setDefaults(d)
        setConn({ ...emptyConnection, baseURL: d.baseURL, sourceKey: d.sourceKey, sourceSubuser: d.sourceSubuser, destKey: d.destKey, subuser: d.subuser })
        if (d.sourceSubuser) setSourceMode("subuser")
        if (d.subuser && !d.destKey) setDestMode("subuser")
        else if (d.sourceSubuser && !d.destKey) setDestMode("parent")
      })
      .catch((err) => setLoadError(errorMessage(err)))
    // A failed update check isn't worth bothering anyone about.
    Backend.CheckForUpdate()
      .then(setUpdate)
      .catch(() => {})
  }, [])

  /** Records that templates with these names now exist in the destination. */
  const addDestNames = useCallback((names: string[]) => {
    if (names.length === 0) return
    setSession((prev) =>
      prev && {
        ...prev,
        destCount: prev.destCount + names.length,
        destNames: [...new Set([...prev.destNames, ...names])].sort(),
      },
    )
  }, [])

  const startJob = (j: CopyJob) => {
    setJob(j)
    setJobRun((n) => n + 1)
    setStep("copy")
  }

  // Finished steps can be revisited, except while a copy is running.
  const reachable = (s: Step) => {
    if (running) return false
    if (s === "connect") return step !== "connect"
    if (s === "select") return step === "copy" && session !== null
    return false
  }

  return (
    <div className="flex h-svh flex-col bg-background text-foreground">
      <header className="flex items-center justify-between gap-4 border-b px-6 py-3">
        <div className="flex min-w-0 items-center gap-3">
          <img src="/icon.svg" alt="" className="size-9" />
          <div className="min-w-0 leading-tight">
            <h1 className="truncate font-heading text-base font-semibold">SendGrid Template Migration</h1>
            {defaults && (
              <p className="flex items-center gap-2 text-xs text-muted-foreground">
                Version {defaults.version}
                {update && (
                  <Button
                    variant="link"
                    size="xs"
                    className="h-auto p-0 text-xs"
                    onClick={() => Browser.OpenURL(update.url)}
                  >
                    <ArrowUpCircleIcon /> {update.version} available
                  </Button>
                )}
              </p>
            )}
          </div>
        </div>
        <Stepper current={step} reachable={reachable} onSelect={setStep} />
      </header>

      <main className="min-h-0 flex-1">
        {loadError && (
          <Alert variant="destructive" className="m-6 max-w-xl">
            <AlertTitle>The app couldn't start</AlertTitle>
            <AlertDescription>{loadError}</AlertDescription>
          </Alert>
        )}
        {!defaults && !loadError && (
          <div className="flex h-full items-center justify-center">
            <Spinner className="size-6" />
          </div>
        )}
        {defaults && step === "connect" && (
          <ConnectScreen
            regions={defaults.regions}
            conn={conn}
            sourceMode={sourceMode}
            destMode={destMode}
            onChange={setConn}
            onModesChange={(s, d) => {
              setSourceMode(s)
              setDestMode(d)
            }}
            onConnected={(s) => {
              setSession(s)
              // Keep the selection if the same templates are still there.
              const ids = new Set(s.templates.map((t) => t.id))
              setSelected((prev) => new Set([...prev].filter((id) => ids.has(id))))
              setStep("select")
            }}
          />
        )}
        {session && step === "select" && (
          <SelectScreen
            session={session}
            selected={selected}
            onSelectedChange={setSelected}
            onBack={() => setStep("connect")}
            onCopy={startJob}
          />
        )}
        {session && job && step === "copy" && (
          <CopyScreen
            key={jobRun}
            job={job}
            onRunningChange={setRunning}
            onFinished={addDestNames}
            onCopyMore={() => {
              setSelected(new Set())
              setStep("select")
            }}
            onRetry={(ids) => {
              const names = new Map(job.ids.map((id, i) => [id, job.names[i]]))
              startJob({ ...job, ids, names: ids.map((id) => names.get(id) ?? id) })
            }}
            onStartOver={() => {
              setSession(null)
              setSelected(new Set())
              setStep("connect")
            }}
          />
        )}
      </main>
    </div>
  )
}

type StepperProps = {
  current: Step
  reachable: (step: Step) => boolean
  onSelect: (step: Step) => void
}

function Stepper({ current, reachable, onSelect }: StepperProps) {
  const currentIndex = steps.findIndex((s) => s.id === current)
  return (
    <ol className="flex shrink-0 items-center gap-2 text-sm" aria-label="Progress">
      {steps.map((s, i) => {
        const done = i < currentIndex
        const active = i === currentIndex
        const canGo = reachable(s.id)
        const content = (
          <>
            <span
              className={cn(
                "flex size-6 shrink-0 items-center justify-center rounded-full border text-xs font-medium",
                active && "border-primary bg-primary text-primary-foreground",
                done && "border-primary text-primary",
                !active && !done && "text-muted-foreground",
              )}
            >
              {done ? <CheckIcon className="size-3.5" /> : i + 1}
            </span>
            {/* Labels only fit on wider windows; the active one is always shown. */}
            <span className={cn(active ? "font-medium" : "hidden text-muted-foreground md:inline")}>{s.label}</span>
          </>
        )
        return (
          <li key={s.id} className="flex items-center gap-2" aria-current={active ? "step" : undefined}>
            {i > 0 && <span className="h-px w-4 bg-border md:w-6" aria-hidden />}
            {canGo ? (
              <button
                type="button"
                onClick={() => onSelect(s.id)}
                className="flex items-center gap-2 rounded-md outline-none hover:underline focus-visible:ring-3 focus-visible:ring-ring/50"
                title={`Back to ${s.label}`}
              >
                {content}
              </button>
            ) : (
              <span className="flex items-center gap-2">{content}</span>
            )}
          </li>
        )
      })}
    </ol>
  )
}
