import { describe, expect, it } from 'vitest'
import { migrateTether } from './tethers'

describe('migrateTether', () => {
  it('splits "Type: details" into a title and description', () => {
    expect(migrateTether({ id: 't', tier: 2, description: 'Sworn oath: protect the prince' })).toMatchObject({
      title: 'Sworn oath',
      description: 'protect the prince',
    })
  })
  it('makes a short line the title', () => {
    expect(migrateTether({ id: 't', tier: 1, description: 'Debt to the guild' })).toMatchObject({
      title: 'Debt to the guild',
      description: '',
    })
  })
  it('keeps a long line as the description', () => {
    const long = 'I owe the guild master everything after she pulled me from the river'
    expect(migrateTether({ id: 't', tier: 1, description: long })).toMatchObject({ title: '', description: long })
  })
  it('leaves titled tethers alone', () => {
    const t = { id: 't', tier: 3 as const, title: 'Geas', description: 'Never refuse a guest' }
    expect(migrateTether(t)).toBe(t)
  })
})
