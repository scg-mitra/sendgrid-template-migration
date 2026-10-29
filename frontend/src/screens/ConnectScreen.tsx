import { useState, type FormEvent, type ReactNode } from "react"
import {
  ArrowRightIcon,
  Building2Icon,
  CheckIcon,
  EyeIcon,
  EyeOffIcon,
  FileBracesIcon,
  FolderOpenIcon,
  KeyRoundIcon,
  LockIcon,
  SearchIcon,
  TriangleAlertIcon,
  UsersIcon,
} from "lucide-react"

import { Backend, Dialogs, backupFilter, errorMessage, isCancelled, type Connection, type Region, type Session } from "@/api"
import { Screen, ScreenHeading } from "@/screens/layout"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel, FieldLegend, FieldSeparator, FieldSet } from "@/components/ui/field"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Popover, PopoverAnchor, PopoverContent } from "@/components/ui/popover"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { plural } from "@/lib/format"

/** Where templates are copied from. */
export type SourceMode = "key" | "subuser" | "file"

/** Where templates are copied to. */
export type DestMode = "key" | "subuser" | "parent" | "file"

/** The destinations that make sense for each kind of source. */
function destModesFor(source: SourceMode): DestMode[] {
  switch (source) {
    case "key":
      return ["key", "subuser", "file"]
    case "subuser":
      return ["key", "subuser", "parent", "file"]
    case "file":
      return ["key"]
  }
}

/** The connection to send to the backend: only the fields the chosen modes use. */
function connectionFor(conn: Connection, source: SourceMode, dest: DestMode): Connection {
  return {
    baseURL: conn.baseURL,
    sourceKey: source !== "file" ? conn.sourceKey : "",
    sourceSubuser: source === "subuser" ? conn.sourceSubuser : "",
    sourceFile: source === "file" ? conn.sourceFile : "",
    destKey: dest === "key" ? conn.destKey : "",
    subuser: dest === "subuser" ? conn.subuser : "",
    exportOnly: dest === "file",
  }
}

type Props = {
  regions: Region[]
  conn: Connection
  sourceMode: SourceMode
  destMode: DestMode
  onChange: (conn: Connection) => void
  onModesChange: (source: SourceMode, dest: DestMode) => void
  onConnected: (session: Session) => void
}

const toggleItemClass = "flex-1 data-[state=on]:border-primary data-[state=on]:bg-primary/10 data-[state=on]:text-primary"

const sourceOptions: { value: SourceMode; label: string; icon: ReactNode }[] = [
  { value: "key", label: "API key", icon: <KeyRoundIcon /> },
  { value: "subuser", label: "Subuser", icon: <UsersIcon /> },
  { value: "file", label: "Backup file", icon: <FileBracesIcon /> },
]

const destOptions: Record<DestMode, { label: string; icon: ReactNode }> = {
  key: { label: "API key", icon: <KeyRoundIcon /> },
  subuser: { label: "Subuser", icon: <UsersIcon /> },
  parent: { label: "Parent", icon: <Building2Icon /> },
  file: { label: "Backup file", icon: <FileBracesIcon /> },
}

