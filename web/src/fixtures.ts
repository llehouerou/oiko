import type { Capability, Role, RoleMap } from './types'

// A Capability in a test, with the Role Oiko would give it (ADR 0056), if any.
export type Given = Capability & { role?: Role }

// announced is what holds caps as Oiko announces it: their Capabilities, and their Roles apart.
// Each keeps its role, so a test may compare it with what it gave.
export function announced(...caps: Given[]): { capabilities: Capability[]; roles: RoleMap } {
  return {
    capabilities: caps,
    roles: Object.fromEntries(caps.flatMap((c) => (c.role ? [[c.key, c.role]] : []))),
  }
}
