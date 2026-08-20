import { create } from 'zustand'
import { api, type FullSystem } from '../lib/api'

interface SystemState {
  system: FullSystem | null
  loading: boolean
  error: string | null
  lastUpdated: number
  refresh: () => Promise<void>
}

export const useSystemStore = create<SystemState>((set) => ({
  system: null,
  loading: false,
  error: null,
  lastUpdated: 0,
  refresh: async () => {
    set({ loading: true })
    try {
      const system = await api.system()
      set({ system, loading: false, error: null, lastUpdated: Date.now() })
    } catch (e) {
      set({ loading: false, error: e instanceof Error ? e.message : 'failed' })
    }
  },
}))