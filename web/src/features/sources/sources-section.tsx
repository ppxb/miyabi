import { useState } from 'react'
import { toast } from 'sonner'
import {
  FolderOpenIcon,
  HardDriveIcon,
  LogOutIcon,
  PencilIcon,
  PlusIcon,
  QrCodeIcon,
  SaveIcon,
  UserRoundIcon
} from 'lucide-react'

import { describeApiError } from '@/api/client'
import { useDisconnectPanAccount, usePanAccountStatus, type PanAccount } from '@/api/pan'
import { useSaveSources, useSources, type SourceKind, type StorageSource } from '@/api/sources'
import { InlineError } from '@/components/error-state'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldLabel
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { PanDirectoryDialog } from '@/features/settings/pan-directory-dialog'
import { PanLoginDialog } from '@/features/settings/pan-login-dialog'
import { SettingsSection } from '@/features/settings/shared'

const KIND_LABELS: Record<SourceKind, string> = {
  local: '本地目录',
  '115': '115 网盘',
  pikpak: 'PikPak'
}

const KIND_ORDER: SourceKind[] = ['local', '115', 'pikpak']

// Only local directories need a manually entered path.
const LOCAL_PATH = { label: '目录路径', placeholder: '/mnt/media' } as const

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
  const [deleteTarget, setDeleteTarget] = useState<StorageSource | null>(null)

  const persist = (next: StorageSource[], onSuccess?: () => void) =>
    save.mutate(next, {
      onSuccess,
      onError: error => toast.error(describeApiError(error))
    })

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

  function bindAccount(source: StorageSource, accountID: string) {
    persist(
      (sources.data ?? []).map(item =>
        item.id === source.id
          ? { ...item, config: { ...item.config, account_id: accountID } }
          : item
      )
    )
  }

  return (
    <SettingsSection icon={<HardDriveIcon className="size-4" />} title="存储源">
      <div className="flex justify-end">
        <Button
          onClick={() => openDraft({ id: '', kind: 'local', name: '', enabled: true, config: {} })}
        >
          <PlusIcon data-icon="inline-start" />
          添加源
        </Button>
      </div>

      {sources.isPending ? <Skeleton className="h-4 w-40" /> : null}
      {sources.isError ? (
        <InlineError
          onRetry={() => void sources.refetch()}
          retrying={sources.isFetching}
          retryLabel="重试"
        >
          无法读取存储源
        </InlineError>
      ) : null}

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {(sources.data ?? []).map(source => (
          <SourceCard
            key={source.id}
            source={source}
            onEdit={() => openDraft(source)}
            onBind={accountID => bindAccount(source, accountID)}
          />
        ))}
        {(sources.data?.length ?? 0) === 0 && !sources.isPending && !sources.isError ? (
          <Card size="sm" className="sm:col-span-2">
            <CardContent>
              <CardDescription>还没有存储源，点击「添加源」开始。</CardDescription>
            </CardContent>
          </Card>
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
          onDelete={() => setDeleteTarget(draft)}
        />
      ) : null}

      {deleteTarget ? (
        <Dialog
          open
          onOpenChange={next => {
            if (!next) setDeleteTarget(null)
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>删除存储源？</DialogTitle>
              <DialogDescription>
                将移除存储源「{deleteTarget.name || '未命名'}
                」，其挂载绑定也会一并清除，此操作不可撤销。
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDeleteTarget(null)}>
                取消
              </Button>
              <Button
                type="button"
                variant="destructive"
                onClick={() => {
                  const id = deleteTarget.id
                  setDeleteTarget(null)
                  remove(id)
                }}
              >
                确认删除
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      ) : null}
    </SettingsSection>
  )
}

function SourceCard({
  source,
  onEdit,
  onBind
}: {
  source: StorageSource
  onEdit: () => void
  onBind: (accountID: string) => void
}) {
  return source.kind === '115' ? (
    <PanSourceCard source={source} onEdit={onEdit} onBind={onBind} />
  ) : (
    <PlainSourceCard source={source} onEdit={onEdit} />
  )
}

