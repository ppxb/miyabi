import { create } from 'zustand'
import { persist } from 'zustand/middleware'

type SettingsState = {
  nsfwMode: boolean
  setNsfwMode: (enabled: boolean) => void
}

export const useSettingsStore = create<SettingsState>()(
  persist(
    set => ({
      nsfwMode: true,
      setNsfwMode: nsfwMode => set({ nsfwMode })
    }),
    {
      name: 'miyabi-settings',
      partialize: state => ({ nsfwMode: state.nsfwMode })
    }
  )
)
