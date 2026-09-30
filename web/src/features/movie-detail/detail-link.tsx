import { Link } from '@tanstack/react-router'
import { useContext, type ComponentProps } from 'react'

import { MovieDetailDialogContext } from './dialog-context'

export function MovieDetailLink({
  movieId,
  onClick,
  ...props
}: Omit<ComponentProps<'a'>, 'href'> & { movieId: string }) {
  const openMovie = useContext(MovieDetailDialogContext)

  return (
    <Link
      to="/discover/$movieId"
      params={{ movieId }}
      search={previous => ({ main: previous.main || undefined })}
      aria-haspopup="dialog"
      {...props}
      onClick={event => {
        onClick?.(event)
        if (
          !openMovie ||
          event.defaultPrevented ||
          event.button !== 0 ||
          event.metaKey ||
          event.ctrlKey ||
          event.shiftKey ||
          event.altKey ||
          (event.currentTarget.target && event.currentTarget.target !== '_self')
        )
          return

        event.preventDefault()
        openMovie(movieId, event.currentTarget)
      }}
    />
  )
}
