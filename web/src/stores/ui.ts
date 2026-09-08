import { create } from 'zustand'

type UIState = {
  playbackCode: string | null
  openPlayer: (code: string) => void
  closePlayer: () => void
}

export const useUIStore = create<UIState>(set => ({
  playbackCode: null,
  openPlayer: playbackCode => set({ playbackCode }),
  closePlayer: () => set({ playbackCode: null })
}))
