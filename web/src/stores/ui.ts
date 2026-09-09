import { create } from 'zustand'

type UIState = {
  playbackMovieID: number | null
  openPlayer: (movieID: number) => void
  closePlayer: () => void
}

export const useUIStore = create<UIState>(set => ({
  playbackMovieID: null,
  openPlayer: playbackMovieID => set({ playbackMovieID }),
  closePlayer: () => set({ playbackMovieID: null })
}))
