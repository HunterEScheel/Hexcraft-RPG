/**
 * Tethers and flaws saved before titles existed held everything in the
 * description, often as "Type: details". Split that into a title and
 * description; a short line with no colon becomes the title, and anything
 * longer stays the description.
 */
export function splitLegacyTitle(text: string | undefined): { title: string; description: string } {
  const trimmed = (text ?? '').trim()
  const colon = trimmed.indexOf(':')
  if (colon > 0 && colon <= 40)
    return { title: trimmed.slice(0, colon).trim(), description: trimmed.slice(colon + 1).trim() }
  if (trimmed.length <= 40 && !trimmed.includes('\n')) return { title: trimmed, description: '' }
  return { title: '', description: trimmed }
}
