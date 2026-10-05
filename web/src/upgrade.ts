// How to apply the newer Releases on this Oiko's Install: a sentence and what to paste. Oiko and each
// added type go to their newest Release that cannot break them when it is newer, otherwise stay at
// the version built in; a Release that may break them is never applied this way (ADR 0019).

import type { Build, ReleaseStatus } from './types'

export const oiko = 'github.com/llehouerou/oiko'

export interface Upgrade {
  text: string
  code: string
}

// null when no Release that cannot break what is built in is newer than it.
export function upgrade(build: Build, releases: ReleaseStatus[]): Upgrade | null {
  if (!releases.some((s) => s.current && s.newer)) return null
  // A plain binary rebuilds itself, with the same rule.
  if (build.install === 'binary') return { text: "Run this with Oiko's executable on its host, then restart Oiko.", code: 'oiko upgrade' }
  const version = (module: string | undefined, current: string | undefined) => {
    const s = releases.find((r) => r.module === module)
    return (s?.newer && s.newest) || current || s?.newest || '<version>'
  }
  const v = version(oiko, build.version)
  // A rebuild takes every added type, each package once, newer or not.
  const added = [...new Map(build.types.filter((t) => !t.builtIn).map((t) => [t.package, version(t.module, t.version)]))].sort(([a], [b]) => a.localeCompare(b))
  if (build.install === 'nixos') {
    const override = [
      '',
      'services.oiko.package = oiko.packages.${system}.default.override {',
      ...added.map(([p, pv]) => `  bridges."${p}" = "${pv}";`),
      '  vendorHash = lib.fakeHash; # the first build prints the hash to set',
      '};',
    ]
    return {
      text: 'Pin the oiko flake input to the Release in your NixOS configuration, then run nix flake update oiko and rebuild.',
      code: [`inputs.oiko.url = "github:llehouerou/oiko/${v}";`, ...(added.length ? override : [])].join('\n'),
    }
  }
  const code = [`go run ${oiko}/cmd/oiko-build@${v} \\`, ...added.map(([p, pv]) => `  -with ${p}@${pv} \\`), '  -o oiko'].join('\n')
  return { text: 'Replace the oiko-build command of your Dockerfile with this one, then rebuild the image and recreate the container.', code }
}
