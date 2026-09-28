export const MAGIC_SCHOOLS = [
  'Destroy',
  'Create',
  'Alter',
  'Restore',
  'Divine',
  'Control',
  'Summon',
] as const

export const MAGIC_MEDIUMS = [
  'Elemental',
  'Magic',
  'Kinetic',
  'Cognition',
  'Space',
  'Toxicity',
  'Material',
  'Vitality',
] as const

// Short descriptions for tooltips / help text.
export const MAGIC_MEDIUM_DESCRIPTIONS: Record<
  (typeof MAGIC_MEDIUMS)[number],
  string
> = {
  Elemental: 'Fire, water, earth, lightning, air, sound.',
  Magic: 'Counterspelling and effects that target magic itself.',
  Kinetic: 'Manipulates speed and motion.',
  Cognition:
    'Charm, persuasion, and effects influencing thoughts or actions.',
  Space: 'Teleportation and spatial manipulation.',
  Toxicity: 'Poisons and acids.',
  Material:
    'Targets a specific item or person — alter self, locate object, etc.',
  Vitality: 'Healing, life force, and biological vigor.',
}

export type MagicSchool = (typeof MAGIC_SCHOOLS)[number]

// What each school is called as a discipline, e.g. on the sheet's skill list:
// "Destruction Magic" rather than "Destroy Magic".
export const MAGIC_SCHOOL_DISCIPLINES: Record<MagicSchool, string> = {
  Destroy: 'Destruction',
  Create: 'Creation',
  Alter: 'Alteration',
  Restore: 'Restoration',
  Divine: 'Divination',
  Control: 'Control',
  Summon: 'Summoning',
}
export type MagicMedium = (typeof MAGIC_MEDIUMS)[number]
