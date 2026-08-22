import { create } from 'zustand'

export const MAX_SELECTED = 2

interface TelemetryState {
  selected: string[]
  add: (base: string) => void
  toggle: (base: string) => void
  set: (bases: string[]) => void
}

export const useTelemetryStore = create<TelemetryState>((set) => ({
  selected: [],
  add: (base) =>
    set((s) => {
      if (s.selected.includes(base) || s.selected.length >= MAX_SELECTED) return s
      return { selected: [...s.selected, base] }
    }),
  toggle: (base) =>
    set((s) => {
      if (s.selected.includes(base)) return { selected: s.selected.filter((b) => b !== base) }
      if (s.selected.length >= MAX_SELECTED) return s
      return { selected: [...s.selected, base] }
    }),
  set: (bases) => set({ selected: bases.slice(0, MAX_SELECTED) }),
}))
