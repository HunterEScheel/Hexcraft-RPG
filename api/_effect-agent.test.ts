// Underscore prefix: Vercel doesn't deploy this as a function.
import { describe, expect, it } from 'vitest'
import { parseCatalog, responseSchema, type AgentCatalog } from './effect-agent.ts'

// A cut-down spell catalog, shaped like agentCatalog's (src/system/effectAgent.ts).
const CATALOG: AgentCatalog = {
  kind: 'spell',
  criteria: [
    { key: 'range', label: 'Range', repeatable: false, choices: ['touch / 30 ft', '120 ft'] },
    { key: 'aoe', label: 'AOE / Targets', repeatable: false, choices: ['Targeted: single target', 'Splash: 20 ft splash'] },
    { key: 'buffDebuff', label: 'Buff / Debuff', repeatable: true, choices: ['Tier I (5 EP): slowed'] },
  ],
  castingTimes: [
    { key: 'action', label: 'Action' },
    { key: '2actions', label: '2 Actions' },
  ],
  targetings: [
    { key: 'hit', label: 'Roll to hit' },
    { key: 'dodge', label: 'Dodge save' },
  ],
}

interface Schema {
  required: string[]
  additionalProperties: boolean
  properties: {
    picks: { required: string[]; properties: Record<string, { enum: string[] }> }
    lists: { required: string[]; properties: Record<string, { items: { enum: string[] } }> }
    castingTime: { enum: string[] }
  }
}

describe('effect agent response schema', () => {
  it('accepts a catalog as the browser sends it', () => {
    expect(parseCatalog(JSON.parse(JSON.stringify(CATALOG)))).not.toBeNull()
  })

  it('makes every field required and lists only catalog choices', () => {
    const schema = responseSchema(CATALOG) as unknown as Schema
    expect(schema.additionalProperties).toBe(false)
    expect(schema.required).toEqual(['name', 'picks', 'lists', 'damageDice', 'castingTime', 'targeting', 'notes'])
    expect(schema.properties.picks.required).toEqual(['range', 'aoe'])
    expect(schema.properties.picks.properties.aoe.enum).toEqual(['Targeted: single target', 'Splash: 20 ft splash'])
    expect(schema.properties.lists.required).toEqual(['buffDebuff'])
    expect(schema.properties.lists.properties.buffDebuff.items.enum).toEqual(['Tier I (5 EP): slowed'])
    expect(schema.properties.castingTime.enum).toEqual(['action', '2actions'])
  })

  it('rejects a malformed catalog', () => {
    expect(parseCatalog(null)).toBeNull()
    expect(parseCatalog({ ...CATALOG, kind: 'potion' })).toBeNull()
    expect(parseCatalog({ ...CATALOG, criteria: [{ ...CATALOG.criteria[0], key: 'a b' }] })).toBeNull()
    expect(parseCatalog({ ...CATALOG, criteria: [{ ...CATALOG.criteria[0], choices: [] }] })).toBeNull()
    expect(parseCatalog({ ...CATALOG, targetings: [] })).toBeNull()
  })
})
