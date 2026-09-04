import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { FilterPolicy, MCPRegistration } from "../../api/types";
import { ResponseFilterGroup } from "./ResponseFilterGroup";

const ENDPOINT: MCPRegistration = {
  name: "postgres-ro",
  transport: "stdio",
  connect: { command: "pg-ro" },
  status: "active",
  tools: {
    get_user: { name: "get_user" },
    list_users: { name: "list_users" },
  },
};

const FILTERS: FilterPolicy[] = [
  {
    name: "hide-pii",
    match: [],
    mcp: "postgres-ro",
    tool: "get_user",
    drop_fields: ["$.salary", "$.ssn"],
  },
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
        onDelete={vi.fn()}
      />,
    );

    expect(screen.getByText("hide-pii")).toBeInTheDocument();
    expect(screen.queryByText("other")).not.toBeInTheDocument();
  });

  it("creates a filter with comma-separated fields split into an array", async () => {
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
        onDelete={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Add response filter/ }));
    await user.type(screen.getByPlaceholderText("filter name"), "hide-pii");
    await user.selectOptions(screen.getByRole("combobox"), "get_user");
    await user.type(
      screen.getByPlaceholderText(/fields to drop/),
      "$.salary, $.ssn",
    );
    await user.click(screen.getByRole("button", { name: "Create filter" }));

    expect(onCreate).toHaveBeenCalledWith("hide-pii", "get_user", ["$.salary", "$.ssn"]);
  });

  it("deletes a filter by name", async () => {
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
        onDelete={onDelete}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Delete" }));
    expect(onDelete).toHaveBeenCalledWith("hide-pii");
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
        onDelete={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: /No tools discovered yet/ })).toBeDisabled();
  });
});
