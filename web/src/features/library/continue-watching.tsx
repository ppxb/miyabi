import { useScrollFade } from '@animate-ui/react/primitives/effects/scroll-fade'
import { Link } from '@tanstack/react-router'
import { ChevronLeftIcon, ChevronRightIcon } from 'lucide-react'
import { useRef } from 'react'

import { useHistory } from '@/api/history'
import type { LibraryMovie } from '@/api/library'
import { Button } from '@/components/ui/button'
import { LibraryMovieCard } from '@/features/library/movie-card'

export function ContinueWatching({ onPlay }: { onPlay: (movieID: number) => void }) {
  const history = useHistory(1)
  const listRef = useRef<HTMLDivElement>(null)
  useScrollFade(listRef, { orientation: 'horizontal', blur: 4, size: 48 })

  const items = (history.data?.items ?? [])
    .filter(item => item.duration > 0 && item.position > 0 && item.position / item.duration < 0.95)
    .slice(0, 12)
  if (items.length === 0) return null

  const scroll = (direction: -1 | 1) => {
    const list = listRef.current
    if (!list) return
    list.scrollBy({ left: direction * list.clientWidth * 0.8, behavior: 'smooth' })
  }

  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-lg font-semibold tracking-normal">正在观看</h2>
        <div className="flex items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="向左滚动"
            onClick={() => scroll(-1)}
          >
            <ChevronLeftIcon />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="向右滚动"
            onClick={() => scroll(1)}
          >
            <ChevronRightIcon />
          </Button>
          <Button variant="link" size="sm" nativeButton={false} render={<Link to="/history" />}>
            查看全部
          </Button>
        </div>
      </div>
      <div
        ref={listRef}
        className="flex snap-x snap-mandatory [scrollbar-width:none] gap-3 overflow-x-auto pb-2 [&::-webkit-scrollbar]:hidden"
      >
        {items.map(item => {
          const percent = item.duration > 0 ? (item.position / item.duration) * 100 : 0
          const movie: LibraryMovie = {
            id: item.movie_id,
            code: item.code,
            title: item.title,
            cover: item.cover,
            poster: item.poster,
            duration: item.duration,
            rating: item.rating ?? 0,
            actors: item.actors ?? [],
            tags: [],
            scrape_status: 'done'
          }
          return (
            <div
              key={item.movie_id}
              className="w-full shrink-0 snap-start sm:w-[calc((100%-1rem)/2)] lg:w-[calc((100%-3rem)/3)] xl:w-[calc((100%-4.5rem)/4)]"
            >
              <LibraryMovieCard
                movie={movie}
                progress={percent}
                onPlay={() => onPlay(item.movie_id)}
              />
            </div>
          )
        })}
      </div>
    </section>
  )
}
