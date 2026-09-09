import { SearchIcon, XIcon } from 'lucide-react'
import { type FormEvent, useState } from 'react'

import { useSearchMovies } from '@/api/discover'
import { AppPage } from '@/components/app-page'
import { PageHeader } from '@/components/page-header'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput
} from '@/components/ui/input-group'
import { DISCOVER_PAGE_SIZE } from '@/features/discover/constants'
import { DiscoverResults } from '@/features/discover/results'

export function SearchPage({
  keyword,
  page,
  onSearch,
  onPageChange
}: {
  keyword: string
  page: number
  onSearch: (keyword: string) => void
  onPageChange: (page: number) => void
}) {
  return (
    <AppPage>
      <PageHeader title="搜索" description="搜索 JavDB 的所有影片" />
      <SearchForm key={`form:${keyword}`} keyword={keyword} onSearch={onSearch} />
      {keyword ? (
        <SearchResults
          key={`results:${keyword}`}
          keyword={keyword}
          page={page}
          onPageChange={onPageChange}
        />
      ) : null}
    </AppPage>
  )
}

function SearchForm({
  keyword,
  onSearch
}: {
  keyword: string
  onSearch: (keyword: string) => void
}) {
  const [draft, setDraft] = useState(keyword)

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    onSearch(draft.trim())
  }

  return (
    <form className="w-full max-w-xl" onSubmit={submit}>
      <InputGroup>
        <InputGroupAddon>
          <InputGroupButton type="submit" size="icon-xs">
            <SearchIcon className="size-4" />
          </InputGroupButton>
        </InputGroupAddon>
        <InputGroupInput
          type="text"

          inputMode="search"
          enterKeyHint="search"
          value={draft}
          onChange={event => setDraft(event.target.value)}
          placeholder="搜索番号、标题或演员"
        />
        {draft ? (
          <InputGroupAddon align="inline-end">
            <InputGroupButton
              size="icon-xs"
              onClick={() => {
                setDraft('')
                onSearch('')
              }}
            >
              <XIcon />
            </InputGroupButton>
          </InputGroupAddon>
        ) : null}
      </InputGroup>
    </form>
  )
}

function SearchResults({
  keyword,
  page,
  onPageChange
}: {
  keyword: string
  page: number
  onPageChange: (page: number) => void
}) {
  const movies = useSearchMovies({ query: keyword, page, limit: DISCOVER_PAGE_SIZE })
  return (
    <DiscoverResults
      movies={movies.data}
      loading={movies.isPending || movies.isPlaceholderData}
      fetching={movies.isFetching}
      error={movies.isError}
      searching
      page={page}
      onPageChange={onPageChange}
      onRetry={() => movies.refetch()}
    />
  )
}
