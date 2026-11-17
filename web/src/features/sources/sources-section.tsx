import { useState } from 'react'
import { HardDriveIcon, PencilIcon, PlusIcon } from 'lucide-react'

import { useSaveSources, useSources, type SourceKind, type StorageSource } from '@/api/sources'
import { InlineError } from '@/components/error-state'
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
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { SettingRow, SettingsSection } from '@/features/settings/shared'

const KIND_OPTIONS: {
  value: SourceKind
  label: string
  pathKey: string
  pathLabel: string
  placeholder: string
}[] = [
  {
    value: 'local',
    label: '本地目录',
    pathKey: 'path',
    pathLabel: '目录路径',
    placeholder: '/mnt/media'
  },
  {
    value: '115',
    label: '115 网盘',
    pathKey: 'directory',
    pathLabel: '媒体目录',
    placeholder: '/媒体/影片'
  },
  {
    value: 'pikpak',
    label: 'PikPak',
    pathKey: 'path',
    pathLabel: '目录路径',
    placeholder: '/media'
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
    <SettingsSection icon={<HardDriveIcon className="size-4" />} title="存储源">
      <SettingRow title="存储源" description="配置本地目录或云盘，作为媒体库的数据来源">
        <Button
          onClick={() => openDraft({ id: '', kind: 'local', name: '', enabled: true, config: {} })}
        >
          <PlusIcon data-icon="inline-start" />
          添加源
        </Button>
      </SettingRow>

      {sources.isError ? (
        <InlineError
          onRetry={() => void sources.refetch()}
          retrying={sources.isFetching}
          retryLabel="重试"
        >
          无法读取存储源
        </InlineError>
      ) : null}
      {save.isError ? <InlineError>保存失败，请重试</InlineError> : null}

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
          onDelete={() => remove(draft.id)}
        />
      ) : null}
    </SettingsSection>
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
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>{source.name}</CardTitle>
        <CardDescription>
          {kindOf(source.kind).label} · {sourcePath(source) || '未配置路径'}
        </CardDescription>
        <CardAction>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="编辑存储源"
            disabled={busy}
            onClick={onEdit}
          >
            <PencilIcon />
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        <SettingRow title="启用" description="停用后不参与媒体库聚合" inline>
          <Switch checked={source.enabled} disabled={busy} onCheckedChange={() => onToggle()} />
        </SettingRow>
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
  const [nameError, setNameError] = useState<string | null>(null)
  const [pathError, setPathError] = useState<string | null>(null)
  const option = kindOf(kind)

  function submit() {
    let valid = true
    if (name.trim() === '') {
      setNameError('请输入名称')
      valid = false
    }
    if (path.trim() === '') {
      setPathError('请输入目录')
      valid = false
    }
    if (!valid) return
    onSubmit({
      id: source.id,
      kind,
      name: name.trim(),
      enabled,
      config: { ...source.config, [option.pathKey]: path.trim() }
    })
  }

  return (
    <Dialog
      open={open}
      onOpenChange={nextOpen => {
        if (!nextOpen && !saving) onOpenChange(false)
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{source.id ? '编辑存储源' : '添加存储源'}</DialogTitle>
          <DialogDescription>配置一个本地或云端媒体目录</DialogDescription>
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
                <SelectValue>{option.label}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                {KIND_OPTIONS.map(option => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          <Field data-invalid={Boolean(pathError)}>
            <FieldLabel htmlFor="source-path">{option.pathLabel}</FieldLabel>
            <Input
              id="source-path"
              value={path}
              disabled={saving}
              aria-invalid={Boolean(pathError)}
              placeholder={option.placeholder}
              onChange={event => {
                setPath(event.target.value)
                setPathError(null)
              }}
            />
            {pathError ? <FieldError>{pathError}</FieldError> : null}
          </Field>

          <Field orientation="horizontal">
            <FieldContent>
              <FieldLabel htmlFor="source-enabled">启用</FieldLabel>
              <FieldDescription>停用后不参与媒体库聚合</FieldDescription>
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
                <>
                  <Spinner />
                  保存中
                </>
              ) : (
                '保存'
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
