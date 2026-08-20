import { create } from 'zustand'
import { api, type Service } from '../lib/api'

interface ServicesState {
  services: Service[]
  loading: boolean
  error: string | null
  refresh: () => Promise<void>
}

export const useServicesStore = create<ServicesState>((set) => ({
  services: [],
  loading: false,
  error: null,
  refresh: async () => {
    set({ loading: true })
    try {
      const services = await api.services()
      set({ services, loading: false, error: null })
    } catch (e) {
      set({ loading: false, error: e instanceof Error ? e.message : 'failed' })
    }
  },
}))