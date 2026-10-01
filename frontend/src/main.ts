import './style.css'
import { addRoute, initRouter, loadChunk } from './router'
import { initTheme } from './theme'
import { t } from './i18n'
import { initToasts } from './components/toast'

initTheme()
initToasts()

// Images that opt in with `data-hide-on-error` (favicons) hide themselves when
// they fail to load. This replaces inline onerror attributes, which the
// Content-Security-Policy forbids. `error` does not bubble, hence the capture.
document.addEventListener('error', (e) => {
  const el = e.target
  if (el instanceof HTMLImageElement && el.hasAttribute('data-hide-on-error')) el.style.display = 'none'
}, true)

async function dashboardPage() {
  const { DashboardPage } = await loadChunk(() => import('./pages/dashboard'))
  return DashboardPage()
}

async function listPage() {
  const { ListPage } = await loadChunk(() => import('./pages/list'))
  return ListPage()
}

async function detailPage(params: Record<string, string>) {
  const { DetailPage } = await loadChunk(() => import('./pages/detail'))
  return DetailPage(params.id)
}

async function formPage(params: Record<string, string>) {
  const { FormPage } = await loadChunk(() => import('./pages/form'))
  return FormPage(params.id)
}

// Without a title getter, the tab title comes from the page's <h1>: the
// company name on the detail page.
addRoute('/', dashboardPage, () => t('nav.dashboard'))
addRoute('/applications', listPage, () => t('list.title'))
addRoute('/applications/new', () => formPage({}), () => t('form.title_new'))
addRoute('/applications/:id', detailPage)
addRoute('/applications/:id/edit', formPage, () => t('form.title_edit'))

initRouter()
