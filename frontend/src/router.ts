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
  for (const route of routes) {
    const match = path.match(route.pattern)
    if (match) {
      const params: Record<string, string> = {}
      route.keys.forEach((key, i) => { params[key] = match[i + 1] })
      app.innerHTML = ''
      const el = await route.handler(params)
      app.appendChild(el)
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
