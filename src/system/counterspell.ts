// Counterspells. A counter with at least as much EP as the spell stops it
// outright. A smaller counter needs d20 + school + medium (the counter's own,
// in the target spell's medium) against DC 10 + every 10 EP the spell has over
// it, so even a heavily amped spell can be stopped by a modest counter. A
// natural 1 always fails.

/** The roll a counter needs, or null when it succeeds automatically. */
export function counterspellDC(spellEp: number, counterEp: number): number | null {
  const gap = Math.max(0, spellEp) - Math.max(0, counterEp)
  if (gap <= 0) return null
  return 10 + Math.floor(gap / 10)
}

/** Chance (0..1) that d20 + bonus meets the DC; a natural 1 always fails. */
export function counterspellChance(bonus: number, dc: number | null): number {
  if (dc === null) return 1
  let hits = 0
  for (let roll = 2; roll <= 20; roll++) if (roll + bonus >= dc) hits++
  return hits / 20
}
