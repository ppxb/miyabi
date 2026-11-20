import { Link } from '@tanstack/react-router'
import { LockKeyholeIcon, SearchIcon, XIcon } from 'lucide-react'
import { type FormEvent, useState } from 'react'

import { describeApiError, isJavDBLoginRequired } from '@/api/client'
import { useSearchMovies } from '@/api/discover'
import { AppPage } from '@/components/app-page'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
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
  if (movies.isError && isJavDBLoginRequired(movies.error)) {
    return <JavDBLoginRequired />
  }
  return (
    <DiscoverResults
      movies={movies.data}
      loading={movies.isPending}
      fetching={movies.isFetching}
      error={movies.isError}
      errorMessage={movies.error ? describeApiError(movies.error) : undefined}
      searching
      page={page}
      onPageChange={onPageChange}
      onRetry={() => movies.refetch()}
    />
  )
}

function JavDBLoginRequired() {
  return (
    <div className="flex min-h-80 w-full flex-col items-center justify-center gap-3 text-center">
      <div className="flex size-14 items-center justify-center rounded-full bg-muted">
        <LockKeyholeIcon className="size-7 text-muted-foreground" />
      </div>
      <div className="space-y-1">
        <p className="text-sm font-medium">需要登录 JavDB 才能搜索</p>
        <p className="text-xs leading-5 text-muted-foreground">
          请先在「设置 → JavDB 账号」填写用户名和密码，保存后再回来搜索。
        </p>
      </div>
      <Button asChild size="sm">
        <Link to="/settings">去设置</Link>
      </Button>
    </div>
  )
}
