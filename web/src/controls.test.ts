import { expect, test } from 'vitest'
import { toHex, toHS } from './controls'

test('a colour goes to hex and back', () => {
  expect(toHex({ hue: 0, saturation: 100 })).toBe('#ff0000')
  expect(toHex({ hue: 120, saturation: 100 })).toBe('#00ff00')
  expect(toHex({ hue: 0, saturation: 0 })).toBe('#ffffff')
  expect(toHS('#0000ff')).toEqual({ hue: 240, saturation: 100 })
  expect(toHS('#800000')).toEqual({ hue: 0, saturation: 100 }) // darkness has no say
  for (const hue of [0, 37, 200, 359])
    for (const saturation of [0, 20, 80, 100]) {
      const back = toHS(toHex({ hue, saturation }))
      expect(Math.abs(back.saturation - saturation)).toBeLessThanOrEqual(1)
      if (saturation >= 20) expect(Math.min(Math.abs(back.hue - hue), 360 - Math.abs(back.hue - hue))).toBeLessThanOrEqual(2)
    }
})
