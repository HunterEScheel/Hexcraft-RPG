// Maneuvers are the martial counterpart to spells: built from the same
// criteria and priced in EP the same way, but drawn from an attack combat skill
// instead of a school and medium. Their bonus is that skill plus its attribute,
// as a weapon attack's is, and a character can save as many per skill as their
// level in it.
import { COMBAT_SKILLS } from './combatSkills'
import type { SpellDraft } from './spells'

export interface SavedManeuver {
  id: string
  name: string
  /** A combat skill id, e.g. 'combat-melee-1h'. */
  skillId: string
  draft: SpellDraft
}

/** The combat skills a maneuver can be drawn from: the attacks. */
export const MANEUVER_SKILLS = COMBAT_SKILLS.filter((s) => s.category === 'action')
