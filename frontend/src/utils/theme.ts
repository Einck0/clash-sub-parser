import { ref, watch } from 'vue'

type Theme = 'light' | 'dark'

const THEME_KEY = 'clash-sub-theme'

const theme = ref<Theme>(loadTheme())

function loadTheme(): Theme {
  const saved = localStorage.getItem(THEME_KEY)
  if (saved === 'light' || saved === 'dark') return saved
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function applyTheme(t: Theme) {
  document.documentElement.setAttribute('data-theme', t)
  localStorage.setItem(THEME_KEY, t)
}

// Apply immediately
applyTheme(theme.value)

// Watch for changes
watch(theme, (t) => applyTheme(t))

export function useTheme() {
  function toggle() {
    theme.value = theme.value === 'dark' ? 'light' : 'dark'
  }

  function setTheme(t: Theme) {
    theme.value = t
  }

  return { theme, toggle, setTheme }
}
