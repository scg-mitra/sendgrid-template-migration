import type { TemplateInfo } from "@/api"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { clashPolicies } from "@/screens/CopyDialog/CopyDialog.helpers"
import type { Clash, ClashPolicy } from "@/screens/CopyDialog/CopyDialog.types"

interface ClashRowProps {
  clash: Clash
  onPolicyChange: (template: TemplateInfo, policy: ClashPolicy) => void
  onNameChange: (id: string, name: string) => void
}

/** One template whose name already exists, with what to do about it. */
export function ClashRow({ clash, onPolicyChange, onNameChange }: ClashRowProps) {
  const { template, newName, choice, error } = clash
  const inputID = `rename-${template.id}`
  return (
    <li className="space-y-2 px-3 py-2">
      <div className="flex items-center gap-3">
        <span className="min-w-0 flex-1 truncate text-sm font-medium" title={newName}>
          {newName}
        </span>
        <Select value={choice.policy} onValueChange={(v) => onPolicyChange(template, v as ClashPolicy)}>
          <SelectTrigger size="sm" className="w-48 shrink-0" aria-label={`What to do with "${newName}"`}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {clashPolicies.map((p) => (
              <SelectItem key={p.value} value={p.value}>
                {p.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {choice.policy === "rename" && (
        <div className="space-y-1">
          <Input
            id={inputID}
            aria-label={`New name for "${newName}"`}
            aria-invalid={error !== "" || undefined}
            aria-describedby={error ? `${inputID}-error` : undefined}
            value={choice.name}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => onNameChange(template.id, e.target.value)}
            autoFocus
          />
          {error && (
            <p id={`${inputID}-error`} className="text-xs text-destructive">
              {error}
            </p>
          )}
        </div>
      )}
    </li>
  )
}
