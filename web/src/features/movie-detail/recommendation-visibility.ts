export function observeRecommendation(element: Element, request: () => () => void) {
  let timer: ReturnType<typeof setTimeout> | undefined
  let release: (() => void) | undefined
  function leave() {
    clearTimeout(timer)
    release?.()
    release = undefined
  }
  const observer = new IntersectionObserver(
    entries => {
      if (!entries.some(entry => entry.isIntersecting)) {
        leave()
        return
      }
      clearTimeout(timer)
      timer = setTimeout(() => {
        release ??= request()
      }, 200)
    },
    { rootMargin: '200px 0px' }
  )
  observer.observe(element)
  return () => {
    leave()
    observer.disconnect()
  }
}
