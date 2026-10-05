import { expect, test } from 'vitest'
import type { Build, ReleaseStatus } from './types'
import { oiko, upgrade } from './upgrade'

const hue = 'example.com/oiko-hue'
const build = (install: Build['install'], added = true): Build => ({
  version: 'v0.2.0',
  install,
  bridges: {},
  types: [
    { type: 'homekit', builtIn: true, package: `${oiko}/internal/homekit`, module: oiko, version: 'v0.2.0' },
    ...(added ? [{ type: 'hue', builtIn: false, package: hue, module: hue, version: 'v1.2.0' }] : []),
  ],
})
// Oiko has a newer Release; the hue type is up to date.
const releases: ReleaseStatus[] = [
  { module: oiko, current: 'v0.2.0', newest: 'v0.3.0', newer: true },
  { module: hue, current: 'v1.2.0', newest: 'v1.2.0', newer: false },
]

const command = ['go run github.com/llehouerou/oiko/cmd/oiko-build@v0.3.0 \\', '  -with example.com/oiko-hue@v1.2.0 \\', '  -o oiko'].join('\n')

test('binary: oiko upgrade', () => {
  expect(upgrade(build('binary'), releases)).toEqual({ text: expect.stringContaining('restart Oiko'), code: 'oiko upgrade' })
})

test('docker: the same command, for the Dockerfile', () => {
  expect(upgrade(build('docker'), releases)).toEqual({ text: expect.stringContaining('Dockerfile'), code: command })
})

test('nixos: the flake input pinned to the Release, the override with each added type', () => {
  expect(upgrade(build('nixos'), releases)).toEqual({
    text: expect.stringContaining('nix flake update oiko'),
    code: [
      'inputs.oiko.url = "github:llehouerou/oiko/v0.3.0";',
      '',
      'services.oiko.package = oiko.packages.${system}.default.override {',
      '  bridges."example.com/oiko-hue" = "v1.2.0";',
      '  vendorHash = lib.fakeHash; # the first build prints the hash to set',
      '};',
    ].join('\n'),
  })
})

test('nixos without added types: only the flake input', () => {
  expect(upgrade(build('nixos', false), releases)?.code).toBe('inputs.oiko.url = "github:llehouerou/oiko/v0.3.0";')
})

test('only a Release that may break: no instruction', () => {
  expect(upgrade(build('binary'), [{ module: oiko, current: 'v0.2.0', newest: 'v0.2.0', breaking: 'v0.3.0', newer: false }])).toBeNull()
})

test('a version built in unknown: the newest Release, else a placeholder', () => {
  const b = build('docker', false)
  const other = 'example.com/oiko-other'
  b.types.push({ type: 'hue', builtIn: false, package: hue, module: hue }, { type: 'other', builtIn: false, package: other, module: other })
  expect(upgrade(b, releases)?.code).toBe(
    [
      'go run github.com/llehouerou/oiko/cmd/oiko-build@v0.3.0 \\',
      '  -with example.com/oiko-hue@v1.2.0 \\',
      '  -with example.com/oiko-other@<version> \\',
      '  -o oiko',
    ].join('\n'),
  )
})
