import { useQueries, type UseQueryResult } from '@tanstack/react-query'
import { usePlayer } from '@videojs/react/video'
import { useEffect, useRef } from 'react'

import {
  subtitleCuesQuery,
  subtitleLanguageLabel,
  subtitleVersionLabels,
  type SubtitlePlayerCue,
  type SubtitleTrack
} from '@/api/subtitles'

/** Saved choice value for "subtitles off". */
export const SUBTITLES_OFF = 'off'

const offsets = [-5, -2, -1, -0.5, 0, 0.5, 1, 2, 5]

export const subtitleOffsetOptions = offsets.map(offset => ({
  value: String(offset),
  label: offset === 0 ? '不偏移' : offset > 0 ? `延后 ${offset} 秒` : `提前 ${-offset} 秒`
}))

// Menu labels must tell tracks apart; the format is added only where needed.
function trackLabels(tracks: SubtitleTrack[]) {
  const base = tracks.map(track =>
    [subtitleLanguageLabel(track.language), subtitleVersionLabels[track.version]]
      .filter(Boolean)
      .join(' · ')
  )
  return tracks.map((track, index) => {
    const label = base[index]
    const shared = base.filter(other => other === label).length > 1
    return shared ? `${label} · ${track.format.toUpperCase()}` : label
  })
}

function shift(cues: SubtitlePlayerCue[], offset: number) {
  if (offset === 0) return cues
  return cues.map(cue => ({
    ...cue,
    startTime: Math.max(0, cue.startTime + offset),
    endTime: Math.max(0, cue.endTime + offset)
  }))
}

const combineCues = (results: UseQueryResult<SubtitlePlayerCue[]>[]) => ({
  settled: results.every(result => !result.isPending),
  data: results.map(result => result.data)
})

// The add effect must only re-run when the tracks or their cue content actually
// change. `cues.data` and `tracks` get fresh array identities on every render,
// so depending on them directly would tear down and re-add the subtitle tracks
// on each parent re-render (e.g. when the caption choice state updates), which
// drops the current selection and breaks turning subtitles back on. A compact
// signature keeps the effect stable across renders while still reacting to real
// changes.
function cueSignature(tracks: SubtitleTrack[], data: (SubtitlePlayerCue[] | undefined)[]) {
  return tracks
    .map((track, index) => {
      const cues = data[index]
      const first = cues?.[0]
      const last = cues?.[cues.length - 1]
      return `${track.id}:${cues ? cues.length : -1}:${first?.startTime ?? ''}:${last?.endTime ?? ''}`
    })
    .join('|')
}

/**
 * Adds a movie's subtitle tracks to the player so the built-in captions button
 * and menu list them. Without a saved choice the first track is shown. Changing
 * the offset re-adds the tracks with shifted cues and keeps the selection.
 */
export function PlayerSubtitles({
  movieID,
  tracks,
  offset,
  savedChoice,
  onChoiceChange
}: {
  movieID: number
  tracks: SubtitleTrack[]
  offset: number
  savedChoice?: string
  onChoiceChange: (choice: string) => void
}) {
  const player = usePlayer()
  const cues = useQueries({
    queries: tracks.map(track => subtitleCuesQuery(movieID, track.id)),
    combine: combineCues
  })
  // The choice the next re-add restores; follows the user's picks in the menu.
  const choiceRef = useRef(savedChoice)
  // Player track id → subtitle track id, for the tracks currently added.
  const addedRef = useRef(new Map<string, number>())
  // The track just selected; the store marks it showing only after the media's
  // asynchronous change event, so "nothing showing" until then is not "off".
  const pendingRef = useRef<string | undefined>(undefined)
  // Selecting a track needs the store attached to the media, which happens a
  // commit after mount; a known duration means the media is attached.
  const attached = usePlayer(state => Number.isFinite(state.duration) && state.duration > 0)

  const cueKey = cueSignature(tracks, cues.data)

  useEffect(() => {
    if (!cues.settled || !attached) return
    const labels = trackLabels(tracks)
    const added: { handle: { id: string; remove(): void }; trackID: number }[] = []
    tracks.forEach((track, index) => {
      const data = cues.data[index]
      if (!data) return
      const handle = player.addTextTrack({
        kind: 'subtitles',
        label: labels[index],
        language: track.language,
        cues: shift(data, offset)
      })
      added.push({ handle, trackID: track.id })
    })
    addedRef.current = new Map(added.map(({ handle, trackID }) => [handle.id, trackID]))
    const choice = choiceRef.current
    const target =
      choice === SUBTITLES_OFF
        ? undefined
        : (added.find(({ trackID }) => String(trackID) === choice) ?? added[0])
    player.selectSubtitlesTrack(target?.handle.id ?? null)
    pendingRef.current = target?.handle.id
    return () => {
      addedRef.current = new Map()
      pendingRef.current = undefined
      for (const { handle } of added) {
        try {
          handle.remove()
        } catch {
          // The player may already be torn down when the dialog closes.
        }
      }
    }
    // cueKey/offset/attached stand in for tracks & cues so the effect only
    // re-runs on real changes, not on every render's new array identities.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [player, cueKey, offset, attached, cues.settled])

  const showing = usePlayer(
    state =>
      state.textTrackList.find(
        track =>
          (track.kind === 'subtitles' || track.kind === 'captions') && track.mode === 'showing'
      )?.id ?? null
  )

  useEffect(() => {
    if (addedRef.current.size === 0) return
    if (pendingRef.current) {
      if (showing === null) return
      pendingRef.current = undefined
    }
    const trackID = showing ? addedRef.current.get(showing) : undefined
    // Ignore tracks this component did not add, such as ones the media carries.
    if (showing && trackID === undefined) return
    const choice = trackID === undefined ? SUBTITLES_OFF : String(trackID)
    if (choice === choiceRef.current) return
    choiceRef.current = choice
    onChoiceChange(choice)
  }, [showing, onChoiceChange])

  return null
}
