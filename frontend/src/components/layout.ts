import { icons } from '../icons'
import { t, getLocale, setLocale } from '../i18n'
import { toggleTheme, isDark } from '../theme'
import { navigate } from '../router'

function navLink(href: string, label: string): string {
  const active = location.pathname === href || (href !== '/' && location.pathname.startsWith(href))
  const base = 'px-3 py-1.5 rounded-lg text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50'
  const cls = active
    ? `${base} text-accent bg-accent/10`
    : `${base} text-muted hover:text-primary`
  return `<a href="${href}" data-link class="${cls}" ${active ? 'aria-current="page"' : ''}>${label}</a>`
}

export function createLayout(content: HTMLElement): HTMLElement {
  const wrapper = document.createElement('div')
  wrapper.className = 'min-h-screen bg-surface text-primary flex flex-col'

  const locale = getLocale()
  const dark = isDark()

  wrapper.innerHTML = `
    <a href="#main-content" class="skip-link">${t('common.skip_to_content')}</a>

    <header class="border-b border-border bg-surface-1/80 backdrop-blur-sm sticky top-0 z-30">
      <div class="max-w-6xl mx-auto px-5 sm:px-8 h-14 flex items-center justify-between gap-4">
        <div class="flex items-center gap-2 sm:gap-6">
          <button id="mobile-menu-btn" class="sm:hidden btn-ghost p-2 min-w-[44px] min-h-[44px] -ml-2" aria-label="${t('nav.open_menu')}" aria-expanded="false" aria-controls="mobile-nav">
            ${icons.menu}
          </button>
          <a href="/" data-link class="font-bold text-lg tracking-tight text-primary hover:opacity-70 transition-opacity focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent rounded shrink-0">Job<span class="text-accent">Ctrl</span></a>
          <nav class="hidden sm:flex items-center gap-0.5" aria-label="${t('nav.main')}">
            ${navLink('/', t('nav.dashboard'))}
            ${navLink('/applications', t('nav.applications'))}
          </nav>
        </div>
        <div class="flex items-center gap-1.5">
          <a href="/applications/new" data-link class="btn-primary gap-1.5">${icons.plus} <span class="hidden xs:inline">${t('nav.new')}</span></a>
          <button id="locale-btn" class="btn-ghost p-2 min-w-[44px] min-h-[44px] text-xs font-medium" aria-label="${locale === 'fr' ? 'Switch to English' : 'Passer en français'}">
            ${locale.toUpperCase()}
          </button>
          <button id="theme-btn" class="btn-ghost p-2 min-w-[44px] min-h-[44px]" aria-label="${t('common.theme_toggle')}">
            ${dark ? icons.sun : icons.moon}
          </button>
        </div>
      </div>
    </header>

    <div id="mobile-nav" class="sm:hidden hidden border-b border-border bg-surface-1" role="navigation" aria-label="${t('nav.mobile_menu')}">
      <div class="max-w-6xl mx-auto px-5 py-2.5 flex gap-1">
        ${navLink('/', t('nav.dashboard'))}
        ${navLink('/applications', t('nav.applications'))}
      </div>
    </div>

    <main id="main-content" class="max-w-6xl mx-auto px-5 sm:px-8 py-8 sm:py-12 focus:outline-none flex-1 w-full" tabindex="-1"></main>

    <footer class="border-t border-border mt-auto py-6">
      <div class="max-w-6xl mx-auto px-5 sm:px-8 flex items-center justify-between text-xs text-muted">
        <span>Job<span class="text-accent font-semibold">Ctrl</span> <span id="app-version"></span></span>
        <div class="flex items-center gap-3">
          <span>${t('common.open_source')}</span>
          <a href="https://github.com/karl-cta/jobctrl" target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1 hover:text-primary transition-colors" aria-label="${t('common.source_code')}">
            ${icons.github} GitHub
          </a>
        </div>
      </div>
    </footer>
  `

  wrapper.querySelector('#main-content')!.appendChild(content)

  fetch('/api/health').then(r => r.json()).then(data => {
    const el = wrapper.querySelector('#app-version')
    if (el && data.version) el.textContent = data.version
  }).catch(() => {})

  // Mobile nav toggle
  const mobileNav = wrapper.querySelector('#mobile-nav') as HTMLElement
  const menuBtn = wrapper.querySelector('#mobile-menu-btn') as HTMLElement

  menuBtn?.addEventListener('click', () => {
    const isHidden = mobileNav.classList.toggle('hidden')
    menuBtn.setAttribute('aria-expanded', String(!isHidden))
  })
  mobileNav.querySelectorAll('a[data-link]').forEach(a => {
    a.addEventListener('click', () => {
      mobileNav.classList.add('hidden')
      menuBtn.setAttribute('aria-expanded', 'false')
    })
  })

  // Theme toggle
  const handleTheme = () => { toggleTheme(); navigate(location.pathname) }
  wrapper.querySelector('#theme-btn')?.addEventListener('click', handleTheme)

  // Locale toggle
  const handleLocale = () => { setLocale(locale === 'fr' ? 'en' : 'fr'); navigate(location.pathname) }
  wrapper.querySelector('#locale-btn')?.addEventListener('click', handleLocale)

  return wrapper
}
