import { useRef, useState, type ReactElement, type ReactNode } from 'react'

import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

type OverflowTooltipProps = {
  children: ReactElement
  content: ReactNode
}

export function OverflowTooltip({ children, content }: OverflowTooltipProps) {
  const triggerRef = useRef<HTMLElement>(null)
  const [open, setOpen] = useState(false)

  function handleOpenChange(nextOpen: boolean) {
    const element = triggerRef.current
    setOpen(
      nextOpen &&
        element !== null &&
        (element.scrollWidth > element.clientWidth + 1 ||
          element.scrollHeight > element.clientHeight + 1)
    )
  }

  return (
    <Tooltip open={open} onOpenChange={handleOpenChange}>
      <TooltipTrigger
        asChild
        ref={element => {
          triggerRef.current = element
        }}
      >
        {children}
      </TooltipTrigger>
      <TooltipContent>{content}</TooltipContent>
    </Tooltip>
  )
}
