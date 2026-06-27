import { ref, watch } from 'vue'

const THEME_KEY = 'clash-sub-theme'

const theme = ref(loadTheme())

function loadTheme() {
  const saved = localStorage.getItem(THEME_KEY)
  if (saved === 'light' || saved === 'dark') return saved
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function applyTheme(t) {
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

  function setTheme(t) {
    theme.value = t
  }

  return { theme, toggle, setTheme }
}
