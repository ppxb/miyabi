import { Fragment, useRef, useState, type ReactNode } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { SearchIcon, type LucideIcon } from 'lucide-react'
import { useScrollFade } from '@animate-ui/react/primitives/effects/scroll-fade'

import { APP_NAV_ITEMS } from '@/components/app-navigation'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator
} from '@/components/ui/command'
import { Kbd, KbdGroup } from '@/components/ui/kbd'

const MODIFIER_KEY =
  typeof navigator !== 'undefined' && /Mac|iPhone|iPad|iPod/.test(navigator.platform) ? '⌘' : 'Ctrl'

type SearchItem = {
  id: string
  group: string
  label: string
  icon?: LucideIcon
  run: () => void
}

function groupBy(items: SearchItem[]) {
  const groups = [...new Set(items.map(item => item.group))]
  return groups.map(group => ({
    group,
    items: items.filter(item => item.group === group)
  }))
}

/**
 * The result list is the scroll container, so the fade is attached to it. A component of its own
 * keeps the hook mounting together with the dialog: while the palette is closed the list is not in
 * the DOM and the fade would never attach.
 */
function FadingCommandList({ children }: { children: ReactNode }) {
  const listRef = useRef<HTMLDivElement>(null)

  useScrollFade(listRef, { blur: 4 })

  return <CommandList ref={listRef}>{children}</CommandList>
}

/** Header trigger; the palette itself is mounted once by the shell and opened through props. */
export function SearchTrigger({ onOpen }: { onOpen: () => void }) {
  return (
    <Button
      onClick={onOpen}
      variant="link"
      className="px-0! font-normal text-muted-foreground hover:no-underline"
    >
      <SearchIcon data-icon="inline-start" />
      搜索
      <KbdGroup>
        <Kbd>{MODIFIER_KEY}</Kbd>
        <Kbd>J</Kbd>
      </KbdGroup>
    </Button>
  )
}

export function SearchDialog({
  open,
  onOpenChange
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const keyword = query.trim()

  const handleOpenChange = (value: boolean) => {
    onOpenChange(value)
    if (!value) setQuery('')
  }

  const items: SearchItem[] = APP_NAV_ITEMS.map(item => ({
    id: item.to,
    group: '页面',
    label: item.label,
    icon: item.icon,
    run: () => {
      void navigate({ to: item.to })
    }
  }))

  if (keyword) {
    items.push({
      id: 'movie-search',
      group: '影片',
      label: `搜索影片 “${keyword}”`,
      icon: SearchIcon,
      run: () => {
        void navigate({ to: '/search', search: { q: keyword } })
      }
    })
  }

  const groupedItems = groupBy(items)

  const handleSelect = (item: SearchItem) => {
    handleOpenChange(false)
    item.run()
  }

  return (
    <CommandDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="搜索"
      description="搜索目录或页面"
    >
      <Command>
        <CommandInput placeholder="搜索目录或页面" value={query} onValueChange={setQuery} />
        <FadingCommandList>
          <CommandEmpty>没有找到结果</CommandEmpty>
          {groupedItems.map(({ group, items: groupItems }, index) => (
            <Fragment key={group}>
              {index > 0 && <CommandSeparator />}
              <CommandGroup heading={group}>
                {groupItems.map(item => (
                  <CommandItem
                    key={item.id}
                    value={`${item.group} ${item.label}`}
                    onSelect={() => handleSelect(item)}
                  >
                    <span className="flex min-w-0 items-center gap-2">
                      {item.icon && <item.icon />}
                      <span className="truncate">{item.label}</span>
                    </span>
                  </CommandItem>
                ))}
              </CommandGroup>
            </Fragment>
          ))}
        </FadingCommandList>
      </Command>
    </CommandDialog>
  )
}
