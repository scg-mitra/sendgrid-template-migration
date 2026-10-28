import { useState, type FormEvent } from "react"
import { ArrowRightIcon, CheckIcon, EyeIcon, EyeOffIcon, KeyRoundIcon, LockIcon, SearchIcon, UsersIcon } from "lucide-react"

import { Backend, errorMessage, isCancelled, type Connection, type Region, type Session } from "@/api"
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
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Popover, PopoverAnchor, PopoverContent } from "@/components/ui/popover"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

/** How the destination subuser is reached. */
export type DestMode = "key" | "subuser"

type Props = {
  regions: Region[]
  conn: Connection
  mode: DestMode
  onChange: (conn: Connection) => void
  onModeChange: (mode: DestMode) => void
  onConnected: (session: Session) => void
}

export function ConnectScreen({ regions, conn, mode, onChange, onModeChange, onConnected }: Props) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const set = (patch: Partial<Connection>) => onChange({ ...conn, ...patch })

  async function submit(e: FormEvent) {
    e.preventDefault()
    // Only send the destination detail for the chosen mode.
    const request: Connection = { ...conn, destKey: mode === "key" ? conn.destKey : "", subuser: mode === "subuser" ? conn.subuser : "" }
    if (!request.sourceKey.trim()) return setError("Enter the source API key.")
    if (mode === "key" && !request.destKey.trim()) return setError("Enter the subuser's API key.")
    if (mode === "subuser" && !request.subuser.trim()) return setError("Enter the subuser's username.")

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

  return (
    <form onSubmit={submit} className="h-full" autoComplete="off">
      <Screen
        actions={
          <>
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <LockIcon className="size-3.5" /> Keys are only used while the app is open and are never saved.
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
          description="Templates are copied from the source account (usually the parent) to a subuser."
        />

        <Card>
          <CardContent>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="source-key">Source API key</FieldLabel>
                <SecretInput id="source-key" value={conn.sourceKey} onChange={(v) => set({ sourceKey: v })} disabled={busy} autoFocus={!conn.sourceKey} />
                <FieldDescription>Parent account. Needs read access to Template Engine.</FieldDescription>
              </Field>

              <Field>
                <FieldLabel>Copy to the subuser using</FieldLabel>
                <ToggleGroup
                  type="single"
                  variant="outline"
                  value={mode}
                  onValueChange={(v) => v && onModeChange(v as DestMode)}
                  disabled={busy}
                  className="w-full"
                >
                  <ToggleGroupItem value="key" className="flex-1 data-[state=on]:border-primary data-[state=on]:bg-primary/10 data-[state=on]:text-primary">
                    <KeyRoundIcon /> Subuser API key
                  </ToggleGroupItem>
                  <ToggleGroupItem value="subuser" className="flex-1 data-[state=on]:border-primary data-[state=on]:bg-primary/10 data-[state=on]:text-primary">
                    <UsersIcon /> Parent key + subuser username
                  </ToggleGroupItem>
                </ToggleGroup>
              </Field>

              {mode === "key" ? (
                <Field>
                  <FieldLabel htmlFor="dest-key">Subuser API key</FieldLabel>
                  <SecretInput id="dest-key" value={conn.destKey} onChange={(v) => set({ destKey: v })} disabled={busy} />
                  <FieldDescription>The subuser's own key. Needs full access to Template Engine.</FieldDescription>
                </Field>
              ) : (
                <Field>
                  <FieldLabel htmlFor="subuser">Subuser username</FieldLabel>
                  <SubuserPicker key={conn.baseURL + conn.sourceKey} conn={conn} value={conn.subuser} onChange={(v) => set({ subuser: v })} disabled={busy} />
                  <FieldDescription>
                    The source (parent) key acts as this subuser, so it needs full access to Template Engine.
                  </FieldDescription>
                </Field>
              )}

              <Field>
                <FieldLabel htmlFor="region">Data region</FieldLabel>
                <Select value={conn.baseURL} onValueChange={(v) => set({ baseURL: v })} disabled={busy}>
                  <SelectTrigger id="region" className="w-full">
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

type SecretInputProps = {
  id: string
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  autoFocus?: boolean
}

/** A password field with a show/hide toggle. */
function SecretInput({ id, value, onChange, disabled, autoFocus }: SecretInputProps) {
  const [visible, setVisible] = useState(false)
  return (
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
  )
}

type SubuserPickerProps = {
  conn: Connection
  value: string
  onChange: (value: string) => void
  disabled?: boolean
}

/** A username field whose "Find" button lists the parent account's subusers. */
function SubuserPicker({ conn, value, onChange, disabled }: SubuserPickerProps) {
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
              id="subuser"
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
              <CommandGroup heading={`${subusers?.length ?? 0} active subuser(s)`}>
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
