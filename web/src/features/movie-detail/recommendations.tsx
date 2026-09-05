import { Link } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'

import { type MovieReference, useDiscoverMovie } from '@/api/discover'
import { MovieCard, MovieCardSkeleton, MovieGridLayout } from '@/components/movie'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

export function MovieRecommendations({
  id,
  title,
  movies
}: {
  id: string
  title: string
  movies: MovieReference[]
}) {
  const visible = movies.slice(0, 8)
  const sectionRef = useRef<HTMLElement>(null)
  const [loadDetails, setLoadDetails] = useState(false)

  useEffect(() => {
    const observer = new IntersectionObserver(
      entries => {
        if (entries.some(entry => entry.isIntersecting)) {
          setLoadDetails(true)
          observer.disconnect()
        }
      },
      { rootMargin: '200px 0px' }
    )
    observer.observe(sectionRef.current!)
    return () => observer.disconnect()
  }, [])

  return (
    <section ref={sectionRef} aria-labelledby={id} className="space-y-4">
      <h2 id={id} className="text-xl font-semibold tracking-normal">
        {title}
      </h2>
      {visible.length === 0 ? (
        <p className="text-sm text-muted-foreground">暂无相关影片</p>
      ) : (
        <MovieGridLayout>
          {visible.map(movie => (
            <RecommendationCard key={movie.id} movie={movie} loadDetail={loadDetails} />
          ))}
        </MovieGridLayout>
      )}
    </section>
  )
}

function RecommendationCard({ movie, loadDetail }: { movie: MovieReference; loadDetail: boolean }) {
  const detail = useDiscoverMovie(movie.id, loadDetail)

  if (detail.data) return <MovieCard movie={detail.data} />
  if (!detail.isError) return <MovieCardSkeleton />

  return (
    <Card size="sm" className="h-full gap-0 overflow-hidden py-0">
      <div className="flex aspect-3/2 items-center justify-center bg-muted text-sm text-muted-foreground">
        详情加载失败
      </div>
      <CardContent className="flex min-h-24 items-center justify-between gap-2 p-3">
        <Link
          to="/discover/$movieId"
          params={{ movieId: movie.id }}
          className="text-sm outline-ring"
        >
          {movie.code}
        </Link>
        <Button type="button" variant="outline" size="sm" onClick={() => detail.refetch()}>
          重试
        </Button>
      </CardContent>
    </Card>
  )
}
