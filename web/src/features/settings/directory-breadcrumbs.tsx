import { Fragment } from 'react'

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator
} from '@/components/ui/breadcrumb'

type DirectoryBreadcrumbsProps = {
  path?: { id: string; name: string }[]
  currentID: string
  disabled?: boolean
  onNavigate: (id: string) => void
}

export function DirectoryBreadcrumbs({
  path,
  currentID,
  disabled = false,
  onNavigate
}: DirectoryBreadcrumbsProps) {
  return (
    <Breadcrumb className="min-w-0">
      <BreadcrumbList className="gap-1">
        <BreadcrumbItem>
          {currentID === '0' ? (
            <BreadcrumbPage>全部文件</BreadcrumbPage>
          ) : (
            <BreadcrumbLink
              render={<button type="button" disabled={disabled} onClick={() => onNavigate('0')} />}
            >
              全部文件
            </BreadcrumbLink>
          )}
        </BreadcrumbItem>
        {path
          ?.filter(directory => directory.id !== '0')
          .map(directory => (
            <Fragment key={directory.id}>
              <BreadcrumbSeparator className="shrink-0" />
              <BreadcrumbItem className="max-w-full min-w-0">
                {currentID === directory.id ? (
                  <BreadcrumbPage className="max-w-40 min-w-0 truncate">
                    {directory.name}
                  </BreadcrumbPage>
                ) : (
                  <BreadcrumbLink
                    render={
                      <button
                        type="button"
                        className="max-w-40 min-w-0 truncate"
                        disabled={disabled}
                        onClick={() => onNavigate(directory.id)}
                      />
                    }
                  >
                    {directory.name}
                  </BreadcrumbLink>
                )}
              </BreadcrumbItem>
            </Fragment>
          ))}
      </BreadcrumbList>
    </Breadcrumb>
  )
}
