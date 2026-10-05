// Each tier's typical obligations: things the world will hold the character
// to, not things they merely want (those are flaws).
export const TETHER_TIERS = [
  {
    tier: 1,
    label: 'Minor',
    bpRefund: 5,
    examples: ['Defense of another', 'Debt', 'Financial support'],
  },
  {
    tier: 2,
    label: 'Major',
    bpRefund: 15,
    examples: ['Sworn oath', 'Bounty on head', 'Life debt', 'Territorial claim'],
  },
  {
    tier: 3,
    label: 'Binding',
    bpRefund: 40,
    examples: ['Blood oath', "God's orders", 'Geas', 'Hostage'],
  },
] as const

export type TetherTier = (typeof TETHER_TIERS)[number]['tier']

export const TETHER_BP_BY_TIER: Record<TetherTier, number> = {
  1: 5,
  2: 15,
  3: 40,
}

export interface Tether {
  id: string
  /** Short name, shown in the sheet header: "Sworn oath to Prince Aldric". */
  title: string
  /** The full story, shown on the Description tab. */
  description: string
  tier: TetherTier
}

/**
 * Tethers saved before titles existed held everything in the description,
 * usually as "Type: details" (the example chips start one that way). Split that
 * into a title and description; a short line with no colon becomes the title.
 */
export function migrateTether(t: Omit<Tether, 'title'> & { title?: string }): Tether {
  if (t.title !== undefined) return t as Tether
  const text = (t.description ?? '').trim()
  const colon = text.indexOf(':')
  if (colon > 0 && colon <= 40)
    return { ...t, title: text.slice(0, colon).trim(), description: text.slice(colon + 1).trim() }
  if (text.length <= 40 && !text.includes('\n')) return { ...t, title: text, description: '' }
  return { ...t, title: '', description: text }
}

export function tetherRefundTotal(tethers: Tether[]): number {
  return tethers.reduce((sum, t) => sum + TETHER_BP_BY_TIER[t.tier], 0)
}

// Obligation weight = sum of tiers. Tier I=1, II=2, III=3.
// DM sets a minimum threshold the character must meet.
export function tetherObligationWeight(tethers: Tether[]): number {
  return tethers.reduce((sum, t) => sum + t.tier, 0)
}
