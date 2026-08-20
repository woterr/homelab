import { create } from 'zustand'
import { api, type Rule } from '../lib/api'

interface RulesState {
  rules: Rule[]
  loading: boolean
  error: string | null
  refresh: () => Promise<void>
  create: (r: Omit<Rule, 'id' | 'created_at' | 'enabled'>) => Promise<void>
  remove: (id: string) => Promise<void>
}

export const useRulesStore = create<RulesState>((set, get) => ({
  rules: [],
  loading: false,
  error: null,
  refresh: async () => {
    set({ loading: true })
    try {
      const rules = await api.rules()
      set({ rules, loading: false, error: null })
    } catch (e) {
      set({ loading: false, error: e instanceof Error ? e.message : 'failed' })
    }
  },
  create: async (r) => {
    await api.createRule(r)
    await get().refresh()
  },
  remove: async (id) => {
    await api.deleteRule(id)
    set((s) => ({ rules: s.rules.filter((x) => x.id !== id) }))
  },
}))