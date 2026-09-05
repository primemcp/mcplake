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

const LIST_USERS_SCHEMA = {
  type: "object",
  properties: {
    count: { type: "number" },
  },
};

const ENDPOINT: MCPRegistration = {
  name: "postgres-ro",
  transport: "stdio",
  connect: { command: "pg-ro" },
  status: "active",
  tools: {
    get_user: { name: "get_user", output_schema: GET_USER_SCHEMA },
    list_users: { name: "list_users", output_schema: LIST_USERS_SCHEMA },
  },
};

const FILTERS: FilterPolicy[] = [
  { name: "hide-pii", match: [], mcp: "postgres-ro", tool: "get_user", drop_fields: ["$.salary"] },
  // A filter belonging to a different endpoint must not show up here.
  { name: "other", match: [], mcp: "postgres-rw", tool: "get_user", drop_fields: ["$.x"] },
];

function noopProps() {
  return { onRetry: vi.fn(), onCreate: vi.fn(), onUpdate: vi.fn(), onDelete: vi.fn() };
}

describe("ResponseFilterGroup", () => {
  it("lists only the filters scoped to this endpoint", () => {
    render(
      <ResponseFilterGroup endpoint={ENDPOINT} filters={FILTERS} loading={false} error={null} {...noopProps()} />,
    );

    expect(screen.getByText("hide-pii")).toBeInTheDocument();
    expect(screen.queryByText("other")).not.toBeInTheDocument();
  });

  it("shows a 'not used yet' badge on every filter (no Users screen exists to compute real usage)", () => {
    render(
      <ResponseFilterGroup endpoint={ENDPOINT} filters={FILTERS} loading={false} error={null} {...noopProps()} />,
    );
    expect(screen.getByText("not used yet")).toBeInTheDocument();
  });

  it("opens the creation form automatically when the endpoint has no filters yet", () => {
    render(<ResponseFilterGroup endpoint={ENDPOINT} filters={[]} loading={false} error={null} {...noopProps()} />);

    expect(screen.getByPlaceholderText("filter name")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Add response filter/ })).not.toBeInTheDocument();
  });

  it("does not force the form open while filters are still loading", () => {
    render(<ResponseFilterGroup endpoint={ENDPOINT} filters={[]} loading={true} error={null} {...noopProps()} />);
    expect(screen.queryByPlaceholderText("filter name")).not.toBeInTheDocument();
  });

  it("creates one real filter, named '<name>::<tool>', from fields toggled on in the picker", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(
      <ResponseFilterGroup
        endpoint={ENDPOINT}
        filters={[]}
        loading={false}
        error={null}
        {...noopProps()}
        onCreate={onCreate}
      />,
    );

    // Form is already open (no filters yet) -- no button to click first,
    // and no tool selector either -- all tools' fields are merged into one
    // list, and the tool comes from whichever field gets toggled.
    await user.type(screen.getByPlaceholderText("filter name"), "hide-pii");
    await user.click(screen.getByRole("button", { name: "Hide get_user $.salary" }));
    await user.click(screen.getByRole("button", { name: "Create filter" }));

    expect(onCreate).toHaveBeenCalledWith("hide-pii::get_user", "get_user", ["$.salary"]);
  });

  it("rejects a filter name containing '::' -- it would silently merge into an unrelated group sharing that prefix", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(
      <ResponseFilterGroup
        endpoint={ENDPOINT}
        filters={[]}
        loading={false}
        error={null}
        {...noopProps()}
        onCreate={onCreate}
      />,
    );

    await user.type(screen.getByPlaceholderText("filter name"), "billing::extra");
    await user.click(screen.getByRole("button", { name: "Hide get_user $.salary" }));

    expect(screen.getByText(/can't contain/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create filter" })).toBeDisabled();

    // Clicking a disabled button is a no-op, but assert the guard is real
    // (not just the disabled attribute) by trying anyway.
    await user.click(screen.getByRole("button", { name: "Create filter" }));
    expect(onCreate).not.toHaveBeenCalled();
  });

  it("creates one real filter per tool -- sharing a name prefix -- when fields from several tools are picked at once", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(
      <ResponseFilterGroup
        endpoint={ENDPOINT}
        filters={[]}
        loading={false}
        error={null}
        {...noopProps()}
        onCreate={onCreate}
      />,
    );

    await user.type(screen.getByPlaceholderText("filter name"), "hide-content");
    await user.click(screen.getByRole("button", { name: "Hide get_user $.salary" }));
    await user.click(screen.getByRole("button", { name: "Hide list_users $.count" }));
    // Neither tool's fields disappear or get locked out -- both stay pickable.
    expect(screen.getByText("$.name")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Create filter" }));

    expect(onCreate).toHaveBeenCalledTimes(2);
    expect(onCreate).toHaveBeenCalledWith("hide-content::get_user", "get_user", ["$.salary"]);
    expect(onCreate).toHaveBeenCalledWith("hide-content::list_users", "list_users", ["$.count"]);
  });

  it("renders a multi-tool group as a single card in the list, not one per underlying record", () => {
    const grouped: FilterPolicy[] = [
      { name: "hide-content::get_user", match: [], mcp: "postgres-ro", tool: "get_user", drop_fields: ["$.salary"] },
      { name: "hide-content::list_users", match: [], mcp: "postgres-ro", tool: "list_users", drop_fields: ["$.count"] },
    ];
    render(
      <ResponseFilterGroup endpoint={ENDPOINT} filters={grouped} loading={false} error={null} {...noopProps()} />,
    );

    expect(screen.getAllByText("hide-content")).toHaveLength(1);
    expect(screen.getByText(/get_user: \$\.salary/)).toBeInTheDocument();
    expect(screen.getByText(/list_users: \$\.count/)).toBeInTheDocument();
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
        {...noopProps()}
        onUpdate={onUpdate}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Edit" }));
    // hide-pii starts with $.salary toggled on; also toggle $.name on.
    await user.click(screen.getByRole("button", { name: "Hide get_user $.name" }));
    await user.click(screen.getByRole("button", { name: "Save filter" }));

    // Editing an existing member keeps its exact name -- no rename.
    expect(onUpdate).toHaveBeenCalledWith("hide-pii", "get_user", ["$.salary", "$.name"]);
  });

  it("editing a multi-tool group reconciles create/update/delete against its real per-tool records", async () => {
    const user = userEvent.setup();
    const onUpdate = vi.fn().mockResolvedValue(undefined);
    const onDelete = vi.fn().mockResolvedValue(undefined);
    const grouped: FilterPolicy[] = [
      { name: "hide-content::get_user", match: [], mcp: "postgres-ro", tool: "get_user", drop_fields: ["$.salary"] },
      { name: "hide-content::list_users", match: [], mcp: "postgres-ro", tool: "list_users", drop_fields: ["$.count"] },
    ];
    render(
      <ResponseFilterGroup
        endpoint={ENDPOINT}
        filters={grouped}
        loading={false}
        error={null}
        {...noopProps()}
        onUpdate={onUpdate}
        onDelete={onDelete}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Edit" }));
    // Add a field to a tool that's already in the group...
    await user.click(screen.getByRole("button", { name: "Hide get_user $.name" }));
    // ...and remove the other tool from the group entirely.
    await user.click(screen.getByRole("button", { name: "Stop hiding list_users $.count" }));
    await user.click(screen.getByRole("button", { name: "Save filter" }));

    expect(onUpdate).toHaveBeenCalledWith("hide-content::get_user", "get_user", ["$.salary", "$.name"]);
    expect(onDelete).toHaveBeenCalledWith("hide-content::list_users");
  });

  it("deletes every underlying record when deleting a group from within its edit panel", async () => {
    const user = userEvent.setup();
    const onDelete = vi.fn().mockResolvedValue(undefined);
    const grouped: FilterPolicy[] = [
      { name: "hide-content::get_user", match: [], mcp: "postgres-ro", tool: "get_user", drop_fields: ["$.salary"] },
      { name: "hide-content::list_users", match: [], mcp: "postgres-ro", tool: "list_users", drop_fields: ["$.count"] },
    ];
    render(
      <ResponseFilterGroup
        endpoint={ENDPOINT}
        filters={grouped}
        loading={false}
        error={null}
        {...noopProps()}
        onDelete={onDelete}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Edit" }));
    await user.click(screen.getByRole("button", { name: "Delete" }));

    expect(onDelete).toHaveBeenCalledWith("hide-content::get_user");
    expect(onDelete).toHaveBeenCalledWith("hide-content::list_users");
    expect(onDelete).toHaveBeenCalledTimes(2);
  });

  it("the list search narrows filters by name, tool, or dropped field — separate from the per-form field picker search", async () => {
    const user = userEvent.setup();
    const twoFilters: FilterPolicy[] = [
      { name: "hide-pii", match: [], mcp: "postgres-ro", tool: "get_user", drop_fields: ["$.salary"] },
      { name: "hide-email", match: [], mcp: "postgres-ro", tool: "list_users", drop_fields: ["$.email"] },
    ];
    render(
      <ResponseFilterGroup endpoint={ENDPOINT} filters={twoFilters} loading={false} error={null} {...noopProps()} />,
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

  it("collapses long filter lists behind 'Show N more filters'", async () => {
    const user = userEvent.setup();
    const many: FilterPolicy[] = Array.from({ length: 5 }, (_, i) => ({
      name: `filter-${i}`,
      match: [],
      mcp: "postgres-ro",
      tool: "get_user",
      drop_fields: ["$.salary"],
    }));
    render(<ResponseFilterGroup endpoint={ENDPOINT} filters={many} loading={false} error={null} {...noopProps()} />);

    expect(screen.getAllByText(/^filter-/)).toHaveLength(3);
    expect(screen.getByRole("button", { name: "Show 2 more filters" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Show 2 more filters" }));

    expect(screen.getAllByText(/^filter-/)).toHaveLength(5);
    expect(screen.getByRole("button", { name: "Hide 2 filters" })).toBeInTheDocument();
  });

  it("disables adding a filter when the endpoint has no discovered tools", () => {
    render(
      <ResponseFilterGroup
        endpoint={{ ...ENDPOINT, tools: undefined }}
        filters={[]}
        loading={false}
        error={null}
        {...noopProps()}
      />,
    );

    expect(screen.getByRole("button", { name: /No tools discovered yet/ })).toBeDisabled();
  });
});
