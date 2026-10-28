import type { ReactNode } from "react"

/** A screen with a scrollable body and a fixed action bar at the bottom. */
export function Screen({ children, actions }: { children: ReactNode; actions: ReactNode }) {
  return (
    <div className="flex h-full flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex h-full max-w-3xl flex-col gap-6 px-6 py-8">{children}</div>
      </div>
      <footer className="border-t bg-muted/30">
        <div className="mx-auto flex max-w-3xl items-center gap-3 px-6 py-3">{actions}</div>
      </footer>
    </div>
  )
}

export function ScreenHeading({ title, description }: { title: string; description: ReactNode }) {
  return (
    <div className="space-y-1">
      <h2 className="font-heading text-2xl font-semibold tracking-tight">{title}</h2>
      <p className="text-muted-foreground">{description}</p>
    </div>
  )
}
