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
  r18dev: 'r18.dev',
  onepondo: '一本道',
  javdatabase: 'JavDatabase',
  codeav: 'CodeAV'
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
