import type { NamedEntity } from '@/api/discover'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'

export function CommonFilterSelect({
  options,
  value,
  onValueChange
}: {
  options: NamedEntity[]
  value: string
  onValueChange: (value: string) => void
}) {
  const items = [
    { value: 'all', label: '全部条件' },
    ...options.map(option => ({ value: option.id, label: option.name }))
  ]
  return (
    <Select
      value={value || 'all'}
      items={items}
      onValueChange={next => onValueChange(next === 'all' ? '' : (next ?? ''))}
    >
      <SelectTrigger className="w-full sm:w-48">
        <SelectValue placeholder="全部条件" />
      </SelectTrigger>
      <SelectContent align="start">
        <SelectGroup>
          <SelectItem value="all">全部条件</SelectItem>
          {options.map(option => (
            <SelectItem key={option.id} value={option.id}>
              {option.name}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}
