const sizeFormat = new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 2 })

export function formatSize(bytes: number) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = bytes
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit++
  }
  return `${sizeFormat.format(size)} ${units[unit]}`
}
