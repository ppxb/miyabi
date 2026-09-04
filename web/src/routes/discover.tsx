import { createFileRoute } from "@tanstack/react-router"

export const Route = createFileRoute("/discover")({
  component: DiscoverPage,
})

function DiscoverPage() {
  return (
    <section>
      <p className="text-sm text-zinc-500">JavDB</p>
      <h1 className="mt-1 text-2xl font-semibold text-zinc-100">发现</h1>
      <p className="mt-4 text-sm text-zinc-500">JavDB 发现功能将在 Stage 3 接入。</p>
    </section>
  )
}
