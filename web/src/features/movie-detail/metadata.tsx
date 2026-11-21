import type { ReactNode } from 'react'

import type { Zone } from '@/api/discover'
import { Badge } from '@/components/ui/badge'

type MetadataEntity = { provider?: string; id?: string; name: string }

export type MovieMetadataValues = {
  maker?: MetadataEntity
  series?: MetadataEntity
  director?: MetadataEntity
  actors: MetadataEntity[]
  tags: MetadataEntity[]
  zone?: Zone | 'unknown'
}

export function MovieMetadata({ movie }: { movie: MovieMetadataValues }) {
  const actors = movie.actors ?? []
  const tags = movie.tags ?? []
  return (
    <dl className="space-y-3 text-sm">
      <EntityRow label="厂牌">
        {movie.maker ? (
          <EntityLink entity={movie.maker} />
        ) : (
          <span className="text-muted-foreground">N/A</span>
        )}
      </EntityRow>
      <EntityRow label="系列">
        {movie.series ? (
          <EntityLink entity={movie.series} />
        ) : (
          <span className="text-muted-foreground">N/A</span>
        )}
      </EntityRow>
      {actors.length > 0 ? (
        <EntityRow label="演员">
          {actors.map((actor, index) => (
            <span
              key={`${actor.provider}:${actor.id || actor.name}:${index}`}
              className="inline-flex max-w-full items-center gap-3"
            >
              {index > 0 ? <span className="text-muted-foreground">/</span> : null}
              <EntityLink entity={actor} />
            </span>
          ))}
        </EntityRow>
      ) : null}
      <EntityRow label="导演">
        {movie.director ? (
          <EntityLink entity={movie.director} />
        ) : (
          <span className="text-muted-foreground">N/A</span>
        )}
      </EntityRow>
      {tags.length > 0 ? (
        <div className="flex items-start gap-4">
          <dt className="w-10 shrink-0 pt-0.5 leading-5 text-muted-foreground">标签</dt>
          <dd className="flex min-w-0 flex-wrap gap-2">
            {tags.map((tag, index) => (
              <Badge
                key={`${tag.provider}:${tag.id || tag.name}:${index}`}
                variant="outline"
                className="h-auto max-w-full break-words whitespace-normal"
              >
                {tag.name}
              </Badge>
            ))}
          </dd>
        </div>
      ) : null}
    </dl>
  )
}

function EntityLink({ entity }: { entity: MetadataEntity }) {
  return <span className="max-w-full break-words">{entity.name}</span>
}

function EntityRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-start gap-4">
      <dt className="w-10 shrink-0 leading-6 text-muted-foreground">{label}</dt>
      <dd className="flex min-w-0 flex-wrap gap-x-3 gap-y-1 leading-6">{children}</dd>
    </div>
  )
}
