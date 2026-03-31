type ToastType = 'success' | 'error' | 'info'

const typeStyles: Record<ToastType, string> = {
  success: 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-800/60 dark:bg-emerald-950/90 dark:text-emerald-200',
  error: 'border-rose-200 bg-rose-50 text-rose-800 dark:border-rose-800/60 dark:bg-rose-950/90 dark:text-rose-200',
  info: 'border-border bg-surface-1 text-primary/80',
}

const typeIcons: Record<ToastType, string> = {
  success: `<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="2.5" stroke="currentColor" class="w-5 h-5 shrink-0" aria-hidden="true"><circle cx="12" cy="12" r="9.5" stroke-width="1.5" opacity="0.15"/><path stroke-linecap="round" stroke-linejoin="round" d="M7.5 12.5l3 3 6-8" pathLength="1" class="check-draw"/></svg>`,
  error: `<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" class="w-5 h-5 shrink-0" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z"/></svg>`,
  info: '',
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
  el.className = `border rounded-xl px-4 py-3 text-sm font-medium flex items-center gap-2.5 ${typeStyles[type]}`
  el.style.boxShadow = 'var(--shadow-elevated)'
  el.setAttribute('role', type === 'error' ? 'alert' : 'status')

  const icon = typeIcons[type]
  if (icon) {
    el.insertAdjacentHTML('beforeend', icon)
  }

  const textSpan = document.createElement('span')
  textSpan.textContent = message
  el.appendChild(textSpan)

  el.style.opacity = '0'
  el.style.transform = 'translateY(8px) scale(0.96)'
  getContainer().appendChild(el)

  requestAnimationFrame(() => {
    el.style.transition = 'all 0.25s cubic-bezier(0.16, 1, 0.3, 1)'
    el.style.opacity = '1'
    el.style.transform = 'translateY(0) scale(1)'
  })

  setTimeout(() => {
    el.style.opacity = '0'
    el.style.transform = 'translateY(8px) scale(0.96)'
    setTimeout(() => el.remove(), 250)
  }, durationMs)
}
