import { Link, type LinkProps } from '@tanstack/react-router'
import { ChevronLeftIcon, ChevronRightIcon } from 'lucide-react'
import { useEffect, useRef, useState, type ReactNode } from 'react'

import { Button } from '@/components/ui/button'

export function MovieShelf({
  title,
  itemCount,
  href,
  children
}: {
  title: string
  itemCount: number
  href?: LinkProps['to']
  children: ReactNode
}) {
  if (itemCount === 0) return null
  return (
    <MovieShelfContent title={title} itemCount={itemCount} href={href}>
      {children}
    </MovieShelfContent>
  )
}

function MovieShelfContent({
  title,
  itemCount,
  href,
  children
}: {
  title: string
  itemCount: number
  href?: LinkProps['to']
  children: ReactNode
}) {
  const listRef = useRef<HTMLDivElement>(null)
  const [overflow, setOverflow] = useState(false)
  const [atStart, setAtStart] = useState(true)
  const [atEnd, setAtEnd] = useState(true)

  useEffect(() => {
    const list = listRef.current
    if (!list) return
    const update = () => {
      const maxScroll = list.scrollWidth - list.clientWidth
      const edge = 16
      setOverflow(maxScroll > 1)
      setAtStart(list.scrollLeft <= edge)
      setAtEnd(list.scrollLeft >= maxScroll - edge)
    }
    update()
    const observer = new ResizeObserver(update)
    observer.observe(list)
    list.addEventListener('scroll', update, { passive: true })
    return () => {
      observer.disconnect()
      list.removeEventListener('scroll', update)
    }
  }, [itemCount])

  const scroll = (direction: -1 | 1) => {
    const list = listRef.current
    if (!list) return
    list.scrollBy({ left: direction * list.clientWidth, behavior: 'smooth' })
  }

  return (
    <section className="min-w-0 space-y-3">
      <div className="flex items-end justify-between gap-3">
        <h2 className="text-lg font-semibold tracking-normal">{title}</h2>
        <div className="flex items-center gap-1">
          {overflow ? (
            <div className="hidden items-center gap-1 sm:flex">
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="向左滚动"
                disabled={atStart}
                onClick={() => scroll(-1)}
              >
                <ChevronLeftIcon />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="向右滚动"
                disabled={atEnd}
                onClick={() => scroll(1)}
              >
                <ChevronRightIcon />
              </Button>
            </div>
          ) : null}
          {href ? (
            <Button variant="default" size="sm" nativeButton={false} render={<Link to={href} />}>
              查看全部
            </Button>
          ) : null}
        </div>
      </div>
      <div
        ref={listRef}
        className="-mx-4 flex snap-x snap-mandatory scroll-px-4 no-scrollbar gap-3 overflow-x-auto px-4 pt-1 pb-4 sm:gap-4 sm:py-1"
      >
        {children}
      </div>
    </section>
  )
}
