import { cn } from 'cn'
import { getPageNumbers } from '@/lib/pagination'
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious
} from '@/components/ui/pagination'

export function ListPagination({
  page,
  totalPages,
  hasMore,
  disabled,
  scrollToTop = true,
  onPageChange
}: {
  page: number
  totalPages?: number
  hasMore: boolean
  disabled: boolean
  scrollToTop?: boolean
  onPageChange: (page: number) => void
}) {
  function changePage(nextPage: number) {
    if (nextPage === page) return
    onPageChange(nextPage)
    if (scrollToTop) window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  function renderPage(item: number) {
    if (item === page) {
      return (
        <PaginationItem key={item}>
          <PaginationLink isActive>{item}</PaginationLink>
        </PaginationItem>
      )
    }
    return (
      <PaginationItem key={item}>
        <PaginationLink
          href="#"
          tabIndex={disabled ? -1 : undefined}
          className={cn(disabled && 'pointer-events-none opacity-50')}
          onClick={e => {
            e.preventDefault()
            changePage(item)
          }}
        >
          {item}
        </PaginationLink>
      </PaginationItem>
    )
  }

  const singlePage = page <= 1 && !hasMore && (totalPages === undefined || totalPages <= 1)
  if (singlePage) return null

  const items = totalPages === undefined ? [page] : getPageNumbers(page, totalPages)

  return (
    <Pagination className="py-3">
      <PaginationContent>
        <PaginationItem>
          <PaginationPrevious
            text="上一页"
            disabled={page <= 1 || disabled}
            onClick={() => changePage(page - 1)}
          />
        </PaginationItem>

        {items.map(item =>
          typeof item === 'number' ? (
            renderPage(item)
          ) : (
            <PaginationItem key={item}>
              <PaginationEllipsis />
            </PaginationItem>
          )
        )}

        <PaginationItem>
          <PaginationNext
            text="下一页"
            disabled={disabled || !hasMore}
            onClick={() => changePage(page + 1)}
          />
        </PaginationItem>
      </PaginationContent>
    </Pagination>
  )
}
