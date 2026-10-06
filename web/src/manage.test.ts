import { expect, test } from 'vitest'
import { qrPath } from './manage'

test("a link's QR code is drawn as a path of its dark modules", () => {
  const { size, d } = qrPath('https://oiko.example/sign-in#ABCDEFGHIJKLMNOPQRSTUVWXYZ234567')
  expect(size).toBeGreaterThanOrEqual(21) // version 1 or more
  // The finder patterns' corners are dark, the quiet module beside one is not.
  for (const [col, row] of [
    [0, 0],
    [size - 1, 0],
    [0, size - 1],
  ])
    expect(d).toContain(`M${col} ${row}h1v1h-1z`)
  expect(d).not.toContain(`M7 0h1v1h-1z`)
})
