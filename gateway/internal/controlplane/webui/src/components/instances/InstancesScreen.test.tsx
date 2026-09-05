import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FilterPolicy, MCPRegistration } from "../../api/types";
import { InstancesScreen } from "./InstancesScreen";

const ENDPOINTS: MCPRegistration[] = [
  {
    name: "mcp-a",
    transport: "stdio",
    connect: { command: "a" },
    status: "active",
    tools: { only_in_a: { name: "only_in_a", output_schema: { type: "object", properties: { a_field: { type: "string" } } } } },
  },
  {
    name: "mcp-b",
    transport: "stdio",
    connect: { command: "b" },
    status: "active",
    tools: { only_in_b: { name: "only_in_b", output_schema: { type: "object", properties: { b_field: { type: "string" } } } } },
  },
];

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("InstancesScreen", () => {
  it("resets the response filter form's state when switching to a different endpoint", async () => {
    const user = userEvent.setup();
    const createdFilters: FilterPolicy[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string, init?: RequestInit) => {
        if (url === "/admin/mcps") return Promise.resolve(jsonResponse(ENDPOINTS));
        if (url === "/admin/filter-policies" && (!init || init.method === undefined)) {
          return Promise.resolve(jsonResponse(createdFilters));
        }
        if (url === "/admin/filter-policies" && init?.method === "POST") {
          const body = JSON.parse(init.body as string) as FilterPolicy;
          createdFilters.push(body);
          return Promise.resolve(jsonResponse(body));
        }
        throw new Error(`unexpected fetch: ${url} ${init?.method}`);
      }),
    );

    render(<InstancesScreen />);

    // mcp-a is auto-selected (first endpoint); toggle its only field off.
    await screen.findByRole("button", { name: /^mcp-a/ });
    await user.click(await screen.findByRole("button", { name: "Hide only_in_a $.a_field" }));
    await user.type(screen.getByPlaceholderText("filter name"), "hide-a");

    // Switching to mcp-b must not carry mcp-a's toggled field or in-progress
    // filter name along -- without a `key` on ResponseFilterGroup this used
    // to leave the field list scoped to a tool ("only_in_a") that doesn't
    // exist on mcp-b, collapsing it to zero fields instead of showing
    // mcp-b's own, and "Create filter" would submit against the wrong mcp.
    await user.click(screen.getByRole("button", { name: /^mcp-b/ }));

    await waitFor(() => expect(screen.getByText("$.b_field")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Hide only_in_b $.b_field" })).toBeInTheDocument();
    expect(screen.getByPlaceholderText("filter name")).toHaveValue("");

    // The full create flow on the newly-selected endpoint must be scoped
    // correctly too, not just the picker's own display.
    await user.type(screen.getByPlaceholderText("filter name"), "hide-b");
    await user.click(screen.getByRole("button", { name: "Hide only_in_b $.b_field" }));
    await user.click(screen.getByRole("button", { name: "Create filter" }));

    await waitFor(() => expect(createdFilters).toHaveLength(1));
    expect(createdFilters[0]).toMatchObject({ name: "hide-b::only_in_b", mcp: "mcp-b", tool: "only_in_b" });
  });
});
