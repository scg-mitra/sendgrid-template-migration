import { useEffect, useState } from "react"

import { Backend, errorMessage, isCancelled, type Preview, type TemplateInfo } from "@/api"
import { Badge } from "@/components/ui/badge"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

type Props = {
  template: TemplateInfo
  onClose: () => void
}

type State = { status: "loading" } | { status: "error"; message: string } | { status: "done"; preview: Preview | null }

/**
 * Shows a template's active version. The HTML is rendered in a sandbox with
 * scripts disabled. Give it a key per template so each one starts afresh.
 */
export function PreviewDialog({ template, onClose }: Props) {
  const [state, setState] = useState<State>({ status: "loading" })
  const [view, setView] = useState<"html" | "plain">("html")

  useEffect(() => {
    const call = Backend.PreviewTemplate(template.id)
    call
      .then((preview) => setState({ status: "done", preview }))
      .catch((err) => {
        if (!isCancelled(err)) setState({ status: "error", message: errorMessage(err) })
      })
    return () => {
      // Cancelling rejects the promise with a CancelError, handled above.
      void call.cancel()
    }
  }, [template.id])

  const preview = state.status === "done" ? state.preview : null
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="flex max-h-[90svh] flex-col sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle className="truncate pr-6">{template.name}</DialogTitle>
          <DialogDescription className="flex flex-wrap items-center gap-2">
            {preview ? (
              <>
                <span>
                  Version "{preview.versionName}"{preview.subject && <> · Subject: {preview.subject}</>}
                </span>
                {preview.active && <Badge variant="secondary">Active</Badge>}
              </>
            ) : (
              <span className="font-mono text-xs">{template.id}</span>
            )}
          </DialogDescription>
        </DialogHeader>

        {state.status === "loading" && (
          <div className="flex h-64 items-center justify-center">
            <Spinner className="size-6" />
          </div>
        )}
        {state.status === "error" && <p className="text-sm text-destructive">{state.message}</p>}
        {state.status === "done" && !preview && <p className="text-sm text-muted-foreground">This template has no versions.</p>}
        {preview && (
          <>
            {preview.plain && preview.html && (
              <ToggleGroup
                type="single"
                variant="outline"
                size="sm"
                value={view}
                onValueChange={(v) => v && setView(v as "html" | "plain")}
                aria-label="Content to show"
                className="self-start"
              >
                <ToggleGroupItem value="html">HTML</ToggleGroupItem>
                <ToggleGroupItem value="plain">Plain text</ToggleGroupItem>
              </ToggleGroup>
            )}
            {view === "html" && preview.html ? (
              <iframe
                title={`Preview of ${template.name}`}
                sandbox=""
                srcDoc={preview.html}
                className="h-[60svh] w-full rounded-lg border bg-white"
              />
            ) : (
              <pre className="h-[60svh] overflow-auto rounded-lg border bg-muted/30 p-4 text-sm whitespace-pre-wrap">
                {preview.plain || "No content."}
              </pre>
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
