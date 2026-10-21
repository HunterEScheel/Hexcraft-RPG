export const ATTRIBUTES = [
  'Power',
  'Agility',
  'Intelligence',
  'Sense',
  'Influence',
] as const

export type AttributeName = (typeof ATTRIBUTES)[number]

/** The attributes a character may add to the EP they recover on a long rest. */
export const REST_ATTRIBUTES = ['Intelligence', 'Sense', 'Influence'] as const
export type RestAttribute = (typeof REST_ATTRIBUTES)[number]
