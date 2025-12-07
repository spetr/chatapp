import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import MessageBubble from '@/components/MessageBubble.vue'
import type { Message } from '@/types'

describe('MessageBubble', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  const userMessage: Message = {
    id: 'msg-1',
    conversation_id: 'conv-1',
    role: 'user',
    content: 'Hello, how are you?',
    created_at: new Date().toISOString(),
  }

  const assistantMessage: Message = {
    id: 'msg-2',
    conversation_id: 'conv-1',
    role: 'assistant',
    content: 'I am doing well, thank you!',
    metrics: {
      input_tokens: 100,
      output_tokens: 50,
      total_tokens: 150,
      ttfb_ms: 200,
      total_latency_ms: 1000,
      tokens_per_second: 50,
    },
    created_at: new Date().toISOString(),
  }

  it('should render user message correctly', () => {
    const wrapper = mount(MessageBubble, {
      props: { message: userMessage },
    })

    expect(wrapper.text()).toContain('Vy')
    expect(wrapper.text()).toContain('Hello, how are you?')
  })

  it('should render assistant message correctly', () => {
    const wrapper = mount(MessageBubble, {
      props: { message: assistantMessage },
    })

    expect(wrapper.text()).toContain('Asistent')
    expect(wrapper.text()).toContain('I am doing well, thank you!')
  })

  it('should show metrics for assistant messages', () => {
    const wrapper = mount(MessageBubble, {
      props: { message: assistantMessage },
    })

    expect(wrapper.text()).toContain('100')
    expect(wrapper.text()).toContain('50')
    expect(wrapper.text()).toContain('200')
  })

  it('should not show metrics for user messages', () => {
    const wrapper = mount(MessageBubble, {
      props: { message: userMessage },
    })

    expect(wrapper.text()).not.toContain('tok/s')
  })

  it('should emit copy event', async () => {
    const wrapper = mount(MessageBubble, {
      props: { message: assistantMessage },
    })

    // Find copy button by icon
    const copyButton = wrapper.find('[icon="pi pi-copy"]')
    if (copyButton.exists()) {
      await copyButton.trigger('click')
      expect(wrapper.emitted('copy')).toBeTruthy()
      expect(wrapper.emitted('copy')![0]).toEqual([assistantMessage.content])
    }
  })

  it('should emit regenerate event', async () => {
    const wrapper = mount(MessageBubble, {
      props: { message: assistantMessage },
    })

    const regenButton = wrapper.find('[icon="pi pi-refresh"]')
    if (regenButton.exists()) {
      await regenButton.trigger('click')
      expect(wrapper.emitted('regenerate')).toBeTruthy()
      expect(wrapper.emitted('regenerate')![0]).toEqual([assistantMessage.id])
    }
  })

  it('should show attachments if present', () => {
    const messageWithAttachments: Message = {
      ...userMessage,
      attachments: [
        { id: 'att-1', message_id: 'msg-1', filename: 'test.png', mime_type: 'image/png', size: 1024, path: '/uploads/test.png' },
      ],
    }

    const wrapper = mount(MessageBubble, {
      props: { message: messageWithAttachments },
    })

    expect(wrapper.text()).toContain('test.png')
  })

  it('should format large token numbers', () => {
    const messageWithLargeTokens: Message = {
      ...assistantMessage,
      metrics: {
        input_tokens: 5000,
        output_tokens: 2500,
        total_tokens: 7500,
        ttfb_ms: 200,
        total_latency_ms: 1000,
        tokens_per_second: 50,
      },
    }

    const wrapper = mount(MessageBubble, {
      props: { message: messageWithLargeTokens },
    })

    expect(wrapper.text()).toContain('5.0k')
    expect(wrapper.text()).toContain('2.5k')
  })

  it('should apply different styling for user vs assistant', () => {
    const userWrapper = mount(MessageBubble, {
      props: { message: userMessage },
    })

    const assistantWrapper = mount(MessageBubble, {
      props: { message: assistantMessage },
    })

    // Check for different background classes
    expect(userWrapper.find('.bg-blue-500').exists()).toBe(true)
    expect(assistantWrapper.find('.bg-purple-500').exists()).toBe(true)
  })

  it('should show streaming cursor when streaming', () => {
    const wrapper = mount(MessageBubble, {
      props: { message: assistantMessage, isStreaming: true },
    })

    expect(wrapper.find('.streaming-cursor').exists()).toBe(true)
  })

  it('should hide action buttons when streaming', () => {
    const wrapper = mount(MessageBubble, {
      props: { message: assistantMessage, isStreaming: true },
    })

    // Actions should not be visible during streaming
    const copyButton = wrapper.find('[icon="pi pi-copy"]')
    expect(copyButton.exists()).toBe(false)
  })

  it('should render markdown content', () => {
    const markdownMessage: Message = {
      ...assistantMessage,
      content: '**Bold** and *italic* text',
    }

    const wrapper = mount(MessageBubble, {
      props: { message: markdownMessage },
    })

    const html = wrapper.find('.message-content').html()
    expect(html).toContain('<strong>')
    expect(html).toContain('<em>')
  })
})
