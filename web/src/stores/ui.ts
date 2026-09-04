import { create } from "zustand"

type UIState = {
  taskPanelOpen: boolean
  setTaskPanelOpen: (open: boolean) => void
}

export const useUIStore = create<UIState>((set) => ({
  taskPanelOpen: false,
  setTaskPanelOpen: (taskPanelOpen) => set({ taskPanelOpen }),
}))
