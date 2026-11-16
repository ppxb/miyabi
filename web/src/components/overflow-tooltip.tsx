import { useId, useState, type ReactElement, type ReactNode } from 'react'

import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

type OverflowTooltipProps = {
  children: ReactElement
  content: ReactNode
}

function isOverflowing(element: HTMLElement) {
  return (
    element.scrollWidth > element.clientWidth + 1 || element.scrollHeight > element.clientHeight + 1
  )
}

export function OverflowTooltip({ children, content }: OverflowTooltipProps) {
  const triggerId = useId()
  const [open, setOpen] = useState(false)

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setOpen(false)
      return
    }
    const element = document.getElementById(triggerId)
    setOpen(element instanceof HTMLElement && isOverflowing(element))
  }

  return (
    <Tooltip open={open} onOpenChange={handleOpenChange} triggerId={triggerId}>
      <TooltipTrigger asChild id={triggerId}>
        {children}
      </TooltipTrigger>
      <TooltipContent>{content}</TooltipContent>
    </Tooltip>
  )
}
