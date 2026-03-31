import { t } from '../i18n'

export function openModal(opts: { title: string; content: HTMLElement; onClose?: () => void }): HTMLElement {
  const overlay = document.createElement('div')
  overlay.className = 'fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm'
  overlay.setAttribute('role', 'dialog')
  overlay.setAttribute('aria-modal', 'true')
  overlay.setAttribute('aria-label', opts.title)

  const modal = document.createElement('div')
  modal.className = 'bg-surface-1 border border-border rounded-2xl max-w-lg w-full mx-4 max-h-[85vh] flex flex-col'
  modal.style.boxShadow = 'var(--shadow-elevated)'

  const header = document.createElement('div')
  header.className = 'flex items-center justify-between px-5 py-4 border-b border-border'
  header.innerHTML = `
    <h2 class="text-base font-semibold text-primary">${opts.title}</h2>
    <button class="btn-ghost p-1.5 -mr-1.5 min-w-[44px] min-h-[44px]" data-modal-close aria-label="${t('common.close')}">
      <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" class="w-5 h-5" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
    </button>
  `

  const body = document.createElement('div')
  body.className = 'px-5 py-5 overflow-y-auto'
  body.appendChild(opts.content)

  modal.appendChild(header)
  modal.appendChild(body)
  overlay.appendChild(modal)

  const previouslyFocused = document.activeElement as HTMLElement | null

  const onKeydown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      close()
      return
    }
    if (e.key === 'Tab') {
      const focusable = modal.querySelectorAll<HTMLElement>(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
      )
      if (!focusable.length) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault()
        first.focus()
      }
    }
  }

  function close() {
    document.removeEventListener('keydown', onKeydown)
    overlay.remove()
    document.body.style.overflow = ''
    previouslyFocused?.focus()
    opts.onClose?.()
  }

  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) close()
  })
  header.querySelector('[data-modal-close]')?.addEventListener('click', close)
  document.addEventListener('keydown', onKeydown)

  document.body.style.overflow = 'hidden'
  document.body.appendChild(overlay)

  const closeBtn = header.querySelector('[data-modal-close]') as HTMLElement
  closeBtn?.focus()

  return overlay
}
