export const MAX_PAGE = 100_000_000

export function parseSearchPage(raw: unknown): number | undefined {
  if (raw === undefined || raw === null || raw === '') return undefined
  const page = Number(raw)
  if (!Number.isInteger(page) || page < 1 || page > MAX_PAGE) {
    throw new Error('页码无效')
  }
  return page > 1 ? page : undefined
}
