import { icons } from '../icons'
import { t, getLocale, setLocale } from '../i18n'
import { toggleTheme, isDark } from '../theme'
import { navigate } from '../router'

function navLink(href: string, icon: string, label: string): string {
  const active = location.pathname === href || (href !== '/' && location.pathname.startsWith(href))
  const base = 'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50'
  const style = active
    ? `${base} bg-accent/10 text-accent`
    : `${base} text-muted hover:text-primary hover:bg-surface-2`
  const ariaCurrent = active ? 'aria-current="page"' : ''
  return `<a href="${href}" data-link class="${style}" ${ariaCurrent}>${icon}<span>${label}</span></a>`
}

export function createLayout(content: HTMLElement): HTMLElement {
  const wrapper = document.createElement('div')
  wrapper.className = 'min-h-screen flex bg-surface text-primary'

  const locale = getLocale()
  const dark = isDark()

  wrapper.innerHTML = `
    <a href="#main-content" class="skip-link">${t('common.skip_to_content')}</a>

    <aside id="sidebar" class="hidden lg:flex flex-col w-[220px] shrink-0 border-r border-border bg-surface-1 fixed inset-y-0 left-0 z-30" aria-label="${t('nav.sidebar')}">
      <div class="h-14 flex items-center gap-2.5 px-5">
        <div class="w-7 h-7 rounded-lg accent-glow flex items-center justify-center">
          <span class="text-white [&>svg]:w-4 [&>svg]:h-4" aria-hidden="true">${icons.zap}</span>
        </div>
        <a href="/" data-link class="font-semibold text-[15px] text-primary hover:text-accent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent rounded">Job-Ctrl</a>
      </div>
      <nav class="flex-1 flex flex-col gap-0.5 px-3 pt-4" aria-label="${t('nav.main')}">
        ${navLink('/', icons.dashboard, t('nav.dashboard'))}
        ${navLink('/applications', icons.briefcase, t('nav.applications'))}
      </nav>
      <div class="px-3 pb-3 space-y-2">
        <a href="/applications/new" data-link class="flex items-center justify-center gap-2 rounded-lg px-3 py-2.5 text-sm font-medium bg-accent text-white hover:bg-accent-hover transition-all duration-150 active:scale-[0.98] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50">${icons.plus} <span>${t('nav.new_application')}</span></a>
        <div class="flex items-center gap-1 pt-2 border-t border-border">
          <button id="locale-btn" class="btn-ghost py-1.5 px-2 flex-1 text-xs gap-1.5" aria-label="${locale === 'fr' ? 'Switch to English' : 'Passer en français'}">
            ${icons.globe} <span aria-hidden="true">${locale.toUpperCase()}</span>
          </button>
          <button id="theme-btn" class="btn-ghost py-1.5 px-2" aria-label="${t('common.theme_toggle')}">
            ${dark ? icons.sun : icons.moon}
          </button>
        </div>
      </div>
    </aside>

    <div id="mobile-nav" class="lg:hidden fixed inset-0 z-50 hidden" role="dialog" aria-modal="true" aria-label="${t('nav.mobile_menu')}">
      <div class="absolute inset-0 bg-black/50 backdrop-blur-sm transition-opacity" id="mobile-nav-overlay"></div>
      <aside class="absolute left-0 top-0 bottom-0 w-72 bg-surface-1 border-r border-border flex flex-col transform transition-transform">
        <div class="h-14 flex items-center justify-between px-4">
          <span class="font-semibold text-[15px] text-primary flex items-center gap-2.5">
            <div class="w-7 h-7 rounded-lg accent-glow flex items-center justify-center">
              <span class="text-white [&>svg]:w-4 [&>svg]:h-4" aria-hidden="true">${icons.zap}</span>
            </div>
            Job-Ctrl
          </span>
          <button id="mobile-close-btn" class="btn-ghost p-1.5" aria-label="${t('common.close')}">${icons.close}</button>
        </div>
        <nav class="flex-1 flex flex-col gap-0.5 p-3" aria-label="${t('nav.main')}">
          ${navLink('/', icons.dashboard, t('nav.dashboard'))}
          ${navLink('/applications', icons.briefcase, t('nav.applications'))}
        </nav>
        <div class="p-3 border-t border-border">
          <a href="/applications/new" data-link class="flex items-center justify-center gap-2 rounded-lg px-3 py-2.5 text-sm font-medium bg-accent text-white hover:bg-accent-hover transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50">${icons.plus} <span>${t('nav.new_application')}</span></a>
        </div>
      </aside>
    </div>

    <div class="flex-1 flex flex-col lg:ml-[220px] min-w-0">
      <header class="h-14 flex items-center justify-between px-4 sm:px-6 border-b border-border bg-surface-1/80 backdrop-blur-sm lg:bg-transparent lg:border-0 sticky top-0 z-20">
        <button id="mobile-menu-btn" class="lg:hidden btn-ghost p-1.5 -ml-1.5" aria-label="${t('nav.open_menu')}" aria-expanded="false" aria-controls="mobile-nav">
          ${icons.menu}
        </button>
        <div class="lg:hidden flex items-center gap-2.5">
          <div class="w-7 h-7 rounded-lg accent-glow flex items-center justify-center">
            <span class="text-white [&>svg]:w-4 [&>svg]:h-4" aria-hidden="true">${icons.zap}</span>
          </div>
          <span class="font-semibold text-primary">Job-Ctrl</span>
        </div>
        <div class="flex items-center gap-1 lg:hidden">
          <button id="mobile-locale-btn" class="btn-ghost p-1.5" aria-label="${locale === 'fr' ? 'Switch to English' : 'Passer en français'}">${icons.globe}</button>
          <button id="mobile-theme-btn" class="btn-ghost p-1.5" aria-label="${t('common.theme_toggle')}">${dark ? icons.sun : icons.moon}</button>
        </div>
      </header>
      <main id="main-content" class="flex-1 px-4 sm:px-6 lg:px-8 py-6 lg:py-8 w-full max-w-6xl" tabindex="-1"></main>
    </div>
  `

  wrapper.querySelector('#main-content')!.appendChild(content)

  // Mobile menu
  const mobileNav = wrapper.querySelector('#mobile-nav') as HTMLElement
  const menuBtn = wrapper.querySelector('#mobile-menu-btn') as HTMLElement

  const openMobile = () => {
    mobileNav.classList.remove('hidden')
    menuBtn.setAttribute('aria-expanded', 'true')
    const closeBtn = mobileNav.querySelector('#mobile-close-btn') as HTMLElement
    closeBtn?.focus()
    document.body.style.overflow = 'hidden'
  }

  const closeMobile = () => {
    mobileNav.classList.add('hidden')
    menuBtn.setAttribute('aria-expanded', 'false')
    document.body.style.overflow = ''
    menuBtn.focus()
  }

  menuBtn?.addEventListener('click', openMobile)
  wrapper.querySelector('#mobile-close-btn')?.addEventListener('click', closeMobile)
  wrapper.querySelector('#mobile-nav-overlay')?.addEventListener('click', closeMobile)
  mobileNav.querySelectorAll('a[data-link]').forEach(a => {
    a.addEventListener('click', closeMobile)
  })

  // Theme toggles
  const handleTheme = () => { toggleTheme(); navigate(location.pathname) }
  wrapper.querySelector('#theme-btn')?.addEventListener('click', handleTheme)
  wrapper.querySelector('#mobile-theme-btn')?.addEventListener('click', handleTheme)

  // Locale toggles
  const handleLocale = () => { setLocale(locale === 'fr' ? 'en' : 'fr'); navigate(location.pathname) }
  wrapper.querySelector('#locale-btn')?.addEventListener('click', handleLocale)
  wrapper.querySelector('#mobile-locale-btn')?.addEventListener('click', handleLocale)

  return wrapper
}
