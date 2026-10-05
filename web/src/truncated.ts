import type { MouseEvent } from 'react'

// titleIfTruncated, as the onMouseEnter of a `truncate` element, shows its
// full text as a tooltip only when the ellipsis hides part of it.
export function titleIfTruncated(e: MouseEvent<HTMLElement>) {
  const el = e.currentTarget
  if (el.scrollWidth > el.clientWidth) el.title = el.textContent ?? ''
  else el.removeAttribute('title')
}
