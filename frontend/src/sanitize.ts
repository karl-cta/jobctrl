export function esc(str: string | null | undefined): string {
  if (!str) return ''
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

export function sanitizeUrl(url: string | null | undefined): string {
  if (!url) return ''
  try {
    const parsed = new URL(url)
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:') return url
    return ''
  } catch {
    return ''
  }
}

/** Bare hostname of `url`, or '' when it does not parse. Plain text, never
 *  HTML: callers must esc() it before putting it in markup. */
export function safeHostname(url: string): string {
  try {
    return new URL(url).hostname
  } catch {
    return ''
  }
}
