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

/** Creates the live region up front: screen readers announce reliably only
 *  what is added to a region that already exists. */
export function initToasts() {
  getContainer()
}

function getContainer(): HTMLElement {
  if (container && document.body.contains(container)) return container
  container = document.createElement('div')
  container.className = 'fixed bottom-4 right-4 z-[60] flex flex-col gap-2 max-w-sm'
  container.setAttribute('aria-live', 'polite')
  container.setAttribute('aria-atomic', 'false')
  document.body.appendChild(container)
  return container
}

/** Burst of small particles from an element — for celebrating milestones */
export function celebrate(anchor?: HTMLElement) {
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return

  const rect = anchor?.getBoundingClientRect()
  const cx = rect ? rect.left + rect.width / 2 : window.innerWidth / 2
  const cy = rect ? rect.top + rect.height / 2 : window.innerHeight / 2

  const colors = ['#10b981', '#14b8a6', '#0ea5e9', '#f59e0b', '#8b5cf6']

  for (let i = 0; i < 12; i++) {
    const dot = document.createElement('div')
    dot.className = 'celebrate-particle'
    const angle = (Math.PI * 2 * i) / 12 + (Math.random() - 0.5) * 0.4
    const dist = 40 + Math.random() * 50
    dot.style.setProperty('--dx', `${Math.cos(angle) * dist}px`)
    dot.style.setProperty('--dy', `${Math.sin(angle) * dist - 20}px`)
    dot.style.left = `${cx}px`
    dot.style.top = `${cy}px`
    dot.style.backgroundColor = colors[i % colors.length]
    dot.style.animationDelay = `${i * 25}ms`
    document.body.appendChild(dot)
    setTimeout(() => dot.remove(), 800)
  }
}

/** How long a toast stays: longer for errors, and for long messages so they
 *  can be read to the end. */
function defaultDuration(message: string, type: ToastType): number {
  const base = type === 'error' ? 8000 : 4000
  return Math.min(Math.max(base, 2000 + message.length * 60), 15000)
}

export function toast(message: string, type: ToastType = 'info', durationMs?: number) {
  const el = document.createElement('div')
  el.className = `border rounded px-4 py-3 text-sm font-medium flex items-center gap-2.5 ${typeStyles[type]}`
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
  el.style.transform = 'translateY(8px)'
  getContainer().appendChild(el)

  requestAnimationFrame(() => {
    el.style.transition = 'opacity 0.2s ease, transform 0.2s ease'
    el.style.opacity = '1'
    el.style.transform = 'translateY(0)'
  })

  let dismissed = false
  const dismiss = () => {
    if (dismissed) return
    dismissed = true
    el.style.opacity = '0'
    el.style.transform = 'translateY(8px)'
    setTimeout(() => el.remove(), 200)
  }

  // The countdown pauses while the pointer is over the toast.
  let remaining = durationMs ?? defaultDuration(message, type)
  let startedAt = Date.now()
  let timer = setTimeout(dismiss, remaining)
  el.addEventListener('mouseenter', () => {
    clearTimeout(timer)
    remaining -= Date.now() - startedAt
  })
  el.addEventListener('mouseleave', () => {
    if (dismissed) return
    startedAt = Date.now()
    remaining = Math.max(remaining, 1500)
    timer = setTimeout(dismiss, remaining)
  })
}
