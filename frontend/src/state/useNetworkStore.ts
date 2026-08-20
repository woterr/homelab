import { create } from 'zustand'
import { api, type Topology, type Device } from '../lib/api'

interface NetworkState {
  topology: Topology | null
  devices: Device[]
  loading: boolean
  error: string | null
  refresh: () => Promise<void>
}

export const useNetworkStore = create<NetworkState>((set) => ({
  topology: null,
  devices: [],
  loading: false,
  error: null,
  refresh: async () => {
    set({ loading: true })
    try {
      const [topology, devices] = await Promise.all([api.topology(), api.devices()])
      set({ topology, devices, loading: false, error: null })
    } catch (e) {
      set({ loading: false, error: e instanceof Error ? e.message : 'failed' })
    }
  },
}))