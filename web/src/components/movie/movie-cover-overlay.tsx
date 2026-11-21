import { StarIcon } from 'lucide-react'
import type { ReactNode } from 'react'

/** Rating pill shown at the top-right of a cover; renders nothing for unrated. */
export function MovieRatingPill({ rating }: { rating: number }) {
  if (!(rating > 0)) return null
  return (
    <div className="pointer-events-none absolute top-2 right-2 flex items-center gap-1 rounded-full bg-black/55 px-2 py-0.5 text-xs font-semibold text-white backdrop-blur-sm">
      <StarIcon className="size-3.5 fill-yellow-400 text-yellow-400" />
      <span className="tabular-nums">{rating.toFixed(1)}</span>
    </div>
  )
}

/** Gradient caption at the bottom of a cover: title, an optional subtitle line,
 * plus a feature-specific slot. */
export function MovieCoverCaption({
  title,
  subtitle,
  children
}: {
  title: string
  subtitle?: string | undefined
  children?: ReactNode
}) {
  return (
    <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-linear-to-t from-black/85 via-black/45 to-transparent p-3 pt-8">
      <h3 className="truncate text-sm leading-5 font-semibold text-white" title={title}>
        {title}
      </h3>
      {subtitle ? <p className="mt-0.5 truncate text-xs text-white/75">{subtitle}</p> : null}
      {children}
    </div>
  )
}
