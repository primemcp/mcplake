import { act, render, screen, waitFor } from "@testing-library/react";
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
    enabled: true,
    tools: { only_in_a: { name: "only_in_a", output_schema: { type: "object", properties: { a_field: { type: "string" } } } } },
  },
  {
    name: "mcp-b",
    transport: "stdio",
    connect: { command: "b" },
    status: "active",
    enabled: true,
    tools: { only_in_b: { name: "only_in_b", output_schema: { type: "object", properties: { b_field: { type: "string" } } } } },
  },
];

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
}

function stubEndpointsFetch() {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      if (url === "/admin/mcps") return Promise.resolve(jsonResponse(ENDPOINTS));
      if (url === "/admin/filter-policies") return Promise.resolve(jsonResponse([]));
      throw new Error(`unexpected fetch: ${url}`);
    }),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
  // Several tests below navigate for real via history.pushState; reset so
  // one test's URL can't leak into the next.
  window.history.pushState({}, "", "/");
});

describe("InstancesScreen", () => {
  it("selecting an endpoint restores it after a simulated reload, instead of always falling back to the first one", async () => {
    const user = userEvent.setup();
    stubEndpointsFetch();
    const { unmount } = render(<InstancesScreen />);

    await screen.findByRole("button", { name: /^mcp-a/ });
    await user.click(screen.getByRole("button", { name: /^mcp-b/ }));
    await waitFor(() => expect(screen.getByText("$.b_field")).toBeInTheDocument());

    // A reload remounts the whole tree fresh -- simulate that rather than
    // relying on any in-memory state surviving.
    unmount();
    render(<InstancesScreen />);

    await waitFor(() => expect(screen.getByText("$.b_field")).toBeInTheDocument());
    expect(screen.queryByText("$.a_field")).not.toBeInTheDocument();
  });

  it("falls back to the first endpoint when the persisted selection no longer exists", async () => {
    localStorage.setItem("mcplake:instances:selected", JSON.stringify("mcp-deleted"));
    stubEndpointsFetch();
    render(<InstancesScreen />);

    await waitFor(() => expect(screen.getByText("$.a_field")).toBeInTheDocument());
  });

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

  it("a direct load of /mcps/:name selects that endpoint over whatever localStorage remembers", async () => {
    localStorage.setItem("mcplake:instances:selected", JSON.stringify("mcp-a"));
    window.history.pushState({}, "", "/mcps/mcp-b");
    stubEndpointsFetch();

    render(<InstancesScreen />);

    await waitFor(() => expect(screen.getByText("$.b_field")).toBeInTheDocument());
    expect(screen.queryByText("$.a_field")).not.toBeInTheDocument();
  });

  it("selecting an endpoint pushes /mcps/:name so it's bookmarkable/shareable", async () => {
    const user = userEvent.setup();
    stubEndpointsFetch();
    render(<InstancesScreen />);

    await user.click(await screen.findByRole("button", { name: /^mcp-b/ }));

    await waitFor(() => expect(window.location.pathname).toBe("/mcps/mcp-b"));
  });

  it("auto-selecting the first endpoint on load corrects the URL without adding a history entry", async () => {
    stubEndpointsFetch();
    const pushSpy = vi.spyOn(window.history, "pushState");

    render(<InstancesScreen />);

    await waitFor(() => expect(window.location.pathname).toBe("/mcps/mcp-a"));
    expect(pushSpy).not.toHaveBeenCalled();
  });

  it("browser back/forward between two endpoints re-selects to match the URL", async () => {
    const user = userEvent.setup();
    stubEndpointsFetch();
    render(<InstancesScreen />);

    await screen.findByRole("button", { name: /^mcp-a/ });
    await user.click(screen.getByRole("button", { name: /^mcp-b/ }));
    await waitFor(() => expect(screen.getByText("$.b_field")).toBeInTheDocument());

    act(() => {
      window.history.pushState({}, "", "/mcps/mcp-a");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });

    await waitFor(() => expect(screen.getByText("$.a_field")).toBeInTheDocument());
  });

  it("toggling Enabled end to end calls the real PATCH and the list reflects it after refresh", async () => {
    const user = userEvent.setup();
    let mcpA = ENDPOINTS[0];
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string, init?: RequestInit) => {
        if (url === "/admin/mcps" && (!init || init.method === undefined)) {
          return Promise.resolve(jsonResponse([mcpA, ENDPOINTS[1]]));
        }
        if (url === "/admin/mcps/mcp-a" && init?.method === "PATCH") {
          const body = JSON.parse(init.body as string) as { enabled: boolean };
          mcpA = { ...mcpA, enabled: body.enabled };
          return Promise.resolve(jsonResponse(mcpA));
        }
        if (url === "/admin/filter-policies") return Promise.resolve(jsonResponse([]));
        throw new Error(`unexpected fetch: ${url} ${init?.method}`);
      }),
    );

    render(<InstancesScreen />);

    await user.click(await screen.findByRole("switch", { name: "Disable mcp-a" }));

    await waitFor(() => expect(screen.getByRole("switch", { name: "Enable mcp-a" })).toBeInTheDocument());
    expect(
      screen.getByText("Disabled — requests to this endpoint are rejected and grants are suspended"),
    ).toBeInTheDocument();
  });
});
