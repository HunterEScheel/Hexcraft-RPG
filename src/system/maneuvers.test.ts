import { describe, expect, it } from 'vitest'
import { emptyCharacter, ensureCombatSkills, maneuverBonus } from './character'
import { MANEUVER_SKILLS } from './maneuvers'

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
})
