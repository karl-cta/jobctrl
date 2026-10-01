// localStorage can throw (site data blocked, some private modes): the theme
// then simply is not remembered.
function readSavedTheme(): string | null {
  try {
    return localStorage.getItem('jc-theme')
  } catch {
    return null
  }
}

export function initTheme() {
  const saved = readSavedTheme()
  if (saved === 'light') {
    document.documentElement.classList.remove('dark')
  } else if (saved === 'dark') {
    document.documentElement.classList.add('dark')
  } else {
    // No saved preference — respect system preference, default dark
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
    if (!prefersDark) {
      document.documentElement.classList.remove('dark')
    }
  }
}

export function toggleTheme() {
  const isDarkNow = document.documentElement.classList.toggle('dark')
  try {
    localStorage.setItem('jc-theme', isDarkNow ? 'dark' : 'light')
  } catch { /* storage blocked: the choice lasts for this page only */ }
}

export function isDark(): boolean {
  return document.documentElement.classList.contains('dark')
}
