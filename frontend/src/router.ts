type Handler = (params: Record<string, string>) => Promise<HTMLElement>

const routes: Array<{ pattern: RegExp; keys: string[]; handler: Handler }> = []

export function addRoute(path: string, handler: Handler) {
  const keys: string[] = []
  const pattern = new RegExp(
    '^' + path.replace(/:([^/]+)/g, (_: string, key: string) => { keys.push(key); return '([^/]+)' }) + '$'
  )
  routes.push({ pattern, keys, handler })
}

export async function navigate(path: string) {
  window.history.pushState({}, '', path)
  await render(path)
}

async function render(path: string) {
  const app = document.getElementById('app')!
  const pathname = path.split('?')[0]
  for (const route of routes) {
    const match = pathname.match(route.pattern)
    if (match) {
      const params: Record<string, string> = {}
      route.keys.forEach((key, i) => { params[key] = match[i + 1] })

      // Brief fade-out, swap content, fade-in
      const hasContent = app.children.length > 0
      if (hasContent) {
        app.classList.add('route-exit')
        await new Promise<void>(r => {
          const fallback = setTimeout(r, 150)
          app.addEventListener('transitionend', () => { clearTimeout(fallback); r() }, { once: true })
        })
      }

      app.innerHTML = ''
      const el = await route.handler(params)
      app.appendChild(el)
      app.classList.remove('route-exit')
      document.getElementById('main-content')?.focus({ preventScroll: true })
      window.scrollTo(0, 0)
      return
    }
  }
  const { t } = await import('./i18n')
  app.innerHTML = `<div class="flex items-center justify-center h-screen text-muted">${t('common.page_not_found')}</div>`
}

export function initRouter() {
  window.addEventListener('popstate', () => render(location.pathname))
  document.addEventListener('click', (e) => {
    const target = (e.target as HTMLElement).closest('a[data-link]') as HTMLAnchorElement | null
    if (target) {
      e.preventDefault()
      navigate(target.getAttribute('href')!)
    }
  })
  render(location.pathname)
}
