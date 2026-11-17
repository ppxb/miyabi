import { useState } from 'react'
import { BoxesIcon, CloudIcon, HardDriveIcon, PencilIcon, PlusIcon } from 'lucide-react'

import { useSaveSources, useSources, type SourceKind, type StorageSource } from '@/api/sources'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

const KIND_OPTIONS: {
  value: SourceKind
  label: string
  pathKey: string
  pathLabel: string
  placeholder: string
  icon: typeof HardDriveIcon
}[] = [
  {
    value: 'local',
    label: '本地目录',
    pathKey: 'path',
    pathLabel: '目录路径',
    placeholder: '/mnt/media',
    icon: HardDriveIcon
  },
  {
    value: '115',
    label: '115 网盘',
    pathKey: 'directory',
    pathLabel: '媒体目录',
    placeholder: '/媒体/影片',
    icon: CloudIcon
  },
  {
    value: 'pikpak',
    label: 'PikPak',
    pathKey: 'path',
    pathLabel: '目录路径',
    placeholder: '/media',
    icon: BoxesIcon
  }
]

function kindOf(kind: SourceKind) {
  return KIND_OPTIONS.find(option => option.value === kind) ?? KIND_OPTIONS[0]!
}

function sourcePath(source: StorageSource) {
  return source.config?.path ?? source.config?.directory ?? ''
}

function newSourceID() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `source-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

export function SourcesSection() {
  const sources = useSources()
  const save = useSaveSources()
  const [draft, setDraft] = useState<StorageSource | null>(null)
  const [draftKey, setDraftKey] = useState(0)
  const [open, setOpen] = useState(false)

  const persist = (next: StorageSource[], onSuccess?: () => void) =>
    save.mutate(next, { onSuccess })

  function openDraft(source: StorageSource) {
    setDraft(source)
    setDraftKey(key => key + 1)
    setOpen(true)
  }

  function submit(source: StorageSource) {
    const list = sources.data ?? []
    if (source.id) {
      persist(
        list.map(item => (item.id === source.id ? source : item)),
        () => setOpen(false)
      )
    } else {
      persist([...list, { ...source, id: newSourceID() }], () => setOpen(false))
    }
  }

  function remove(id: string) {
    persist(
      (sources.data ?? []).filter(item => item.id !== id),
      () => setOpen(false)
    )
  }

  function toggle(source: StorageSource) {
    persist(
      (sources.data ?? []).map(item =>
        item.id === source.id ? { ...item, enabled: !item.enabled } : item
      )
    )
  }

  return (
    <section className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h2 className="text-sm font-semibold">存储源</h2>
          <p className="text-xs text-muted-foreground">
            配置本地目录或云盘，作为媒体库的数据来源。
          </p>
        </div>
        <Button
          onClick={() => openDraft({ id: '', kind: 'local', name: '', enabled: true, config: {} })}
        >
          <PlusIcon data-icon="inline-start" />
          添加源
        </Button>
      </div>

      {sources.isError ? (
        <ErrorState
          message="无法读取存储源"
          onRetry={() => void sources.refetch()}
          retrying={sources.isFetching}
        />
      ) : null}

      {save.isError ? <p className="text-xs text-destructive">保存失败，请重试。</p> : null}

      <div className="grid gap-3 sm:grid-cols-2">
        {(sources.data ?? []).map(source => (
          <SourceCard
            key={source.id}
            source={source}
            busy={save.isPending}
            onEdit={() => openDraft(source)}
            onToggle={() => toggle(source)}
          />
        ))}
        {(sources.data?.length ?? 0) === 0 && !sources.isPending && !sources.isError ? (
          <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground sm:col-span-2">
            还没有存储源，点击「添加源」开始。
          </div>
        ) : null}
      </div>

      {draft ? (
        <SourceDialog
          key={draftKey}
          source={draft}
          open={open}
          saving={save.isPending}
          onOpenChange={setOpen}
          onSubmit={submit}
          onDelete={() => remove(draft.id)}
        />
      ) : null}
    </section>
  )
}

function SourceCard({
  source,
  busy,
  onEdit,
  onToggle
}: {
  source: StorageSource
  busy: boolean
  onEdit: () => void
  onToggle: () => void
}) {
  const option = kindOf(source.kind)
  const Icon = option.icon

  return (
    <Card>
      <CardContent className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <Icon className="size-4" />
          </span>
          <div className="min-w-0 space-y-1">
            <div className="flex items-center gap-2">
              <span className="truncate text-sm font-medium">{source.name}</span>
              <span className="shrink-0 rounded-md bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
                {option.label}
              </span>
            </div>
            <div className="truncate text-xs text-muted-foreground">
              {sourcePath(source) || '未配置路径'}
            </div>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Switch checked={source.enabled} disabled={busy} onCheckedChange={() => onToggle()} />
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="编辑存储源"
            disabled={busy}
            onClick={onEdit}
          >
            <PencilIcon />
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

function SourceDialog({
  source,
  open,
  saving,
  onOpenChange,
  onSubmit,
  onDelete
}: {
  source: StorageSource
  open: boolean
  saving: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (source: StorageSource) => void
  onDelete: () => void
}) {
  const [name, setName] = useState(source.name)
  const [kind, setKind] = useState<SourceKind>(source.kind)
  const [path, setPath] = useState(sourcePath(source))
  const [enabled, setEnabled] = useState(source.enabled)
  const option = kindOf(kind)
  const canSave = name.trim() !== '' && path.trim() !== ''

  function submit() {
    if (!canSave) return
    onSubmit({
      id: source.id,
      kind,
      name: name.trim(),
      enabled,
      config: { ...source.config, [option.pathKey]: path.trim() }
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{source.id ? '编辑存储源' : '添加存储源'}</DialogTitle>
          <DialogDescription>配置一个本地或云端媒体目录。</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-sm font-medium" htmlFor="source-name">
              名称
            </label>
            <Input
              id="source-name"
              value={name}
              placeholder="我的媒体库"
              onChange={event => setName(event.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-medium">类型</label>
            <Select value={kind} onValueChange={value => setKind(value as SourceKind)}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {KIND_OPTIONS.map(option => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-medium" htmlFor="source-path">
              {option.pathLabel}
            </label>
            <Input
              id="source-path"
              value={path}
              placeholder={option.placeholder}
              onChange={event => setPath(event.target.value)}
            />
          </div>

          <div className="flex items-center justify-between rounded-lg border p-3">
            <div className="space-y-0.5">
              <div className="text-sm font-medium">启用</div>
              <div className="text-xs text-muted-foreground">停用后不参与媒体库聚合</div>
            </div>
            <Switch checked={enabled} onCheckedChange={checked => setEnabled(checked)} />
          </div>
        </div>

        <DialogFooter>
          {source.id ? (
            <Button variant="destructive" className="mr-auto" disabled={saving} onClick={onDelete}>
              删除
            </Button>
          ) : null}
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button disabled={!canSave || saving} onClick={submit}>
            保存
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
