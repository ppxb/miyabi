import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPut } from './client'

export type ScrapingSource = { id: string; enabled: boolean }
const sourcesKey = ['settings', 'scraping'] as const

export const sourceNames: Record<string, string> = {
  fanza: 'FANZA',
  fc2: 'FC2 官方',
  fc2db: 'FC2 备用库',
  heyzo: 'HEYZO 官方',
  pacopacomama: 'Pacopacomama 官方',
  javdb: 'JavDB',
  javbus: 'JavBus',
  jav321: 'Jav321',
  tokyohot: 'Tokyo-Hot',
  heydouga: 'Hey動画',
  caribbeancom: 'Caribbeancom',
  caribbeancompr: 'Caribbeancom Premium',
  c0930: 'C0930',
  h0930: 'H0930',
  h4610: 'H4610',
  '10musume': '10musume',
  muramura: 'ムラムラ',
  dahlia: 'Dahlia',
  duga: 'DUGA',
  r18dev: 'r18.dev',
  onepondo: '一本道',
  codeav: 'CodeAV',
  freejavbt: 'FreeJavBT'
}

export function useScrapingSources() {
  return useQuery({
    queryKey: sourcesKey,
    queryFn: ({ signal }) => apiGet<ScrapingSource[]>('/api/settings/scraping', undefined, signal)
  })
}

export function useUpdateScrapingSources() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (sources: ScrapingSource[]) =>
      apiPut<ScrapingSource[]>('/api/settings/scraping', sources),
    onSuccess: sources => {
      client.setQueryData(sourcesKey, sources)
    }
  })
}

export type ActorSource = { id: string; enabled: boolean }
const actorSourcesKey = ['settings', 'actor-sources'] as const

export const actorSourceNames: Record<string, string> = {
  'av-league': 'AV-LEAGUE',
  gfriends: 'gfriends'
}

export function useActorSources() {
  return useQuery({
    queryKey: actorSourcesKey,
    queryFn: ({ signal }) => apiGet<ActorSource[]>('/api/settings/actor-sources', undefined, signal)
  })
}

export function useUpdateActorSources() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (sources: ActorSource[]) =>
      apiPut<ActorSource[]>('/api/settings/actor-sources', sources),
    onSuccess: sources => {
      client.setQueryData(actorSourcesKey, sources)
    }
  })
}
