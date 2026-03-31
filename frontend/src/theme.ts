export function initTheme() {
  const saved = localStorage.getItem('jc-theme')
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
  localStorage.setItem('jc-theme', isDarkNow ? 'dark' : 'light')
}

export function isDark(): boolean {
  return document.documentElement.classList.contains('dark')
}
