import { JOB_BOARDS, faviconUrl, type JobBoard } from '../job-boards'

interface Suggestion {
  name: string
  domain?: string
  aliases?: string[]
  fromUser: boolean
}

function matchScore(s: Suggestion, q: string): number {
  const name = s.name.toLowerCase()
  if (name === q) return 100
  if (name.startsWith(q)) return 80
  if (name.includes(q)) return 60
  if (s.domain?.toLowerCase().includes(q)) return 40
  if (s.aliases?.some(a => a.toLowerCase().includes(q))) return 30
  return 0
}

function buildSuggestions(userSources: string[]): Suggestion[] {
  const boardByName = new Map<string, JobBoard>()
  for (const jb of JOB_BOARDS) boardByName.set(jb.name.toLowerCase(), jb)

  const result: Suggestion[] = []
  const seen = new Set<string>()

  for (const src of userSources) {
    const key = src.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    const board = boardByName.get(key)
    result.push({
      name: src,
      domain: board?.domain,
      aliases: board?.aliases,
      fromUser: true,
    })
  }

  for (const jb of JOB_BOARDS) {
    const key = jb.name.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    result.push({ ...jb, fromUser: false })
  }

  return result
}

const MAX_RESULTS = 8

export function setupSourceAutocomplete(
  input: HTMLInputElement,
  fetchUserSources: () => Promise<string[]>,
): void {
  const wrapper = document.createElement('div')
  wrapper.className = 'relative'
  input.parentNode!.insertBefore(wrapper, input)
  wrapper.appendChild(input)
  input.setAttribute('autocomplete', 'off')

  const dropdown = document.createElement('div')
  dropdown.className = 'source-dropdown'
  dropdown.style.display = 'none'
  dropdown.setAttribute('role', 'listbox')
  dropdown.id = 'source-suggestions'
  wrapper.appendChild(dropdown)

  input.setAttribute('role', 'combobox')
  input.setAttribute('aria-autocomplete', 'list')
  input.setAttribute('aria-controls', 'source-suggestions')
  input.setAttribute('aria-expanded', 'false')

  let suggestions: Suggestion[] | null = null
  let selectedIndex = -1
  let visible = false

  async function ensureLoaded() {
    if (suggestions) return
    try {
      const userSources = await fetchUserSources()
      suggestions = buildSuggestions(userSources)
    } catch {
      suggestions = buildSuggestions([])
    }
  }

  function filter(query: string): Suggestion[] {
    if (!suggestions) return []
    const q = query.toLowerCase().trim()
    if (!q) return suggestions.slice(0, MAX_RESULTS)
    return suggestions
      .map(s => ({ s, score: matchScore(s, q) }))
      .filter(x => x.score > 0)
      .sort((a, b) => b.score - a.score)
      .slice(0, MAX_RESULTS)
      .map(x => x.s)
  }

  function renderOption(s: Suggestion, index: number): string {
    const active = index === selectedIndex
    const cls = `source-option${active ? ' source-option-active' : ''}`
    const attrs = `role="option" aria-selected="${active}" data-index="${index}"`

    const initial = s.name[0].toUpperCase()
    const favicon = s.domain
      ? `<img src="${faviconUrl(s.domain)}" width="16" height="16" alt="" class="source-favicon" data-initial="${initial}" />`
      : `<span class="source-favicon-placeholder">${initial}</span>`

    return `<div class="${cls}" ${attrs}>${favicon}<span>${esc(s.name)}</span></div>`
  }

  function render(results: Suggestion[]) {
    if (results.length === 0) {
      hide()
      return
    }

    dropdown.innerHTML = results.map(renderOption).join('')

    dropdown.querySelectorAll<HTMLImageElement>('.source-favicon').forEach(img => {
      img.onerror = () => {
        const span = document.createElement('span')
        span.className = 'source-favicon-placeholder'
        span.textContent = img.dataset.initial || '?'
        img.replaceWith(span)
      }
    })

    show()
  }

  function show() {
    if (visible) return
    dropdown.style.display = ''
    dropdown.classList.remove('dropdown-exit')
    dropdown.classList.add('dropdown-enter')
    input.setAttribute('aria-expanded', 'true')
    visible = true
  }

  function hide() {
    if (!visible) return
    dropdown.classList.remove('dropdown-enter')
    dropdown.classList.add('dropdown-exit')
    input.setAttribute('aria-expanded', 'false')
    visible = false
    selectedIndex = -1
    setTimeout(() => {
      if (!visible) {
        dropdown.style.display = 'none'
        dropdown.classList.remove('dropdown-exit')
      }
    }, 100)
  }

  function select(s: Suggestion) {
    input.value = s.name
    input.dispatchEvent(new Event('input', { bubbles: true }))
    hide()
  }

  function scrollToSelected() {
    const el = dropdown.querySelector('.source-option-active')
    el?.scrollIntoView({ block: 'nearest' })
  }

  input.addEventListener('input', () => {
    selectedIndex = -1
    render(filter(input.value))
  })

  input.addEventListener('focus', async () => {
    await ensureLoaded()
    render(filter(input.value))
  })

  input.addEventListener('keydown', (e) => {
    const results = filter(input.value)
    if (!visible && results.length > 0 && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      e.preventDefault()
      render(results)
      return
    }
    if (!visible) return

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        selectedIndex = Math.min(selectedIndex + 1, results.length - 1)
        render(results)
        scrollToSelected()
        break
      case 'ArrowUp':
        e.preventDefault()
        selectedIndex = Math.max(selectedIndex - 1, -1)
        render(results)
        scrollToSelected()
        break
      case 'Enter':
        if (selectedIndex >= 0 && results[selectedIndex]) {
          e.preventDefault()
          select(results[selectedIndex])
        }
        break
      case 'Escape':
        e.preventDefault()
        hide()
        break
      case 'Tab':
        hide()
        break
    }
  })

  dropdown.addEventListener('pointerdown', (e) => {
    e.preventDefault()
    const option = (e.target as HTMLElement).closest('.source-option') as HTMLElement | null
    if (!option) return
    const idx = Number(option.dataset.index)
    const results = filter(input.value)
    if (results[idx]) select(results[idx])
  })

  document.addEventListener('click', (e) => {
    if (!wrapper.contains(e.target as Node)) hide()
  })
}

function esc(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}
