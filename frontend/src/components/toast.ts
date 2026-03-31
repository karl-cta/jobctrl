type ToastType = 'success' | 'error' | 'info'

const typeStyles: Record<ToastType, string> = {
  success: 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-800/60 dark:bg-emerald-950/90 dark:text-emerald-200',
  error: 'border-rose-200 bg-rose-50 text-rose-800 dark:border-rose-800/60 dark:bg-rose-950/90 dark:text-rose-200',
  info: 'border-border bg-surface-1 text-primary/80',
}

let container: HTMLElement | null = null

function getContainer(): HTMLElement {
  if (container && document.body.contains(container)) return container
  container = document.createElement('div')
  container.className = 'fixed bottom-4 right-4 z-[60] flex flex-col gap-2 max-w-sm'
  container.setAttribute('aria-live', 'polite')
  container.setAttribute('aria-atomic', 'false')
  document.body.appendChild(container)
  return container
}

export function toast(message: string, type: ToastType = 'info', durationMs = 4000) {
  const el = document.createElement('div')
  el.className = `border rounded-xl px-4 py-3 text-sm font-medium transition-all duration-200 ${typeStyles[type]}`
  el.style.boxShadow = 'var(--shadow-elevated)'
  el.setAttribute('role', type === 'error' ? 'alert' : 'status')
  el.textContent = message

  el.style.opacity = '0'
  el.style.transform = 'translateY(8px)'
  getContainer().appendChild(el)

  requestAnimationFrame(() => {
    el.style.opacity = '1'
    el.style.transform = 'translateY(0)'
  })

  setTimeout(() => {
    el.style.opacity = '0'
    el.style.transform = 'translateY(8px)'
    setTimeout(() => el.remove(), 200)
  }, durationMs)
}
