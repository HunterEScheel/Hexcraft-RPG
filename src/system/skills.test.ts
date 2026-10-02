import { describe, expect, it } from 'vitest'
import { emptyCharacter, ensureCombatSkills, skillLabel, splitSkillName } from './character'

describe('skill specificity', () => {
  it('splits "Name (Specificity)" and labels it back', () => {
    expect(splitSkillName('Animal Handling (Dogs)')).toEqual({ name: 'Animal Handling', specificity: 'Dogs' })
    expect(splitSkillName('  Lockpicking ')).toEqual({ name: 'Lockpicking' })
    expect(skillLabel({ name: 'Animal Handling', specificity: 'Dogs' })).toBe('Animal Handling (Dogs)')
    expect(skillLabel({ name: 'Lockpicking', specificity: '  ' })).toBe('Lockpicking')
  })

  it("splits older characters' typed specialties once, leaving combat skills alone", () => {
    const base = emptyCharacter('Peasants', 150)
    const old = {
      ...base,
      rulesVersion: 3,
      skills: [...base.skills, { id: 'custom-1', name: 'Driving (Motorcycle)', level: 2 }],
    }
    const once = ensureCombatSkills(old)
    const driving = once.skills.find((s) => s.id === 'custom-1')!
    expect(driving).toMatchObject({ name: 'Driving', specificity: 'Motorcycle', level: 2 })
    expect(once.skills.find((s) => s.id === 'combat-melee-1h')?.name).toBe('1-handed melee')
    expect(ensureCombatSkills(once).skills.find((s) => s.id === 'custom-1')).toEqual(driving)
  })
})
