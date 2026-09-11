import type { WatchHistoryItem } from '@/api/watch-history'
import { MovieCard } from '@/components/movie'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Progress } from '@/components/ui/progress'
import { cn } from '@/lib/utils'
import { formatWatchTime, watchProgressPercent } from '@/lib/watch-progress'
import { useUIStore } from '@/stores/ui'

const dateFormat = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit'
})

export function HistoryCard({
  item,
  selecting,
  selected,
  disabled,
  onSelect
}: {
  item: WatchHistoryItem
  selecting: boolean
  selected: boolean
  disabled: boolean
  onSelect: () => void
}) {
  const openPlayer = useUIStore(state => state.openPlayer)
  const progress = watchProgressPercent(item.position, item.duration)

  return (
    <div className="relative h-full min-w-0">
      <Button
        type="button"
        variant="ghost"
        className={cn(
          'block h-full w-full min-w-0 rounded-2xl p-0 text-left whitespace-normal hover:bg-transparent hover:text-current dark:hover:bg-transparent',
          selected && 'ring-2 ring-success'
        )}
        disabled={disabled}
        onClick={selecting ? onSelect : () => openPlayer(item.movie_id)}
      >
        <MovieCard
          movie={item}
          coverOverlay={
            <div className="absolute right-2 bottom-2 left-2">
              <Progress value={progress} variant="success" className="h-1.5 bg-black/40" />
            </div>
          }
          description={
            <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
              <time dateTime={item.watched_at}>{dateFormat.format(new Date(item.watched_at))}</time>
              <span className="tabular-nums">
                {item.duration > 0
                  ? `${formatWatchTime(item.position)} / ${formatWatchTime(item.duration)}`
                  : '尚未播放'}
              </span>
            </div>
          }
        />
      </Button>
      {selecting ? (
        <Checkbox
          checked={selected}
          disabled={disabled}
          onCheckedChange={onSelect}
          className="absolute top-3 right-3 size-5 data-checked:border-success data-checked:bg-success data-checked:text-white"
        />
      ) : null}
    </div>
  )
}
