import { describe, expect, it } from 'vitest'
import { emptyCharacter, ensureCombatSkills, maneuverBonus } from './character'
import { MANEUVER_CRITERIA, MANEUVER_SKILLS } from './maneuvers'
import { emptySpellDraft, savedSpellCost, spellCost } from './spells'

describe('maneuvers', () => {
  it('are drawn only from attack skills', () => {
    expect(MANEUVER_SKILLS.map((s) => s.id)).not.toContain('combat-dodge')
    expect(MANEUVER_SKILLS.map((s) => s.id)).toContain('combat-melee-1h')
  })

  it('add the combat skill to its attribute for the bonus', () => {
    const c = ensureCombatSkills(emptyCharacter('New Adventurers', 400))
    c.attributes.Agility = 2
    c.attributes.Power = 1
    c.skills = c.skills.map((s) =>
      s.id === 'combat-melee-1h' ? { ...s, level: 3 } : s.id === 'combat-tackle' ? { ...s, level: 1 } : s,
    )
    expect(maneuverBonus(c, 'combat-melee-1h')).toBe(5) // 3 + Agility 2
    expect(maneuverBonus(c, 'combat-tackle')).toBe(2) // 1 + Power 1
  })

  it('default to none on older characters', () => {
    const legacy = { ...emptyCharacter('Peasants', 150), savedManeuvers: undefined }
    // @ts-expect-error older saved characters have no savedManeuvers
    expect(ensureCombatSkills(legacy).savedManeuvers).toEqual([])
  })

  it('price only their own criteria', () => {
    // Hamstring: slowed for 1 minute, +1d6, two actions.
    const draft = emptySpellDraft()
    draft.selections.buffDebuff = { modeIndex: 1, optionIndex: 5 } // slowed, Tier I
    draft.selections.duration = { modeIndex: 0, optionIndex: 2 } // 1 minute, tier 2
    draft.selections.challenge = { modeIndex: 0, optionIndex: 6 } // not a maneuver criterion
    draft.damageDice = 1
    expect(spellCost(draft, MANEUVER_CRITERIA).totalEp).toBe(5 + 8 + 2)
    expect(savedSpellCost(draft, MANEUVER_CRITERIA).totalEp).toBe(11)
  })

  it('have no concentration or Challenge', () => {
    expect(MANEUVER_CRITERIA.map((c) => c.key)).not.toContain('challenge')
    const duration = MANEUVER_CRITERIA.find((c) => c.key === 'duration')!
    expect(duration.modes).toBeUndefined()
    expect(duration.options?.map((o) => o.label)).toEqual(['instantaneous', '1 round', '1 minute'])
  })
})
