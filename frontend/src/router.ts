import { t } from './i18n'

type Handler = (params: Record<string, string>) => Promise<HTMLElement>
type TitleGetter = (params: Record<string, string>) => string

const routes: Array<{ pattern: RegExp; keys: string[]; handler: Handler; title?: TitleGetter }> = []

let navigationGuard: (() => boolean) | null = null
let navigationCleanup: (() => void) | null = null
// URL of the page that installed the guard: a cancelled Back navigation puts
// it back in the address bar (location already points at the destination).
let guardUrl = ''

export function setNavigationGuard(guard: (() => boolean) | null) {
  navigationGuard = guard
  guardUrl = guard ? location.pathname + location.search : ''
}

export function setNavigationCleanup(cleanup: (() => void) | null) {
  navigationCleanup = cleanup
}

function checkGuard(): boolean {
  if (navigationGuard && !navigationGuard()) return false
  return true
}

/** Marks a failed dynamic import, so render() can tell a stale chunk (a tab
 *  left open across an upgrade) from an ordinary page error. */
class ChunkLoadError extends Error {}

/** Wraps a page's dynamic import: `loadChunk(() => import('./pages/x'))`. */
export function loadChunk<T>(load: () => Promise<T>): Promise<T> {
  return load().catch((err: unknown) => {
    throw new ChunkLoadError(err instanceof Error ? err.message : String(err))
  })
}

const RELOAD_FLAG = 'jc-chunk-reload'

/** Reloads the page to pick up the new build, at most once per tab session
 *  so a server that is really down cannot cause a reload loop. */
function reloadOnce(): boolean {
  try {
    if (sessionStorage.getItem(RELOAD_FLAG)) return false
    sessionStorage.setItem(RELOAD_FLAG, '1')
  } catch {
    return false
  }
  location.reload()
  return true
}

function clearReloadFlag() {
  try { sessionStorage.removeItem(RELOAD_FLAG) } catch { /* storage blocked */ }
}

export function addRoute(path: string, handler: Handler, title?: TitleGetter) {
  const keys: string[] = []
  const pattern = new RegExp(
    '^' + path.replace(/:([^/]+)/g, (_: string, key: string) => { keys.push(key); return '([^/]+)' }) + '$'
  )
  routes.push({ pattern, keys, handler, title })
}

export async function navigate(path: string) {
  if (!checkGuard()) return
  window.history.pushState({}, '', path)
  await render(path)
}

/** Re-renders the current URL (query string included) without adding a
 *  history entry, e.g. after a language switch. The navigation guard is asked
 *  first and `beforeRender` only runs once it agrees, so a cancelled prompt
 *  leaves everything unchanged. Returns false when the guard said no. */
export function rerender(beforeRender?: () => void): boolean {
  if (!checkGuard()) return false
  beforeRender?.()
  void render(location.pathname + location.search)
  return true
}

function setTitle(text?: string | null) {
  const label = text?.trim()
  document.title = label ? `${label} · JobCtrl` : 'JobCtrl'
}

// Renders run one at a time. A navigation that arrives while a page is
// loading is queued (only the latest one is kept) and the loading page is
// dropped instead of shown. Because only one handler runs at a time, the
// guard and cleanup it registers are its own, and the next render's reset
// discards them cleanly.
let rendering = false
let queuedPath: string | null = null
// Path and query of the last page rendered. A history entry that only differs
// by its hash (the skip link) fires popstate too, and must not re-render it.
let renderedUrl = ''

/** Updates the address of the current page in place (filters kept in the
 *  query string) without re-rendering it. */
export function replaceUrl(url: string) {
  window.history.replaceState({}, '', url)
  renderedUrl = location.pathname + location.search
}

async function render(path: string) {
  if (rendering) {
    queuedPath = path
    return
  }
  rendering = true
  try {
    await renderNow(path)
  } finally {
    rendering = false
    if (queuedPath !== null) {
      const next = queuedPath
      queuedPath = null
      void render(next)
    }
  }
}

async function renderNow(path: string) {
  if (navigationCleanup) { navigationCleanup(); navigationCleanup = null }
  setNavigationGuard(null)
  // Read from location rather than `path`, so both are encoded the same way.
  renderedUrl = location.pathname + location.search

  const app = document.getElementById('app')!
  const pathname = path.split('?')[0]
  for (const route of routes) {
    const match = pathname.match(route.pattern)
    if (match) {
      const params: Record<string, string> = {}
      // api.ts encodes ids into paths, so params are decoded here to avoid
      // double encoding. A malformed escape is kept as is.
      route.keys.forEach((key, i) => {
        try {
          params[key] = decodeURIComponent(match[i + 1])
        } catch {
          params[key] = match[i + 1]
        }
      })

      // Brief fade-out, swap content, fade-in. The page starts loading its
      // code and data right away, in parallel with the fade.
      const pending = route.handler(params)
      const fade = app.children.length > 0
        ? new Promise<void>(r => {
          app.classList.add('route-exit')
          const fallback = setTimeout(r, 150)
          app.addEventListener('transitionend', () => { clearTimeout(fallback); r() }, { once: true })
        })
        : Promise.resolve()

      let el: HTMLElement
      try {
        el = (await Promise.all([pending, fade]))[0]
      } catch (err) {
        if (queuedPath !== null) return
        if (err instanceof ChunkLoadError && reloadOnce()) return
        console.error(err)
        renderError(app)
        return
      }

      // A newer navigation arrived meanwhile: it replaces this page.
      if (queuedPath !== null) return

      app.innerHTML = ''
      window.scrollTo(0, 0)
      app.appendChild(el)
      app.classList.remove('route-exit')
      setTitle(route.title ? route.title(params) : el.querySelector('h1')?.textContent)
      document.getElementById('main-content')?.focus({ preventScroll: true })
      clearReloadFlag()
      return
    }
  }
  app.innerHTML = `
    <div class="flex flex-col items-center justify-center h-screen gap-4 px-5 text-center text-muted">
      <p>${t('common.page_not_found')}</p>
      <a href="/" data-link class="btn-ghost">${t('nav.dashboard')}</a>
    </div>`
  app.classList.remove('route-exit')
  setTitle(t('common.page_not_found'))
}

/** Shown when a page fails to load, instead of leaving #app blank. */
function renderError(app: HTMLElement) {
  app.innerHTML = `
    <div class="flex flex-col items-center justify-center h-screen gap-4 px-5 text-center text-muted">
      <p>${t('common.page_error')}</p>
      <button type="button" class="btn-ghost" data-reload>${t('common.reload')}</button>
    </div>`
  app.querySelector('[data-reload]')?.addEventListener('click', () => location.reload())
  app.classList.remove('route-exit')
  setTitle()
}

export function initRouter() {
  window.addEventListener('popstate', () => {
    if (location.pathname + location.search === renderedUrl) return
    if (!checkGuard()) {
      window.history.pushState({}, '', guardUrl || location.pathname + location.search)
      return
    }
    void render(location.pathname + location.search)
  })
  document.addEventListener('click', (e) => {
    // Let the browser handle modified clicks (new tab, new window, download).
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
    const target = (e.target as Element | null)?.closest?.('a[data-link]') as HTMLAnchorElement | null
    if (!target || target.target === '_blank' || target.hasAttribute('download')) return
    e.preventDefault()
    void navigate(target.getAttribute('href')!)
  })
  void render(location.pathname + location.search)
}
