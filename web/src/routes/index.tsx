import { createFileRoute } from "@tanstack/react-router"

import { useHealth } from "@/api/health"

export const Route = createFileRoute("/")({
  component: LibraryPage,
})

function LibraryPage() {
  const health = useHealth()

  return (
    <section className="flex flex-1 flex-col">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm text-zinc-500">115 媒体索引</p>
          <h1 className="mt-1 text-2xl font-semibold text-zinc-100">媒体库</h1>
        </div>
        <div className="flex items-center gap-2 text-xs text-zinc-500">
          <span
            className={`size-2 rounded-full ${health.data?.status === "ok" ? "bg-emerald-400" : health.isError ? "bg-red-400" : "bg-amber-400"}`}
          />
          {health.data?.status === "ok"
            ? "后端已连接"
            : health.isError
              ? "后端不可用"
              : "正在连接"}
        </div>
      </div>

      <div className="mt-8 flex flex-1 items-center justify-center rounded-xl border border-dashed border-zinc-800 bg-zinc-950/35 px-6 py-20 text-center">
        <div>
          <p className="text-sm font-medium text-zinc-300">媒体库还是空的</p>
          <p className="mt-2 max-w-md text-sm leading-6 text-zinc-500">
            后续接入 115 并扫描影片目录后，已入库内容会显示在这里。
          </p>
        </div>
      </div>
    </section>
  )
}
