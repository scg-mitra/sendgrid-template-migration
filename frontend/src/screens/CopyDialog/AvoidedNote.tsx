import { CircleCheckIcon } from "lucide-react"

interface AvoidedNoteProps {
  count: number
}

/** Confirms that the new names avoid every name clash. */
export function AvoidedNote({ count }: AvoidedNoteProps) {
  return (
    <p className="flex items-center gap-2 text-sm text-emerald-700 dark:text-emerald-400">
      <CircleCheckIcon className="size-4 shrink-0" />
      {count === 1
        ? "The new names avoid the 1 name that already exists in the destination."
        : `The new names avoid the ${count} names that already exist in the destination.`}
    </p>
  )
}
