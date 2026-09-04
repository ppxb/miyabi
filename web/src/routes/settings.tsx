import { createFileRoute } from "@tanstack/react-router"

export const Route = createFileRoute("/settings")({
  component: SettingsPage,
})

function SettingsPage() {
  return (
    <section>
      <p className="text-sm text-zinc-500">系统与外部服务</p>
      <h1 className="mt-1 text-2xl font-semibold text-zinc-100">设置</h1>
      <p className="mt-4 text-sm text-zinc-500">115 登录与 JavDB 线路设置将在后续阶段接入。</p>
    </section>
  )
}
