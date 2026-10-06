import { expect, test } from 'vitest'
import { endsAfter, howSignedIn, lastDay, qrPath, type Session } from './manage'

test('a Session tells how it signed in', () => {
  const s = (x: Partial<Session>): Session => ({ id: 's', browser: '', method: '', signedIn: '', lastUse: '', current: false, ...x })
  expect(howSignedIn(s({ method: 'passkey', provider: 'Apple Passwords' }))).toBe('with a Passkey from Apple Passwords')
  expect(howSignedIn(s({ method: 'link', by: { id: 'p1', name: 'Alice' } }))).toBe('with a Sign-in link from Alice')
  expect(howSignedIn(s({ method: 'link', by: { id: 'p1' } }))).toBe('with a Sign-in link from a removed Person')
  expect(howSignedIn(s({ method: 'link' }))).toBe('with a Sign-in link')
})

test("a Guest's last day ends as the next one begins, and reads back as that day", () => {
  const ends = endsAfter('2026-03-31')!
  expect(new Date(ends).getTime()).toBe(new Date(2026, 3, 1).getTime())
  expect(lastDay(ends)).toBe('2026-03-31')
  expect(endsAfter('')).toBeNull()
})

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
