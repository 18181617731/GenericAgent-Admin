import { Moon, Sun, Sunset } from 'lucide-react'

// Appearance is data-driven: add a registry entry and a matching CSS token scope.
// `colorScheme` selects shared light/dark compatibility rules; theme IDs only select palettes.
// Keep the order intentional; pickers render this registry as an explicit choice list.
export const THEMES = Object.freeze([
  {
    id: 'light',
    colorScheme: 'light',
    icon: Sun,
    label: { zh: '\u6d45\u8272', en: 'Light' },
    description: { zh: '\u6e05\u6670\u3001\u514b\u5236\u7684\u51b7\u8c03\u754c\u9762', en: 'Crisp, restrained cool tones' },
    preview: ['#f7f8fb', '#ffffff', '#171717'],
    antdAlgorithm: 'default',
    antdToken: { colorBgBase: '#ffffff', colorTextBase: '#171717', colorBorder: 'rgba(23, 23, 23, .14)' },
  },
  {
    id: 'warm',
    colorScheme: 'light',
    icon: Sunset,
    label: { zh: '暖色', en: 'Warm' },
    description: { zh: '温暖金色调卡其色系统', en: 'Warm gold-tinted khaki palette' },
    preview: ['#FEFCF7', '#FAF7F0', '#1A1610'],
    antdAlgorithm: 'default',
    antdToken: {
      colorBgBase: '#FEFCF7',
      colorTextBase: '#1A1610',
      colorBorder: '#E0D8C8',
      colorPrimary: '#B8860B',
      colorSuccess: '#3D8B40',
      colorWarning: '#E68A00',
      colorError: '#C62828',
      colorInfo: '#0288A8',
      colorBgContainer: '#F3EEE3',
      colorBgLayout: '#FAF7F0',
      colorBgElevated: '#F3EEE3',
    },
  },
  {
    id: 'dark',
    colorScheme: 'dark',
    icon: Moon,
    label: { zh: '\u6df1\u8272', en: 'Dark' },
    description: { zh: '\u4f4e\u7729\u5149\u7684\u6df1\u8272\u5de5\u4f5c\u533a', en: 'Low-glare dark workspace' },
    preview: ['#111214', '#202124', '#e6e7e9'],
    antdAlgorithm: 'dark',
    antdToken: { colorBgBase: '#191c21', colorTextBase: '#ececf1', colorBorder: '#343a42' },
  },
])

export const DEFAULT_THEME_ID = 'warm'

const themeById = new Map(THEMES.map(theme => [theme.id, theme]))

export const isThemeId = value => themeById.has(value)

export const getTheme = value => themeById.get(value) || themeById.get(DEFAULT_THEME_ID)

export const getNextThemeId = value => {
  const currentIndex = THEMES.findIndex(theme => theme.id === value)
  return THEMES[(currentIndex + 1) % THEMES.length].id
}

export const getThemeLabel = (value, lang = 'en') => {
  const theme = getTheme(value)
  return theme.label[lang] || theme.label.en
}

export const applyThemeToDocument = (value, documentRef = globalThis.document) => {
  const theme = getTheme(value)
  const root = documentRef?.documentElement
  if (root) {
    root.dataset.theme = theme.id
    root.dataset.colorScheme = theme.colorScheme
  }
  return theme
}

export const getInitialTheme = () => {
  if (typeof window === 'undefined') return DEFAULT_THEME_ID
  const stored = window.localStorage.getItem('ga-admin-theme')
  return isThemeId(stored) ? stored : DEFAULT_THEME_ID
}
