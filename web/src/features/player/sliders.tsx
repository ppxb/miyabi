import { useEffect, useState } from 'react'
import {
  isPointerEvent,
  TimeSlider,
  useMediaPlayer,
  VolumeSlider,
  type TimeSliderInstance
} from '@vidstack/react'
import { useDefaultLayoutContext } from '@vidstack/react/player/layouts/default'

import { PlayerSliderTooltip } from './tooltip'

export function PlayerTimeSlider() {
  const {
    disableTimeSlider,
    noScrubGesture,
    seekStep,
    sliderChaptersMinWidth = 325
  } = useDefaultLayoutContext()
  const player = useMediaPlayer()
  const [slider, setSlider] = useState<TimeSliderInstance | null>(null)
  const [width, setWidth] = useState(0)

  useEffect(() => {
    const element = slider?.el
    if (!element) return
    const observer = new ResizeObserver(() => setWidth(element.clientWidth))
    observer.observe(element)
    return () => observer.disconnect()
  }, [slider])

  return (
    <TimeSlider.Root
      ref={setSlider}
      className="vds-time-slider vds-slider"
      disabled={disableTimeSlider}
      noSwipeGesture={noScrubGesture}
      keyStep={seekStep}
      onDragEnd={(_, event) => {
        // Vidstack also emits this after clicks and drags released outside the slider.
        if (isPointerEvent(event.trigger)) player?.el?.focus({ preventScroll: true })
      }}
    >
      <TimeSlider.Chapters
        className="vds-slider-chapters"
        disabled={width < sliderChaptersMinWidth}
      >
        {(cues, forwardRef) =>
          cues.map(cue => (
            <div key={cue.startTime} ref={forwardRef} className="vds-slider-chapter">
              <TimeSlider.Track className="vds-slider-track" />
              <TimeSlider.TrackFill className="vds-slider-track-fill vds-slider-track" />
              <TimeSlider.Progress className="vds-slider-progress vds-slider-track" />
            </div>
          ))
        }
      </TimeSlider.Chapters>
      <TimeSlider.Thumb className="vds-slider-thumb" />
      <PlayerSliderTooltip>
        <TimeSlider.ChapterTitle className="max-w-48 truncate empty:hidden" />
        <TimeSlider.Value className="tabular-nums" />
      </PlayerSliderTooltip>
    </TimeSlider.Root>
  )
}

export function PlayerVolumeSlider() {
  const { isSmallLayout } = useDefaultLayoutContext()
  const orientation = isSmallLayout ? 'vertical' : 'horizontal'

  return (
    <VolumeSlider.Root className="vds-volume-slider vds-slider" orientation={orientation}>
      <VolumeSlider.Track className="vds-slider-track" />
      <VolumeSlider.TrackFill className="vds-slider-track-fill vds-slider-track" />
      <VolumeSlider.Thumb className="vds-slider-thumb" />
      <PlayerSliderTooltip orientation={orientation}>
        <VolumeSlider.Value className="tabular-nums" />
      </PlayerSliderTooltip>
    </VolumeSlider.Root>
  )
}
