import type { ReactNode } from 'react'

import type { JavDBEntityType, JavDBZone } from '@/api/discover'
import { Badge } from '@/components/ui/badge'
import { MetadataLink } from './metadata-link'

type MetadataEntity = { id?: string; name: string }

export type MovieMetadataValues = {
  maker?: MetadataEntity
  series?: MetadataEntity
  director?: MetadataEntity
  actors: MetadataEntity[]
  tags: MetadataEntity[]
  zone?: JavDBZone | 'unknown'
}

export function MovieMetadata({ movie }: { movie: MovieMetadataValues }) {
  return (
    <dl className="space-y-3 text-sm">
      {movie.maker ? (
        <EntityRow label="厂牌">
          <EntityLink kind="maker" entity={movie.maker} />
        </EntityRow>
      ) : null}
      {movie.series ? (
        <EntityRow label="系列">
          <EntityLink kind="series" entity={movie.series} />
        </EntityRow>
      ) : null}
      {movie.actors.length > 0 ? (
        <EntityRow label="演员">
          {movie.actors.map((actor, index) => (
            <span
              key={actor.id || `${actor.name}:${index}`}
              className="inline-flex max-w-full items-center gap-3"
            >
              {index > 0 ? <span className="text-muted-foreground">/</span> : null}
              <EntityLink kind="actor" entity={actor} />
            </span>
          ))}
        </EntityRow>
      ) : null}
      {movie.director ? (
        <EntityRow label="导演">
          <EntityLink kind="director" entity={movie.director} />
        </EntityRow>
      ) : null}
      {movie.tags.length > 0 ? (
        <div className="flex items-start gap-4">
          <dt className="w-10 shrink-0 pt-0.5 leading-5 text-muted-foreground">标签</dt>
          <dd className="flex min-w-0 flex-wrap gap-2">
            {movie.tags.map((tag, index) =>
              !tag.id || movie.zone === 'unknown' ? (
                <Badge
                  key={tag.id || `${tag.name}:${index}`}
                  variant="outline"
                  className="h-auto max-w-full whitespace-normal break-words"
                >
                  {tag.name}
                </Badge>
              ) : (
                <MetadataLink
                  key={tag.id}
                  kind="tag"
                  id={tag.id}
                  name={tag.name}
                  zone={movie.zone}
                  badge
                />
              )
            )}
          </dd>
        </div>
      ) : null}
    </dl>
  )
}

function EntityLink({ kind, entity }: { kind: JavDBEntityType; entity: MetadataEntity }) {
  return entity.id ? (
    <MetadataLink kind={kind} id={entity.id} name={entity.name} />
  ) : (
    <span className="max-w-full break-words">{entity.name}</span>
  )
}

function EntityRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-start gap-4">
      <dt className="w-10 shrink-0 leading-6 text-muted-foreground">{label}</dt>
      <dd className="flex min-w-0 flex-wrap gap-x-3 gap-y-1 leading-6">{children}</dd>
    </div>
  )
}
