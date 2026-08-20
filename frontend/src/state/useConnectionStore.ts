import { create } from 'zustand'

interface ConnectionState {
  connected: boolean
  setConnected: (v: boolean) => void
}

export const useConnectionStore = create<ConnectionState>((set) => ({
  connected: false,
  setConnected: (v) => set({ connected: v }),
}))