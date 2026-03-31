export interface TableColumn<T> {
  key: string
  label: string
  render?: (item: T) => string
  class?: string
}

export function createTable<T>(opts: {
  columns: TableColumn<T>[]
  data: T[]
  onRowClick?: (item: T) => void
  rowKey: (item: T) => string
}): HTMLElement {
  const table = document.createElement('div')
  table.className = 'overflow-x-auto'

  const headerCells = opts.columns
    .map(col => `<th class="px-4 py-3 text-left text-xs font-medium text-muted uppercase tracking-wider ${col.class || ''}">${col.label}</th>`)
    .join('')

  const rows = opts.data
    .map(item => {
      const cells = opts.columns
        .map(col => {
          const value = col.render
            ? col.render(item)
            : String((item as Record<string, unknown>)[col.key] ?? '')
          return `<td class="px-4 py-3 text-sm text-primary/80 ${col.class || ''}">${value}</td>`
        })
        .join('')
      const clickClass = opts.onRowClick ? 'cursor-pointer hover:bg-surface-2' : ''
      return `<tr class="border-b border-border ${clickClass}" data-row-key="${opts.rowKey(item)}">${cells}</tr>`
    })
    .join('')

  table.innerHTML = `
    <table class="w-full">
      <thead class="border-b border-border bg-surface-2/50">
        <tr>${headerCells}</tr>
      </thead>
      <tbody class="divide-y divide-border">
        ${rows || '<tr><td colspan="' + opts.columns.length + '" class="px-4 py-8 text-center text-muted text-sm">Aucune donn\u00e9e</td></tr>'}
      </tbody>
    </table>
  `

  if (opts.onRowClick) {
    table.querySelectorAll('[data-row-key]').forEach(row => {
      row.addEventListener('click', () => {
        const key = row.getAttribute('data-row-key')!
        const item = opts.data.find(d => opts.rowKey(d) === key)
        if (item) opts.onRowClick!(item)
      })
    })
  }

  return table
}
