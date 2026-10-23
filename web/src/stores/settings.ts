import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type ImageSafety = 'show' | 'blur' | 'hide'

type SettingsState = {
  imageSafety: ImageSafety
  setImageSafety: (imageSafety: ImageSafety) => void
}

export const useSettingsStore = create<SettingsState>()(
  persist(
    set => ({
      imageSafety: 'blur',
      setImageSafety: imageSafety => set({ imageSafety })
    }),
    {
      name: 'miyabi-settings',
      version: 1,
      migrate: persistedState => {
        const state = persistedState as
          | { imageSafety?: unknown; nsfwMode?: unknown }
          | null
          | undefined
        if (
          state?.imageSafety === 'show' ||
          state?.imageSafety === 'blur' ||
          state?.imageSafety === 'hide'
        ) {
          return { imageSafety: state.imageSafety }
        }
        return {
          imageSafety:
            state?.nsfwMode === true ? 'hide' : state?.nsfwMode === false ? 'show' : 'blur'
        }
      },
      partialize: state => ({ imageSafety: state.imageSafety })
    }
  )
)
