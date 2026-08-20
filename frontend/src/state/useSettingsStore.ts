import { create } from 'zustand'

type Motion = 'full' | 'reduced'

interface SettingsState {
  motion: Motion
  accent: string
  livePoll: boolean
  setMotion: (m: Motion) => void
  setAccent: (hex: string) => void
  setLivePoll: (on: boolean) => void
  init: () => void
}

const ACCENTS = ['#ffb4a8', '#f0a093', '#e6b45f', '#b6bdea']

function hexToRgb(hex: string): [number, number, number] {
  const h = hex.replace('#', '')
  const n = h.length === 3 ? h.split('').map((c) => c + c).join('') : h
  const int = parseInt(n, 16)
  return [(int >> 16) & 255, (int >> 8) & 255, int & 255]
}

function rgba(hex: string, a: number): string {
  const [r, g, b] = hexToRgb(hex)
  return `rgba(${r}, ${g}, ${b}, ${a})`
}

function applyAccent(hex: string) {
  const root = document.documentElement.style
  root.setProperty('--accent', hex)
  root.setProperty('--accent-dim', hex)
  root.setProperty('--cyan', hex)
  root.setProperty('--accent-soft', rgba(hex, 0.1))
  root.setProperty('--accent-glow', rgba(hex, 0.35))
}

function applyMotion(m: Motion) {
  document.documentElement.dataset.motion = m
}

export const useSettingsStore = create<SettingsState>((set, get) => ({
  motion: 'full',
  accent: ACCENTS[0],
  livePoll: true,
  setMotion: (m) => {
    applyMotion(m)
    try {
      localStorage.setItem('3p14-motion', m)
    } catch {
      /* ignore */
    }
    set({ motion: m })
  },
  setAccent: (hex) => {
    applyAccent(hex)
    try {
      localStorage.setItem('3p14-accent', hex)
    } catch {
      /* ignore */
    }
    set({ accent: hex })
  },
  setLivePoll: (on) => {
    try {
      localStorage.setItem('3p14-livepoll', on ? '1' : '0')
    } catch {
      /* ignore */
    }
    set({ livePoll: on })
  },
  init: () => {
    try {
      const m = localStorage.getItem('3p14-motion')
      const a = localStorage.getItem('3p14-accent')
      const lp = localStorage.getItem('3p14-livepoll')
      if (m === 'full' || m === 'reduced') set({ motion: m })
      if (a && ACCENTS.includes(a)) set({ accent: a })
      set({
        livePoll: lp === null ? true : lp === '1',
      })
      applyMotion(get().motion)
      applyAccent(get().accent)
    } catch {
      /* ignore */
    }
  },
}))