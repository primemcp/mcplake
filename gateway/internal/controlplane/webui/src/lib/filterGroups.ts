import type { FilterPolicy } from "../api/types";

/**
 * The real backend's FilterPolicy is one tool per record -- there's no way
 * to change that without touching filter.Strip()'s matching logic. A
 * "filter across N tools" is therefore a purely frontend concept: N real
 * records sharing a `<group>::<tool>` name prefix, rendered as one card.
 * Legacy/manually-created records with no `::` in their name are their
 * own single-member group (id = the whole name) -- no migration needed.
 */
export const GROUP_SEP = "::";

export function groupIdOf(filterName: string): string {
  const i = filterName.indexOf(GROUP_SEP);
  return i === -1 ? filterName : filterName.slice(0, i);
}

/**
 * Strips a `<prefix>::` segment a caller already confirmed via
 * `groupIdOf(name) === prefix` -- e.g. the Users & access screen strips a
 * filter's `<user>::` segment before re-grouping the remainder by its own
 * next segment, so one user can have several independently-named filters
 * per endpoint (mirroring this same convention one level down). Returns
 * `name` unchanged if it doesn't actually start with that prefix.
 */
export function stripGroupPrefix(name: string, prefix: string): string {
  const full = prefix + GROUP_SEP;
  return name.startsWith(full) ? name.slice(full.length) : name;
}

// A single qualifier ("::<tool>") is enough when the group is already
// scoped to one mcp (the MCP-connections response-filter case below). The
// Users & access screen reuses this same convention but needs a second
// qualifier ("::<mcp>::<tool>") -- one user can be granted access across
// several endpoints, so the tool name alone isn't enough to keep two
// FilterPolicy records apart when two different mcps happen to expose a
// tool with the same name.
export function memberName(groupId: string, ...qualifiers: string[]): string {
  return [groupId, ...qualifiers].join(GROUP_SEP);
}

export type FilterGroup = { id: string; members: FilterPolicy[] };

export function groupFilters(filters: FilterPolicy[]): FilterGroup[] {
  const order: string[] = [];
  const byId = new Map<string, FilterPolicy[]>();
  for (const f of filters) {
    const id = groupIdOf(f.name);
    const existing = byId.get(id);
    if (existing) {
      existing.push(f);
    } else {
      byId.set(id, [f]);
      order.push(id);
    }
  }
  return order.map((id) => ({ id, members: byId.get(id)! }));
}

/** tool -> the field paths to drop for that tool; a tool absent here is no
 * longer part of the group at all. */
export type FieldsByTool = Record<string, string[]>;

export type GroupSaveItem = { name: string; tool: string; dropFields: string[] };
export type GroupPlan = { toCreate: GroupSaveItem[]; toUpdate: GroupSaveItem[]; toDelete: string[] };

/**
 * Diffs a group's real, currently-persisted members against the desired
 * per-tool field selection, so saving (whether creating a brand new group
 * -- `existing: []` -- or editing one) only ever issues the real API
 * calls a change actually needs: existing members keep their exact name
 * (no rename, PUT), new tools mint a `<group>::<tool>` name (POST), and
 * tools dropped from the selection entirely get deleted (DELETE).
 */
export function planGroupSave(groupId: string, existing: FilterPolicy[], desired: FieldsByTool): GroupPlan {
  const byTool = new Map(existing.map((m) => [m.tool, m]));
  const toCreate: GroupSaveItem[] = [];
  const toUpdate: GroupSaveItem[] = [];

  for (const [tool, dropFields] of Object.entries(desired)) {
    const current = byTool.get(tool);
    if (current) {
      toUpdate.push({ name: current.name, tool, dropFields });
      byTool.delete(tool);
    } else {
      toCreate.push({ name: memberName(groupId, tool), tool, dropFields });
    }
  }

  const toDelete = [...byTool.values()].map((m) => m.name);

  return { toCreate, toUpdate, toDelete };
}