export function ConnectScreen({ regions, conn, sourceMode, destMode, onChange, onModesChange, onConnected }: Props) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const set = (patch: Partial<Connection>) => onChange({ ...conn, ...patch })

  const allowedDest = destModesFor(sourceMode)
  const dest = allowedDest.includes(destMode) ? destMode : allowedDest[0]

  async function submit(e: FormEvent) {
    e.preventDefault()
    const request = connectionFor(conn, sourceMode, dest)
    const missing = missingField(request, sourceMode, dest)
    if (missing) return setError(missing)

    setBusy(true)
    setError("")
    try {
      const session = await Backend.Connect(request)
      if (session) onConnected(session)
    } catch (err) {
      if (!isCancelled(err)) setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const parentKeyLabel = sourceMode === "subuser" || dest === "subuser" || dest === "parent" ? "Parent API key" : "Source API key"
  const sourceAccess = dest === "subuser" || dest === "parent" ? "full" : "read"

  return (
    <form onSubmit={submit} className="h-full" autoComplete="off">
      <Screen
        actions={
          <>
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <LockIcon className="size-3.5 shrink-0" /> Keys are only used while the app is open and are never saved.
            </p>
            <Button type="submit" className="ml-auto" disabled={busy}>
              {busy ? <Spinner /> : null}
              {busy ? "Connecting…" : "Connect"}
              {!busy && <ArrowRightIcon />}
            </Button>
          </>
        }
      >
        <ScreenHeading
          title="Connect your SendGrid accounts"
          description="Copy templates between a parent account, its subusers, and backup files."
        />

        <Card className="shrink-0">
          <CardContent>
            <FieldGroup>
              <Field orientation="responsive">
                <FieldContent>
                  <FieldLabel htmlFor="region">Data region</FieldLabel>
                  <FieldDescription>Both accounts must be in this region. EU accounts use the EU region.</FieldDescription>
                </FieldContent>
                <Select value={conn.baseURL} onValueChange={(v) => set({ baseURL: v })} disabled={busy}>
                  <SelectTrigger id="region" className="@md/field-group:w-72">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {regions.map((r) => (
                      <SelectItem key={r.baseURL} value={r.baseURL}>
                        {r.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>

              <FieldSeparator />

              {/* Side by side when there's room, so the whole form fits without scrolling. */}
              <div className="grid gap-6 md:grid-cols-2 md:gap-0 md:divide-x">
                <FieldSet className="md:pr-6">
                  <FieldLegend>Copy from</FieldLegend>
                  <FieldGroup>
                    <ToggleGroup
                      type="single"
                      variant="outline"
                      value={sourceMode}
                      onValueChange={(v) => {
                        if (!v) return
                        const next = v as SourceMode
                        const allowed = destModesFor(next)
                        onModesChange(next, allowed.includes(destMode) ? destMode : allowed[0])
                      }}
                      disabled={busy}
                      className="w-full"
                      aria-label="Copy from"
                    >
                      {sourceOptions.map((o) => (
                        <ToggleGroupItem key={o.value} value={o.value} className={toggleItemClass}>
                          {o.icon} {o.label}
                        </ToggleGroupItem>
                      ))}
                    </ToggleGroup>

                    {sourceMode === "file" ? (
                      <Field>
                        <FieldLabel htmlFor="source-file">Backup file</FieldLabel>
                        <FilePicker id="source-file" value={conn.sourceFile} onChange={(v) => set({ sourceFile: v })} disabled={busy} />
                        <FieldDescription>A file saved with "Backup file" as the destination.</FieldDescription>
                      </Field>
                    ) : (
                      <Field>
                        <FieldLabel htmlFor="source-key">{parentKeyLabel}</FieldLabel>
                        <SecretInput id="source-key" value={conn.sourceKey} onChange={(v) => set({ sourceKey: v })} disabled={busy} autoFocus={!conn.sourceKey} />
                        <FieldDescription>Needs {sourceAccess} access to Template Engine.</FieldDescription>
                      </Field>
                    )}

                    {sourceMode === "subuser" && (
                      <Field>
                        <FieldLabel htmlFor="source-subuser">Subuser to copy from</FieldLabel>
                        <SubuserPicker
                          key={conn.baseURL + conn.sourceKey}
                          id="source-subuser"
                          conn={conn}
                          value={conn.sourceSubuser}
                          onChange={(v) => set({ sourceSubuser: v })}
                          disabled={busy}
                        />
                      </Field>
                    )}
                  </FieldGroup>
                </FieldSet>

                <FieldSet className="md:pl-6">
                  <FieldLegend>Copy to</FieldLegend>
                  <FieldGroup>
                    {allowedDest.length > 1 && (
                      <ToggleGroup
                        type="single"
                        variant="outline"
                        value={dest}
                        onValueChange={(v) => v && onModesChange(sourceMode, v as DestMode)}
                        disabled={busy}
                        className="w-full flex-wrap"
                        aria-label="Copy to"
                      >
                        {allowedDest.map((d) => (
                          <ToggleGroupItem key={d} value={d} className={toggleItemClass}>
                            {destOptions[d].icon} {destOptions[d].label}
                          </ToggleGroupItem>
                        ))}
                      </ToggleGroup>
                    )}

                    {dest === "key" && (
                      <Field>
                        <FieldLabel htmlFor="dest-key">Destination API key</FieldLabel>
                        <SecretInput id="dest-key" value={conn.destKey} onChange={(v) => set({ destKey: v })} disabled={busy} />
                        <FieldDescription>The destination account's own key. Needs full access to Template Engine.</FieldDescription>
                      </Field>
                    )}
                    {dest === "subuser" && (
                      <Field>
                        <FieldLabel htmlFor="dest-subuser">Subuser to copy to</FieldLabel>
                        <SubuserPicker
                          key={conn.baseURL + conn.sourceKey}
                          id="dest-subuser"
                          conn={conn}
                          value={conn.subuser}
                          onChange={(v) => set({ subuser: v })}
                          disabled={busy}
                        />
                        <FieldDescription>The parent key acts as this subuser, so you don't need the subuser's own key.</FieldDescription>
                      </Field>
                    )}
                    {dest === "parent" && (
                      <p className="text-sm text-muted-foreground">
                        Templates are copied to the parent account that the parent API key belongs to.
                      </p>
                    )}
                    {dest === "file" && (
                      <p className="text-sm text-muted-foreground">
                        Choose templates next, then where to save them. The file keeps every version, and can be used as the
                        source later, from any computer.
                      </p>
                    )}
                  </FieldGroup>
                </FieldSet>
              </div>
            </FieldGroup>
          </CardContent>
        </Card>

        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
      </Screen>
    </form>
  )
}

/** Returns a message for the first required field that's empty, or "". */
function missingField(c: Connection, source: SourceMode, dest: DestMode): string {
  if (source === "file" && !c.sourceFile.trim()) return "Choose a backup file to copy from."
  if (source !== "file" && !c.sourceKey.trim()) return "Enter the source API key."
  if (source === "subuser" && !c.sourceSubuser.trim()) return "Enter the subuser to copy from."
  if (dest === "key" && !c.destKey.trim()) return "Enter the destination API key."
  if (dest === "subuser" && !c.subuser.trim()) return "Enter the subuser to copy to."
  if (source === "subuser" && dest === "subuser" && c.subuser.trim().toLowerCase() === c.sourceSubuser.trim().toLowerCase())
    return "Choose a different subuser to copy to."
  return ""
}

type SecretInputProps = {
  id: string
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  autoFocus?: boolean
}

/** A password field with a show/hide toggle and a hint for keys that look wrong. */
function SecretInput({ id, value, onChange, disabled, autoFocus }: SecretInputProps) {
  const [visible, setVisible] = useState(false)
  const trimmed = value.trim()
  const suspicious = trimmed !== "" && !trimmed.startsWith("SG.")
  return (
    <>
      <InputGroup>
        <InputGroupInput
          id={id}
          type={visible ? "text" : "password"}
          placeholder="SG.xxxxxxxx"
          spellCheck={false}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
          autoFocus={autoFocus}
          aria-invalid={suspicious || undefined}
          aria-describedby={suspicious ? `${id}-hint` : undefined}
          className="font-mono"
        />
        <InputGroupAddon align="inline-end">
          <InputGroupButton
            size="icon-xs"
            aria-label={visible ? "Hide key" : "Show key"}
            onClick={() => setVisible((v) => !v)}
            disabled={disabled}
          >
            {visible ? <EyeOffIcon /> : <EyeIcon />}
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
      {suspicious && (
        <p id={`${id}-hint`} className="flex items-center gap-1.5 text-sm text-amber-700 dark:text-amber-400">
          <TriangleAlertIcon className="size-3.5 shrink-0" /> SendGrid API keys start with "SG." Check the whole key was
          copied.
        </p>
      )}
    </>
  )
}

type FilePickerProps = {
  id: string
  value: string
  onChange: (value: string) => void
  disabled?: boolean
}

/** A read-only path field with a "Choose…" button that opens a file dialog. */
function FilePicker({ id, value, onChange, disabled }: FilePickerProps) {
  const [error, setError] = useState("")

  async function choose() {
    setError("")
    try {
      const path = await Dialogs.OpenFile({
        Title: "Choose a template backup",
        Filters: [backupFilter],
        CanChooseFiles: true,
        AllowsMultipleSelection: false,
      })
      if (typeof path === "string" && path) onChange(path)
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  return (
    <>
      <InputGroup>
        <InputGroupInput id={id} readOnly placeholder="No file chosen" value={value} disabled={disabled} onClick={choose} className="cursor-pointer" />
        <InputGroupAddon align="inline-end">
          <InputGroupButton onClick={choose} disabled={disabled}>
            <FolderOpenIcon /> Choose…
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
      {error && <p className="text-sm text-destructive">{error}</p>}
    </>
  )
}

type SubuserPickerProps = {
  id: string
  conn: Connection
  value: string
  onChange: (value: string) => void
  disabled?: boolean
}

/** A username field whose "Find" button lists the parent account's subusers. */
function SubuserPicker({ id, conn, value, onChange, disabled }: SubuserPickerProps) {
  const [subusers, setSubusers] = useState<string[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")
  const [open, setOpen] = useState(false)

  async function find() {
    if (subusers) return setOpen(true)
    setLoading(true)
    setError("")
    try {
      const names = await Backend.ListSubusers(conn.baseURL, conn.sourceKey)
      setSubusers(names)
      setOpen(true)
    } catch (err) {
      if (!isCancelled(err)) setError(errorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-2">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverAnchor asChild>
          <InputGroup>
            <InputGroupInput
              id={id}
              placeholder="Type a username or click Find"
              spellCheck={false}
              value={value}
              onChange={(e) => onChange(e.target.value)}
              disabled={disabled}
            />
            <InputGroupAddon align="inline-end">
              <InputGroupButton onClick={find} disabled={disabled || loading}>
                {loading ? <Spinner /> : <SearchIcon />} Find
              </InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
        </PopoverAnchor>
        <PopoverContent className="w-(--radix-popover-trigger-width) p-0" align="start">
          <Command>
            <CommandInput placeholder="Search subusers…" />
            <CommandList>
              <CommandEmpty>No subusers found.</CommandEmpty>
              <CommandGroup heading={plural(subusers?.length ?? 0, "active subuser")}>
                {subusers?.map((name) => (
                  <CommandItem
                    key={name}
                    value={name}
                    onSelect={() => {
                      onChange(name)
                      setOpen(false)
                    }}
                  >
                    {name}
                    {name === value && <CheckIcon className="ml-auto" />}
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
      {error && <p className="text-sm text-destructive">{error}</p>}
    </div>
  )
}
