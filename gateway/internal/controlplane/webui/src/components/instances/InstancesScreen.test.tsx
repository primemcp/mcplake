import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MCPRegistration } from "../../api/types";
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
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string) => {
        if (url === "/admin/mcps") return Promise.resolve(jsonResponse(ENDPOINTS));
        if (url === "/admin/filter-policies") return Promise.resolve(jsonResponse([]));
        throw new Error(`unexpected fetch: ${url}`);
      }),
    );

    render(<InstancesScreen />);

    // mcp-a is auto-selected (first endpoint); scope its only field.
    await screen.findByRole("button", { name: /^mcp-a/ });
    await user.click(await screen.findByRole("button", { name: "Hide only_in_a $.a_field" }));
    expect(screen.getByText(/Scoped to/)).toBeInTheDocument();

    // Switching to mcp-b must not carry mcp-a's scoped tool/state along --
    // without a `key` on ResponseFilterGroup this used to leave the field
    // list scoped to a tool ("only_in_a") that doesn't exist on mcp-b,
    // collapsing it to zero fields instead of showing mcp-b's own.
    await user.click(screen.getByRole("button", { name: /^mcp-b/ }));

    await waitFor(() => expect(screen.getByText("$.b_field")).toBeInTheDocument());
    expect(screen.queryByText(/Scoped to/)).not.toBeInTheDocument();
    expect(screen.queryByText("No field matches that.")).not.toBeInTheDocument();
  });
});
