import './style.css'
import { addRoute, initRouter } from './router'
import { initTheme } from './theme'

initTheme()

async function dashboardPage() {
  const { DashboardPage } = await import('./pages/dashboard')
  return DashboardPage()
}

async function listPage() {
  const { ListPage } = await import('./pages/list')
  return ListPage()
}

async function detailPage(params: Record<string, string>) {
  const { DetailPage } = await import('./pages/detail')
  return DetailPage(params.id)
}

async function formPage(params: Record<string, string>) {
  const { FormPage } = await import('./pages/form')
  return FormPage(params.id)
}

addRoute('/', dashboardPage)
addRoute('/applications', listPage)
addRoute('/applications/new', () => formPage({}))
addRoute('/applications/:id', detailPage)
addRoute('/applications/:id/edit', formPage)

initRouter()
