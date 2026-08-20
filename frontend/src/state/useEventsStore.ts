import { create } from 'zustand'
import { api, type Event } from '../lib/api'

interface EventsState {
  events: Event[]
  loading: boolean
  error: string | null
  refresh: (limit?: number) => Promise<void>
  push: (e: Event) => void
}

export const useEventsStore = create<EventsState>((set, get) => ({
  events: [],
  loading: false,
  error: null,
  refresh: async (limit = 200) => {
    set({ loading: true })
    try {
      const events = await api.events(limit)
      set({ events, loading: false, error: null })
    } catch (e) {
      set({ loading: false, error: e instanceof Error ? e.message : 'failed' })
    }
  },
  push: (e) => {
    const events = [e, ...get().events.filter((x) => x.id !== e.id)].slice(0, 500)
    set({ events })
  },
}))