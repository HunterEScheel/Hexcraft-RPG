import { describe, expect, it } from 'vitest'
import { agentCatalog, criterionChoices, draftFromAgent } from './effectAgent'
import { maneuverCriteria } from './maneuvers'
import { SPELL_CRITERIA, emptySpellDraft, selectedOption, spellCost } from './spells'

const option = (criteria: typeof SPELL_CRITERIA, key: string, draft: ReturnType<typeof emptySpellDraft>) => {
  const c = criteria.find((x) => x.key === key)!
  return selectedOption(c, draft.selections[c.key])?.label
}

describe('effect agent catalog', () => {
  it('names every spell option once, by mode and option', () => {
    const catalog = agentCatalog(SPELL_CRITERIA, 'spell')
    const aoe = catalog.criteria.find((c) => c.key === 'aoe')!
    expect(aoe.choices).toContain('Splash: 20 ft splash')
    expect(aoe.choices).toContain('Targeted: single target')
    const range = catalog.criteria.find((c) => c.key === 'range')!
    expect(range.choices[0]).toBe('touch / 30 ft')
    for (const c of SPELL_CRITERIA) {
      const total = c.options?.length ?? c.modes!.reduce((n, m) => n + m.options.length, 0)
      const choices = catalog.criteria.find((x) => x.key === c.key)!.choices
      expect(new Set(choices).size).toBe(choices.length)
      // A list criterion leaves out its "none".
      expect(choices.length).toBe(c.key === 'buffDebuff' ? total - 1 : total)
    }
  })

  it('offers only the timings and targetings the kind allows', () => {
    const spell = agentCatalog(SPELL_CRITERIA, 'spell')
    expect(spell.castingTimes.map((t) => t.key)).not.toContain('none')
    expect(spell.targetings.map((t) => t.key)).toContain('countermagic')
    const maneuver = agentCatalog(maneuverCriteria('combat-melee-2h'), 'maneuver')
    expect(maneuver.castingTimes.map((t) => t.key)).toContain('none')
    expect(maneuver.targetings.map((t) => t.key)).not.toContain('countermagic')
    expect(maneuver.criteria.find((c) => c.key === 'selfDebuff')?.repeatable).toBe(true)
  })

  it('maps each choice back to its selection', () => {
    const aoe = SPELL_CRITERIA.find((c) => c.key === 'aoe')!
    expect(criterionChoices(aoe).get('Line: ≤ 300 ft line')).toEqual({ modeIndex: 2, optionIndex: 1 })
  })
})

describe('draftFromAgent', () => {
  it('builds the fireball from the task description', () => {
    const { draft, name, warnings } = draftFromAgent(
      {
        name: 'Fireball',
        picks: {
          range: 'touch / 30 ft',
          aoe: 'Splash: 20 ft splash',
          duration: 'No concentration: instantaneous',
          challenge: 'none',
        },
        lists: { buffDebuff: [] },
        damageDice: 6,
        castingTime: 'action',
        targeting: 'dodge',
        notes: '',
      },
      SPELL_CRITERIA,
      'spell',
    )
    expect(warnings).toEqual([])
    expect(name).toBe('Fireball')
    expect(option(SPELL_CRITERIA, 'aoe', draft)).toBe('20 ft splash')
    expect(draft.damageDice).toBe(6)
    expect(draft.targeting).toBe('dodge')
    // 20 ft splash 5 + 6 dice 12 = 17, × 2 for one action.
    expect(spellCost(draft).totalEp).toBe(34)
  })

  it('puts the first effect in the selection and the rest in extras', () => {
    const { draft } = draftFromAgent(
      {
        picks: {},
        lists: { buffDebuff: ['Tier II (10 EP): frightened', 'Tier I (5 EP): slowed', 'Tier II (10 EP): frightened'] },
        damageDice: 0,
        castingTime: '2actions',
        targeting: 'resolve',
      },
      SPELL_CRITERIA,
      'spell',
    )
    expect(option(SPELL_CRITERIA, 'buffDebuff', draft)).toBe('frightened')
    // The repeat is dropped.
    expect(draft.extraEffects).toHaveLength(1)
    expect(spellCost(draft).baseEp).toBe(15)
  })

  it('leaves invalid picks at their defaults and says so', () => {
    const { draft, warnings } = draftFromAgent(
      {
        picks: { aoe: 'Splash: 25 ft splash', range: 42 },
        lists: { buffDebuff: ['Tier IX: godhood'] },
        damageDice: -3,
        castingTime: 'none',
        targeting: 'parry',
      },
      SPELL_CRITERIA,
      'spell',
    )
    expect(draft).toEqual(emptySpellDraft())
    expect(warnings.join('\n')).toMatch(/25 ft splash/)
    expect(warnings.join('\n')).toMatch(/godhood/)
    expect(warnings.join('\n')).toMatch(/Range/)
    // A spell can't take no action.
    expect(warnings.join('\n')).toMatch(/Timing "none"/)
  })

  it('rounds damage dice and survives a non-object answer', () => {
    expect(draftFromAgent({ damageDice: 4.6 }, SPELL_CRITERIA, 'spell').draft.damageDice).toBe(5)
    const bad = draftFromAgent('nope', SPELL_CRITERIA, 'spell')
    expect(bad.draft).toEqual(emptySpellDraft())
    expect(bad.warnings).toHaveLength(1)
  })

  it('applies the concentration rule to a lasting damage spell', () => {
    const { draft, warnings } = draftFromAgent(
      { picks: { duration: 'No concentration: 1 minute' }, lists: {}, damageDice: 2 },
      SPELL_CRITERIA,
      'spell',
    )
    const duration = SPELL_CRITERIA.find((c) => c.key === 'duration')!
    expect(duration.modes![draft.selections.duration.modeIndex].key).toBe('concentration')
    expect(option(SPELL_CRITERIA, 'duration', draft)).toBe('1 minute')
    expect(warnings).toContain('Duration moved to the list the concentration rule allows.')
  })

  it('builds a maneuver with self-debuffs and a no-action timing', () => {
    const criteria = maneuverCriteria('combat-melee-2h')
    const { draft, warnings } = draftFromAgent(
      {
        picks: { aoe: 'Splash: 10 ft splash', duration: '1 round', selfDuration: '2 rounds' },
        lists: {
          buffDebuff: ['Tier II (10 EP): prone'],
          selfDebuff: ['Tier I (−5 EP): off-balance', 'Tier I (−5 EP): exposed'],
        },
        damageDice: 2,
        castingTime: 'none',
        targeting: 'grit',
      },
      criteria,
      'maneuver',
    )
    expect(warnings).toEqual([])
    expect(draft.castingTime).toBe('none')
    expect(draft.extraSelfDebuffs).toHaveLength(1)
    // splash 10 + prone 10 + dice 4 − refunds 10 = 14, × 8.
    expect(spellCost(draft, criteria).totalEp).toBe(112)
  })

  it('flags a point-blank shot without inevitable', () => {
    const criteria = maneuverCriteria('combat-fired-1h')
    const { warnings } = draftFromAgent(
      { picks: { range: 'point-blank (≤ 5 ft)' }, lists: {}, damageDice: 3, castingTime: 'action' },
      criteria,
      'maneuver',
    )
    expect(warnings.join('\n')).toMatch(/inevitable/)
  })
})
