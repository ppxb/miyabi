import { InlineError } from '@/components/error-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { SettingRow } from './shared'

type ToggleSource = { id: string; enabled: boolean }
type ToggleQuery = {
  data?: ToggleSource[]
  isPending: boolean
  isError: boolean
  isFetching: boolean
  refetch: () => void
}
type ToggleUpdate = {
  isPending: boolean
  isError: boolean
  variables?: ToggleSource[]
  mutate: (next: ToggleSource[]) => void
}

/** Source on/off toggles with loading, optimistic state and retry handling. */
export function SourceToggleList({
  query,
  update,
  names,
  descriptions,
  errorText
}: {
  query: ToggleQuery
  update: ToggleUpdate
  names: Record<string, string>
  descriptions: Record<string, string>
  errorText: string
}) {
  // Show the submitted state while the save is in flight so the toggle reacts
  // immediately instead of waiting for the refetch.
  const sources = (update.isPending ? update.variables : query.data) ?? []
  return (
    <>
      {query.isPending ? <Skeleton className="h-4 w-40" /> : null}
      {sources.map(source => (
        <SettingRow
          key={source.id}
          title={names[source.id] ?? source.id}
          description={descriptions[source.id]}
          inline
        >
          <Switch
            checked={source.enabled}
            onCheckedChange={enabled =>
              update.mutate(
                sources.map(item => (item.id === source.id ? { ...item, enabled } : item))
              )
            }
          />
        </SettingRow>
      ))}
      {query.isError || update.isError ? (
        <InlineError
          onRetry={() => {
            if (query.isError) void query.refetch()
            else if (update.variables) update.mutate(update.variables)
          }}
          retrying={query.isFetching || update.isPending}
        >
          {errorText}
        </InlineError>
      ) : null}
    </>
  )
}
