'use client'

import '../styles/video/base.css'
import type { ComponentProps, ReactNode } from 'react'

import { BufferingIndicator } from '@/components/videojs/ui/buffering-indicator'
import { Container } from '@/components/videojs/ui/container'
import { ErrorDialog } from '@/components/videojs/ui/error-dialog'
import { Poster } from '@/components/videojs/ui/poster'
import { Title } from '@/components/videojs/ui/title'
import { cn } from '@/lib/resolve-class-name'

import { VideoGestures } from './behaviors/gestures'
import { VideoHotkeys } from './behaviors/hotkeys'
import { VideoStatusIndicators } from './display/status-indicators'
import { DefaultVideoControls } from './layout/controls'

export interface VideoSkinProps extends Omit<
  NonNullable<ComponentProps<typeof Container>>,
  'children'
> {
  children?: ReactNode
  renderPoster?: NonNullable<ComponentProps<typeof Poster>>['renderImage']
  renderThumbnail?: NonNullable<ComponentProps<typeof DefaultVideoControls>>['renderThumbnail']
  qualityOptions: NonNullable<ComponentProps<typeof DefaultVideoControls>>['qualityOptions']
  qualityValue: string
  onQualityChange: (value: string) => void
  onSettingsOpen?: () => void
  subtitleOffsetMenu?: NonNullable<
    ComponentProps<typeof DefaultVideoControls>
  >['subtitleOffsetMenu']
}

export function VideoSkin({
  children,
  className,
  renderPoster,
  renderThumbnail,
  qualityOptions,
  qualityValue,
  onQualityChange,
  onSettingsOpen,
  subtitleOffsetMenu,
  ...props
}: VideoSkinProps) {
  return (
    <Container
      className={cn('pointer-fine:not-data-controls-visible:cursor-none', className)}
      data-theme="default"
      data-preset="video"
      {...props}
    >
      {children}
      <Poster renderImage={renderPoster} />
      <BufferingIndicator />
      <ErrorDialog />
      <Title />

      <DefaultVideoControls
        renderThumbnail={renderThumbnail}
        qualityOptions={qualityOptions}
        qualityValue={qualityValue}
        onQualityChange={onQualityChange}
        onSettingsOpen={onSettingsOpen}
        subtitleOffsetMenu={subtitleOffsetMenu}
      />

      <VideoHotkeys />
      <VideoGestures />
      <VideoStatusIndicators />
    </Container>
  )
}
