import { useEffect, useState } from "react"
import { CheckIcon } from "lucide-react"

import { Backend, errorMessage, type Connection, type Defaults, type Session } from "@/api"
import { ConnectScreen, type DestMode } from "@/screens/ConnectScreen"
import { CopyScreen, type CopyJob } from "@/screens/CopyScreen"
import { SelectScreen } from "@/screens/SelectScreen"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"

type Step = "connect" | "select" | "copy"

const steps: { id: Step; label: string }[] = [
  { id: "connect", label: "Connect" },
  { id: "select", label: "Choose templates" },
  { id: "copy", label: "Copy" },
]

export default function App() {
  const [defaults, setDefaults] = useState<Defaults | null>(null)
  const [loadError, setLoadError] = useState("")
  const [step, setStep] = useState<Step>("connect")
  const [conn, setConn] = useState<Connection>({ baseURL: "", sourceKey: "", destKey: "", subuser: "" })
  const [mode, setMode] = useState<DestMode>("key")
  const [session, setSession] = useState<Session | null>(null)
  const [job, setJob] = useState<CopyJob | null>(null)

  useEffect(() => {
    Backend.Defaults()
      .then((d) => {
        setDefaults(d)
        setConn({ baseURL: d.baseURL, sourceKey: d.sourceKey, destKey: d.destKey, subuser: d.subuser })
        if (d.subuser && !d.destKey) setMode("subuser")
      })
      .catch((err) => setLoadError(errorMessage(err)))
  }, [])

  return (
    <div className="flex h-svh flex-col bg-background text-foreground">
      <header className="flex items-center justify-between gap-6 border-b px-6 py-3">
        <div className="flex items-center gap-3">
          <img src="/icon.svg" alt="" className="size-9" />
          <div className="leading-tight">
            <h1 className="font-heading text-base font-semibold">SendGrid Template Migration</h1>
            {defaults && <p className="text-xs text-muted-foreground">Version {defaults.version}</p>}
          </div>
        </div>
        <Stepper current={step} />
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
            mode={mode}
            onChange={setConn}
            onModeChange={setMode}
            onConnected={(s) => {
              setSession(s)
              setStep("select")
            }}
          />
        )}
        {session && step === "select" && (
          <SelectScreen
            session={session}
            onBack={() => setStep("connect")}
            onCopy={(j) => {
              setJob(j)
              setStep("copy")
            }}
          />
        )}
        {session && job && step === "copy" && (
          <CopyScreen
            job={job}
            onCopyMore={(copiedNames) => {
              // Copied templates now exist in the destination.
              setSession({
                ...session,
                destCount: session.destCount + copiedNames.size,
                templates: session.templates.map((t) =>
                  copiedNames.has(t.name) ? { ...t, existsInDest: true } : t,
                ),
              })
              setStep("select")
            }}
          />
        )}
      </main>
    </div>
  )
}

function Stepper({ current }: { current: Step }) {
  const currentIndex = steps.findIndex((s) => s.id === current)
  return (
    <ol className="hidden items-center gap-2 text-sm sm:flex" aria-label="Progress">
      {steps.map((s, i) => {
        const done = i < currentIndex
        const active = i === currentIndex
        return (
          <li key={s.id} className="flex items-center gap-2">
            {i > 0 && <span className="h-px w-6 bg-border" aria-hidden />}
            <span
              className={cn(
                "flex size-6 items-center justify-center rounded-full border text-xs font-medium",
                active && "border-primary bg-primary text-primary-foreground",
                done && "border-primary text-primary",
                !active && !done && "text-muted-foreground",
              )}
              aria-current={active ? "step" : undefined}
            >
              {done ? <CheckIcon className="size-3.5" /> : i + 1}
            </span>
            <span className={cn(active ? "font-medium" : "text-muted-foreground")}>{s.label}</span>
          </li>
        )
      })}
    </ol>
  )
}
