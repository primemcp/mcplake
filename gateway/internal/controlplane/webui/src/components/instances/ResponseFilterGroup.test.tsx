import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { FilterPolicy, MCPRegistration } from "../../api/types";
import { ResponseFilterGroup } from "./ResponseFilterGroup";

const GET_USER_SCHEMA = {
  type: "object",
  properties: {
    name: { type: "string" },
    salary: { type: "number" },
  },
};

const ENDPOINT: MCPRegistration = {
  name: "postgres-ro",
  transport: "stdio",
  connect: { command: "pg-ro" },
  status: "active",
  tools: {
    get_user: { name: "get_user", output_schema: GET_USER_SCHEMA },
    list_users: { name: "list_users" },
  },
};

const FILTERS: FilterPolicy[] = [
  { name: "hide-pii", match: [], mcp: "postgres-ro", tool: "get_user", drop_fields: ["$.salary"] },
  // A filter belonging to a different endpoint must not show up here.
  { name: "other", match: [], mcp: "postgres-rw", tool: "get_user", drop_fields: ["$.x"] },
];

describe("ResponseFilterGroup", () => {
  it("lists only the filters scoped to this endpoint", () => {
    render(
      <ResponseFilterGroup
        endpoint={ENDPOINT}
        filters={FILTERS}
        loading={false}
        error={null}
        onRetry={vi.fn()}
        onCreate={vi.fn()}
        onUpdate={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    expect(screen.getByText("hide-pii")).toBeInTheDocument();
    expect(screen.queryByText("other")).not.toBeInTheDocument();
  });

  it("creates a filter from schema fields toggled on in the picker", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(
      <ResponseFilterGroup
        endpoint={ENDPOINT}
        filters={[]}
        loading={false}
        error={null}
        onRetry={vi.fn()}
        onCreate={onCreate}
        onUpdate={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Add response filter/ }));
    await user.type(screen.getByPlaceholderText("filter name"), "hide-pii");
    await user.selectOptions(screen.getByRole("combobox"), "get_user");
    await user.click(screen.getByRole("button", { name: "Hide $.salary" }));
    await user.click(screen.getByRole("button", { name: "Create filter" }));

    expect(onCreate).toHaveBeenCalledWith("hide-pii", "get_user", ["$.salary"]);
  });

  it("edits an existing filter's fields and saves via onUpdate", async () => {
    const user = userEvent.setup();
    const onUpdate = vi.fn().mockResolvedValue(undefined);
    render(
      <ResponseFilterGroup
        endpoint={ENDPOINT}
        filters={FILTERS}
        loading={false}
        error={null}
        onRetry={vi.fn()}
        onCreate={vi.fn()}
        onUpdate={onUpdate}
        onDelete={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Edit" }));
    // hide-pii starts with $.salary toggled on; also toggle $.name on.
    await user.click(screen.getByRole("button", { name: "Hide $.name" }));
    await user.click(screen.getByRole("button", { name: "Save filter" }));

    expect(onUpdate).toHaveBeenCalledWith("hide-pii", "get_user", ["$.salary", "$.name"]);
  });

  it("deletes a filter from within its edit panel", async () => {
    const user = userEvent.setup();
    const onDelete = vi.fn().mockResolvedValue(undefined);
    render(
      <ResponseFilterGroup
        endpoint={ENDPOINT}
        filters={FILTERS}
        loading={false}
        error={null}
        onRetry={vi.fn()}
        onCreate={vi.fn()}
        onUpdate={vi.fn()}
        onDelete={onDelete}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Edit" }));
    await user.click(screen.getByRole("button", { name: "Delete" }));
    expect(onDelete).toHaveBeenCalledWith("hide-pii");
  });

  it("the list search narrows filters by name, tool, or dropped field — separate from the per-form field picker search", async () => {
    const user = userEvent.setup();
    const twoFilters: FilterPolicy[] = [
      { name: "hide-pii", match: [], mcp: "postgres-ro", tool: "get_user", drop_fields: ["$.salary"] },
      { name: "hide-email", match: [], mcp: "postgres-ro", tool: "list_users", drop_fields: ["$.email"] },
    ];
    render(
      <ResponseFilterGroup
        endpoint={ENDPOINT}
        filters={twoFilters}
        loading={false}
        error={null}
        onRetry={vi.fn()}
        onCreate={vi.fn()}
        onUpdate={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    expect(screen.getByText("hide-pii")).toBeInTheDocument();
    expect(screen.getByText("hide-email")).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText("Search filters or fields"), "salary");

    expect(screen.getByText("hide-pii")).toBeInTheDocument();
    expect(screen.queryByText("hide-email")).not.toBeInTheDocument();

    await user.clear(screen.getByPlaceholderText("Search filters or fields"));
    await user.type(screen.getByPlaceholderText("Search filters or fields"), "nope");
    expect(screen.getByText("No filter matches that.")).toBeInTheDocument();
  });

  it("disables adding a filter when the endpoint has no discovered tools", () => {
    render(
      <ResponseFilterGroup
        endpoint={{ ...ENDPOINT, tools: undefined }}
        filters={[]}
        loading={false}
        error={null}
        onRetry={vi.fn()}
        onCreate={vi.fn()}
        onUpdate={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: /No tools discovered yet/ })).toBeDisabled();
  });
});
