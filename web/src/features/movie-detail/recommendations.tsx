import { Link } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'

import { type MovieReference, useDiscoverMovie } from '@/api/discover'
import { DiscoverMovieCard, MovieCardSkeleton, MovieGridLayout } from '@/components/movie'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

export function MovieRecommendations({
  title,
  movies
}: {
  title: string
  movies: MovieReference[]
}) {
  const visible = movies.slice(0, 8)

  return (
    <section className="space-y-4">
      <h2 className="text-xl font-semibold tracking-normal">{title}</h2>
      {visible.length === 0 ? (
        <p className="text-sm text-muted-foreground">暂无相关影片</p>
      ) : (
        <MovieGridLayout>
          {visible.map(movie => (
            <RecommendationCard key={movie.id} movie={movie} />
          ))}
        </MovieGridLayout>
      )}
    </section>
  )
}

function RecommendationCard({ movie }: { movie: MovieReference }) {
  const cardRef = useRef<HTMLDivElement>(null)
  const [loadDetail, setLoadDetail] = useState(false)

  useEffect(() => {
    const observer = new IntersectionObserver(
      entries => {
        if (entries.some(entry => entry.isIntersecting)) {
          setLoadDetail(true)
          observer.disconnect()
        }
      },
      { rootMargin: '200px 0px' }
    )
    observer.observe(cardRef.current!)
    return () => observer.disconnect()
  }, [])

  return (
    <div ref={cardRef} className="h-full min-w-0">
      <RecommendationContent movie={movie} loadDetail={loadDetail} />
    </div>
  )
}

function RecommendationContent({
  movie,
  loadDetail
}: {
  movie: MovieReference
  loadDetail: boolean
}) {
  const detail = useDiscoverMovie(movie.id, loadDetail)

  if (detail.data) return <DiscoverMovieCard movie={detail.data} />
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
