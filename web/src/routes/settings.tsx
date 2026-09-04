import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { PageHeader } from '@/components/page-header'

export const Route = createFileRoute('/settings')({
  component: SettingsPage
})

function SettingsPage() {
  return (
    <AppPage contentClassName="max-w-5xl">
      <PageHeader title="设置" description="管理 115、JavDB 和本地应用选项" />

      <div className="grid gap-4 sm:grid-cols-2">
        <SettingsPreview
          title="115 网盘"
          description="登录账号、媒体目录和扫描选项"
          status="即将接入"
        />
        <SettingsPreview title="JavDB" description="当前线路、代理和请求限速" status="即将接入" />
        <SettingsPreview
          title="任务与缓存"
          description="查看扫描、刮削和下载任务进度"
          status="即将接入"
        />
        <SettingsPreview
          title="数据目录"
          description="管理 SQLite 索引和图片缓存位置"
          status="即将接入"
        />
      </div>
    </AppPage>
  )
}

type SettingsPreviewProps = {
  title: string
  description: string
  status: string
}

function SettingsPreview({ title, description, status }: SettingsPreviewProps) {
  return (
    <section className="rounded-3xl border border-border/70 bg-card/40 p-5 shadow-sm shadow-black/5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="font-semibold">{title}</h2>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">{description}</p>
        </div>
        <span className="shrink-0 rounded-full bg-muted px-2.5 py-1 text-xs text-muted-foreground">
          {status}
        </span>
      </div>
    </section>
  )
}
