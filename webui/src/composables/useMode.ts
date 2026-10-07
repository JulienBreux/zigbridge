import { ref, computed } from 'vue'
import type { UserMode } from '@/types'

const savedMode = (localStorage.getItem('zigbridge_mode') as UserMode) || 'simple'
const currentMode = ref<UserMode>(savedMode)

const savedTheme = localStorage.getItem('zigbridge_theme')
const isDark = ref<boolean>(savedTheme ? savedTheme === 'dark' : true)

// Apply theme class to <html>
function applyTheme(dark: boolean) {
  if (dark) {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
}

// Initial theme apply
applyTheme(isDark.value)

export function useMode() {
  const isAdvanced = computed(() => currentMode.value === 'advanced')

  function toggleMode() {
    currentMode.value = currentMode.value === 'simple' ? 'advanced' : 'simple'
    localStorage.setItem('zigbridge_mode', currentMode.value)
  }

  function toggleTheme() {
    isDark.value = !isDark.value
    localStorage.setItem('zigbridge_theme', isDark.value ? 'dark' : 'light')
    applyTheme(isDark.value)
  }

  return {
    mode: currentMode,
    isAdvanced,
    isDark,
    toggleMode,
    toggleTheme,
  }
}
