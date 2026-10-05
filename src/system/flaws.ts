import { splitLegacyTitle } from './titled'

export const FLAW_SEVERITIES = [
  { key: 'quirk', label: 'Quirk', bpRefund: 5 },
  { key: 'flaw', label: 'Flaw', bpRefund: 15 },
  { key: 'vice', label: 'Vice', bpRefund: 40 },
] as const

export type FlawSeverity = (typeof FLAW_SEVERITIES)[number]['key']

export const FLAW_BP_BY_SEVERITY: Record<FlawSeverity, number> = {
  quirk: 5,
  flaw: 15,
  vice: 40,
}

export interface Flaw {
  id: string
  /** Short name, shown in the sheet header: "Compulsive gambler". */
  title: string
  /** The full story, shown on the Description tab. */
  description: string
  severity: FlawSeverity
}

/** Flaws saved before titles existed get one split out of the description. */
export function migrateFlaw(f: Omit<Flaw, 'title'> & { title?: string }): Flaw {
  return f.title !== undefined ? (f as Flaw) : { ...f, ...splitLegacyTitle(f.description) }
}

export function flawRefundTotal(flaws: Flaw[]): number {
  return flaws.reduce((sum, f) => sum + FLAW_BP_BY_SEVERITY[f.severity], 0)
}
