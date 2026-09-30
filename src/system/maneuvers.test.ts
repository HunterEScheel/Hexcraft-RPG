import { describe, expect, it } from 'vitest'
import { emptyCharacter, ensureCombatSkills, maneuverBonus } from './character'
import { MANEUVER_CRITERIA, MANEUVER_SKILLS } from './maneuvers'
import { emptySpellDraft, savedSpellCost, selectedOption, spellCost } from './spells'
import { conditionDescription } from './conditions'

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

  it('refund EP for a self-debuff, and more the longer it lasts', () => {
    // Reckless Strike: advantage on the attack and +4d6, leaving you rooted.
    const draft = emptySpellDraft()
    draft.selections.buffDebuff = { modeIndex: 1, optionIndex: 0 } // advantage, 5 EP
    draft.damageDice = 4 // 8 EP
    draft.selections.selfDebuff = { modeIndex: 1, optionIndex: 0 } // rooted, −5 EP
    expect(spellCost(draft, MANEUVER_CRITERIA).totalEp).toBe(8) // 1 round: no extra refund
    draft.selections.selfDuration = { modeIndex: 0, optionIndex: 1 } // 1 minute, −4 EP
    expect(spellCost(draft, MANEUVER_CRITERIA).totalEp).toBe(4)
  })

  it('ignore a self-debuff duration without a self-debuff, and never go below 0', () => {
    const draft = emptySpellDraft()
    draft.damageDice = 1 // 2 EP
    draft.selections.selfDuration = { modeIndex: 0, optionIndex: 3 } // until a long rest
    expect(spellCost(draft, MANEUVER_CRITERIA).totalEp).toBe(2)
    draft.selections.selfDebuff = { modeIndex: 4, optionIndex: 0 } // debilitated, −20 EP
    expect(spellCost(draft, MANEUVER_CRITERIA).totalEp).toBe(0)
  })

  it('leave spells untouched by the self-debuff picks', () => {
    const draft = emptySpellDraft()
    draft.damageDice = 3
    draft.selections.selfDebuff = { modeIndex: 4, optionIndex: 0 }
    expect(spellCost(draft).totalEp).toBe(6)
  })

  it('describe every self-debuff', () => {
    const self = MANEUVER_CRITERIA.find((c) => c.key === 'selfDebuff')!
    const labels = self.modes!.flatMap((m) => m.options.map((o) => o.label)).filter((l) => l !== 'none')
    const undefinedYet = labels.filter((l) => !conditionDescription(l))
    expect(undefinedYet).toEqual([])
    expect(selectedOption(self, { modeIndex: 1, optionIndex: 1 })?.label).toBe('exposed')
  })

  it('fold incapacitated into debilitated on older characters, once', () => {
    const pick = (modeIndex: number, optionIndex: number) => {
      const d = emptySpellDraft()
      d.selections.buffDebuff = { modeIndex, optionIndex }
      return d
    }
    const old = {
      ...emptyCharacter('Peasants', 150),
      rulesVersion: undefined,
      savedSpells: [
        { id: 'a', name: 'Incapacitate', school: 'Control', medium: 'Cognition', draft: pick(4, 2) },
        { id: 'b', name: 'Banish', school: 'Control', medium: 'Space', draft: pick(4, 3) },
        { id: 'c', name: 'Paralyze', school: 'Control', medium: 'Vitality', draft: pick(4, 1) },
      ],
      savedManeuvers: [{ id: 'd', name: 'Knockout', skillId: 'combat-unarmed', draft: pick(4, 1) }],
    }
    const once = ensureCombatSkills(old)
    expect(once.savedSpells.map((s) => s.draft.selections.buffDebuff.optionIndex)).toEqual([0, 2, 1])
    expect(once.savedManeuvers[0].draft.selections.buffDebuff.optionIndex).toBe(0)
    const twice = ensureCombatSkills(once)
    expect(twice.savedSpells.map((s) => s.draft.selections.buffDebuff.optionIndex)).toEqual([0, 2, 1])
  })
})
