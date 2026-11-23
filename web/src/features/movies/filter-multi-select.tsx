import { useState } from 'react'
import { ChevronsUpDownIcon } from 'lucide-react'

import type { LibraryFilterOption } from '@/api/library'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

export function FilterMultiSelect({
  label,
  options,
  value,
  onChange
}: {
  label: string
  options: LibraryFilterOption[]
  value: string[]
  onChange: (next: string[]) => void
}) {
  const [open, setOpen] = useState(false)
  const toggle = (id: string) =>
    onChange(value.includes(id) ? value.filter(item => item !== id) : [...value, id])

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button type="button" variant="outline" className="min-w-24 justify-between gap-2" />
        }
      >
        <span className="min-w-0 truncate">
          {label}
          {value.length > 0 ? ` · ${value.length}` : ''}
        </span>
        <ChevronsUpDownIcon className="size-3.5 shrink-0 opacity-60" />
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 p-0">
        <Command className="p-0">
          <CommandInput placeholder={`搜索${label}`} />
          <CommandList className="max-h-64 p-1">
            <CommandEmpty>无匹配项</CommandEmpty>
            {options.map(option => (
              <CommandItem
                key={option.id}
                value={option.name}
                data-checked={value.includes(option.id)}
                onSelect={() => toggle(option.id)}
                className="px-3"
              >
                <span className="min-w-0 flex-1 truncate" title={option.name}>
                  {option.name}
                </span>
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
