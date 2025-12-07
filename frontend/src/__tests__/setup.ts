import { config } from '@vue/test-utils'
import { vi } from 'vitest'

// Mock PrimeVue components
config.global.stubs = {
  Button: true,
  Dialog: true,
  Select: true,
  Textarea: true,
  ProgressSpinner: true,
  Accordion: true,
  AccordionPanel: true,
  AccordionHeader: true,
  AccordionContent: true,
  TabView: true,
  TabPanel: true,
  FileUpload: true,
  Badge: true,
  'router-link': true,
  'router-view': true,
}

// Mock window.matchMedia
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

// Mock localStorage
const localStorageMock = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  removeItem: vi.fn(),
  clear: vi.fn(),
}
Object.defineProperty(window, 'localStorage', { value: localStorageMock })

// Mock clipboard
Object.defineProperty(navigator, 'clipboard', {
  value: {
    writeText: vi.fn(),
  },
})