function EditAction({ onEdit }: { onEdit: () => void }) {
  return (
    <Button variant="ghost" size="icon" aria-label="编辑存储源" onClick={onEdit}>
      <PencilIcon />
    </Button>
  )
}

function PlainSourceCard({ source, onEdit }: { source: StorageSource; onEdit: () => void }) {
  const label = KIND_LABELS[source.kind]
  const path = sourcePath(source)

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <span className="truncate">{source.name}</span>
          <Badge variant="secondary">{label}</Badge>
        </CardTitle>
        {path ? <CardDescription className="truncate">{path}</CardDescription> : null}
        <CardAction>
          <EditAction onEdit={onEdit} />
        </CardAction>
      </CardHeader>
    </Card>
  )
}

function PanSourceCard({
  source,
  onEdit,
  onBind
}: {
  source: StorageSource
  onEdit: () => void
  onBind: (accountID: string) => void
}) {
  const accountID = source.config?.account_id ?? ''
  const status = usePanAccountStatus(accountID)
  const disconnect = useDisconnectPanAccount(accountID)
  const profile = status.data?.account

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <span className="truncate">{source.name}</span>
          <Badge variant="secondary">115 网盘</Badge>
        </CardTitle>
        <CardAction>
          <EditAction onEdit={onEdit} />
        </CardAction>
      </CardHeader>
      <CardContent>
        {!accountID ? (
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs text-muted-foreground">未绑定账号</span>
            <PanLoginDialog onAuthorized={onBind}>
              <Button size="icon" aria-label="登录 115">
                <QrCodeIcon />
              </Button>
            </PanLoginDialog>
          </div>
        ) : status.isPending ? (
          <Spinner className="text-muted-foreground" />
        ) : status.isError || !profile ? (
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs text-muted-foreground">账号不可用</span>
            <Button variant="outline" onClick={() => onBind('')}>
              重新绑定
            </Button>
          </div>
        ) : (
          <div className="space-y-3">
            <div className="flex items-center justify-between gap-2">
              <div className="flex min-w-0 items-center gap-2">
                <Avatar key={profile.avatar} className="size-6">
                  {profile.avatar ? <AvatarImage src={profile.avatar} /> : null}
                  <AvatarFallback>
                    <UserRoundIcon className="size-3.5" />
                  </AvatarFallback>
                </Avatar>
                <span className="truncate text-sm font-medium">{profile.name}</span>
              </div>
              <Button
                variant="ghost"
                size="icon"
                aria-label="退出登录"
                disabled={disconnect.isPending}
                onClick={() => disconnect.mutate(undefined, { onSuccess: () => onBind('') })}
              >
                <LogOutIcon />
              </Button>
            </div>

            <PanSpaceBlocks space={profile.space} />

            <div className="flex items-center justify-between gap-2">
              <span className="truncate text-xs text-muted-foreground">
                {status.data?.directory?.path ?? '未绑定目录'}
              </span>
              <PanDirectoryDialog accountID={profile.id} directory={status.data?.directory}>
                <Button variant="ghost" size="icon" aria-label="修改目录">
                  <FolderOpenIcon />
                </Button>
              </PanDirectoryDialog>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function PanSpaceBlocks({ space }: { space: PanAccount['space'] }) {
  const total = space.total.bytes > 0 ? space.total.bytes : space.used.bytes + space.remaining.bytes
  const usedPercent = total > 0 ? (space.used.bytes / total) * 100 : 0

  return (
    <div className="space-y-1">
      <Progress value={usedPercent} variant="info" className="h-2" />
      <div className="flex justify-between gap-2 text-xs text-muted-foreground">
        <span>已用 {space.used.formatted}</span>
        <span>可用 {space.remaining.formatted}</span>
      </div>
    </div>
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
  const [scrapeMode, setScrapeMode] = useState(source.config?.scrape_mode ?? 'auto')
  const [enabled, setEnabled] = useState(source.enabled)
  const [nameError, setNameError] = useState<string | null>(null)
  const [pathError, setPathError] = useState<string | null>(null)
  const needsPath = kind === 'local'

  function submit() {
    let valid = true
    if (name.trim() === '') {
      setNameError('请输入名称')
      valid = false
    }
    if (needsPath && path.trim() === '') {
      setPathError('请输入目录')
      valid = false
    }
    if (!valid) return
    onSubmit({
      id: source.id,
      kind,
      name: name.trim(),
      enabled,
      config: needsPath
        ? { ...source.config, path: path.trim(), scrape_mode: scrapeMode }
        : { ...source.config }
    })
  }

  return (
    <Dialog
      open={open}
      onOpenChange={nextOpen => {
        if (!nextOpen && !saving) onOpenChange(false)
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{source.id ? '编辑存储源' : '添加存储源'}</DialogTitle>
          <DialogDescription>配置一个本地或云端媒体目录。</DialogDescription>
        </DialogHeader>

        <form
          className="flex flex-col gap-4"
          onSubmit={event => {
            event.preventDefault()
            submit()
          }}
        >
          <Field data-invalid={Boolean(nameError)}>
            <FieldLabel htmlFor="source-name">名称</FieldLabel>
            <Input
              id="source-name"
              value={name}
              autoFocus
              disabled={saving}
              aria-invalid={Boolean(nameError)}
              placeholder="我的媒体库"
              onChange={event => {
                setName(event.target.value)
                setNameError(null)
              }}
            />
            {nameError ? <FieldError>{nameError}</FieldError> : null}
          </Field>

          <Field>
            <FieldLabel>类型</FieldLabel>
            <Select value={kind} onValueChange={value => setKind(value as SourceKind)}>
              <SelectTrigger>
                <SelectValue>{KIND_LABELS[kind]}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                {KIND_ORDER.map(value => (
                  <SelectItem key={value} value={value} disabled={value === 'pikpak'}>
                    {value === 'pikpak' ? `${KIND_LABELS[value]}（即将支持）` : KIND_LABELS[value]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          {needsPath ? (
            <>
              <Field data-invalid={Boolean(pathError)}>
                <FieldLabel htmlFor="source-path">{LOCAL_PATH.label}</FieldLabel>
                <Input
                  id="source-path"
                  value={path}
                  disabled={saving}
                  aria-invalid={Boolean(pathError)}
                  placeholder={LOCAL_PATH.placeholder}
                  onChange={event => {
                    setPath(event.target.value)
                    setPathError(null)
                  }}
                />
                {pathError ? <FieldError>{pathError}</FieldError> : null}
              </Field>

              <Field>
                <FieldLabel>刮削方式</FieldLabel>
                <Select value={scrapeMode} onValueChange={value => value && setScrapeMode(value)}>
                  <SelectTrigger className="w-full">
                    <SelectValue>
                      {scrapeMode === 'local' ? '仅使用本地数据' : '自动补全'}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="auto">自动补全</SelectItem>
                    <SelectItem value="local">仅使用本地数据</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
            </>
          ) : (
            <FieldDescription>保存后请在卡片中登录 115，登录后再选择媒体目录。</FieldDescription>
          )}

          <Field orientation="horizontal">
            <FieldContent>
              <FieldLabel htmlFor="source-enabled">启用</FieldLabel>
              <FieldDescription>停用后该源不参与媒体库聚合与扫描</FieldDescription>
            </FieldContent>
            <Switch
              id="source-enabled"
              checked={enabled}
              disabled={saving}
              onCheckedChange={checked => setEnabled(checked)}
            />
          </Field>

          <DialogFooter>
            {source.id ? (
              <Button
                type="button"
                variant="destructive"
                className="mr-auto"
                disabled={saving}
                onClick={onDelete}
              >
                删除
              </Button>
            ) : null}
            <Button
              type="button"
              variant="outline"
              disabled={saving}
              onClick={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <SaveIcon data-icon="inline-start" />
              )}
              保存
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
