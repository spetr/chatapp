import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useThemeStore } from '@/stores/theme'

describe('Theme Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    document.documentElement.classList.remove('dark')
    vi.clearAllMocks()
  })

  it('should initialize with light theme by default', () => {
    const store = useThemeStore()
    expect(store.isDark).toBe(false)
  })

  it('should toggle theme', () => {
    const store = useThemeStore()

    expect(store.isDark).toBe(false)

    store.toggle()
    expect(store.isDark).toBe(true)
    expect(localStorage.setItem).toHaveBeenCalledWith('theme', 'dark')

    store.toggle()
    expect(store.isDark).toBe(false)
    expect(localStorage.setItem).toHaveBeenCalledWith('theme', 'light')
  })

  it('should apply dark class when dark mode is enabled', () => {
    const store = useThemeStore()

    store.toggle()
    expect(document.documentElement.classList.contains('dark')).toBe(true)

    store.toggle()
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('should restore theme from localStorage', () => {
    vi.mocked(localStorage.getItem).mockReturnValue('dark')

    const store = useThemeStore()
    store.init()

    expect(localStorage.getItem).toHaveBeenCalledWith('theme')
    expect(store.isDark).toBe(true)
  })

  it('should use system preference if no stored theme', () => {
    vi.mocked(localStorage.getItem).mockReturnValue(null)
    vi.mocked(window.matchMedia).mockReturnValue({
      matches: true,
      media: '(prefers-color-scheme: dark)',
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })

    const store = useThemeStore()
    store.init()

    expect(store.isDark).toBe(true)
  })
})
