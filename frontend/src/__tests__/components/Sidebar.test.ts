import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import Sidebar from '@/components/Sidebar.vue'
import { useChatStore } from '@/stores/chat'
import { useThemeStore } from '@/stores/theme'

// Mock vue-router
const mockPush = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: mockPush,
  }),
}))

describe('Sidebar', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('should render title', () => {
    const wrapper = mount(Sidebar)
    expect(wrapper.text()).toContain('ChatApp')
  })

  it('should show empty message when no conversations', () => {
    const wrapper = mount(Sidebar)
    expect(wrapper.text()).toContain('Zatím žádné konverzace')
  })

  it('should show conversations list', () => {
    const chatStore = useChatStore()
    chatStore.conversations = [
      {
        id: '1',
        title: 'Test Chat',
        provider: 'claude',
        model: 'claude-sonnet-4-20250514',
        system_prompt: '',
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    ]

    const wrapper = mount(Sidebar)
    expect(wrapper.text()).toContain('Test Chat')
    expect(wrapper.text()).toContain('claude-sonnet-4-20250514')
  })

  it('should render component structure', () => {
    const wrapper = mount(Sidebar)
    // Check that the component renders aside element
    expect(wrapper.find('aside').exists()).toBe(true)
  })

  it('should toggle theme', () => {
    const themeStore = useThemeStore()
    expect(themeStore.isDark).toBe(false)

    themeStore.toggle()
    expect(themeStore.isDark).toBe(true)
  })
})
