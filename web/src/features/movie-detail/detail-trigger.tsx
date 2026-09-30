import { cn } from 'cn'
import { useContext, type ComponentProps } from 'react'

import { MovieDetailDialogContext } from './dialog-context'

export function MovieDetailTrigger({
  movieId,
  disabled = false,
  role = 'button',
  tabIndex = 0,
  className,
  onClick,
  onKeyDown,
  'aria-haspopup': hasPopup = 'dialog',
  ...props
}: ComponentProps<'div'> & { movieId: string; disabled?: boolean }) {
  const openMovie = useContext(MovieDetailDialogContext)
  if (!openMovie) throw new Error('MovieDetailDialogProvider is missing')

  // Cards contain their own action buttons, so the wrapper must not be a native button.
  return (
    <div
      {...props}
      role={role}
      tabIndex={disabled ? -1 : tabIndex}
      aria-disabled={disabled || undefined}
      aria-haspopup={hasPopup}
      className={cn('cursor-pointer', className)}
      onClick={event => {
        if (disabled) return
        onClick?.(event)
        if (event.defaultPrevented) return
        event.preventDefault()
        openMovie(movieId, event.currentTarget)
      }}
      onKeyDown={event => {
        onKeyDown?.(event)
        if (disabled || event.defaultPrevented || event.target !== event.currentTarget) return
        if (event.key !== 'Enter' && event.key !== ' ') return
        event.preventDefault()
        if (!event.repeat) event.currentTarget.click()
      }}
    />
  )
}
