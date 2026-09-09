import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { FilterPolicy, MCPRegistration } from "../../api/types";
import { AccessTab } from "./AccessTab";

const memory: MCPRegistration = {
  name: "demo-memory",
  transport: "stdio",
  connect: { command: "memory" },
  status: "active",
  tools: {
    add_observations: {
      name: "add_observations",
      output_schema: { type: "object", properties: { results: { type: "array", items: { type: "object", properties: { id: { type: "string" } } } } } },
    },
    delete_entities: { name: "delete_entities", output_schema: { type: "object", properties: { ok: { type: "boolean" } } } },
  },
};

const analytics: MCPRegistration = {
  name: "analytics",
  transport: "stdio",
  connect: { command: "analytics" },
  status: "active",
  tools: { get_report: { name: "get_report", output_schema: { type: "object", properties: { total: { type: "number" } } } } },
};

const notConnected: MCPRegistration = {
  name: "flaky-mcp",
  transport: "stdio",
  connect: { command: "flaky" },
  status: "unreachable",
  tools: { ping: { name: "ping", output_schema: { type: "object", properties: {} } } },
};

function baseProps() {
  return {
    onGrantsChange: vi.fn(),
    allFilters: [] as FilterPolicy[],
    onGoInstances: vi.fn(),
    userName: "alice" as string | null,
    userMatch: [{ path: "$.role", pattern: "^analyst$" }],
    onCreateFilter: vi.fn().mockResolvedValue(undefined),
    onUpdateFilter: vi.fn().mockResolvedValue(undefined),
    onDeleteFilter: vi.fn().mockResolvedValue(undefined),
  };
}

describe("AccessTab", () => {
  it("shows an empty state when there are no endpoints to grant", () => {
    render(<AccessTab endpoints={[]} grants={[]} {...baseProps()} />);
    expect(screen.getByText(/No MCP connections/)).toBeInTheDocument();
  });

  it("lists every endpoint in step 1, ungranted by default", () => {
    render(<AccessTab endpoints={[memory, analytics]} grants={[]} {...baseProps()} />);
    expect(screen.getByRole("switch", { name: "Grant demo-memory" })).toHaveAttribute("aria-checked", "false");
    expect(screen.getByRole("switch", { name: "Grant analytics" })).toHaveAttribute("aria-checked", "false");
  });

  it("granting a connected endpoint adds a wildcard grant", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<AccessTab endpoints={[memory]} grants={[]} {...props} />);

    await user.click(screen.getByRole("switch", { name: "Grant demo-memory" }));

    expect(props.onGrantsChange).toHaveBeenCalledWith([{ mcp: "demo-memory", tools: ["*"] }]);
  });

  it("a not-connected endpoint can't be granted", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<AccessTab endpoints={[notConnected]} grants={[]} {...props} />);

    const row = screen.getByRole("switch", { name: "Grant flaky-mcp" });
    expect(row).toBeDisabled();
    await user.click(row);
    expect(props.onGrantsChange).not.toHaveBeenCalled();
  });

  it("revoking an endpoint removes its grant", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<AccessTab endpoints={[memory]} grants={[{ mcp: "demo-memory", tools: ["*"] }]} {...props} />);

    await user.click(screen.getByRole("switch", { name: "Grant demo-memory" }));

    expect(props.onGrantsChange).toHaveBeenCalledWith([]);
  });

  it("search narrows step 1 to matching endpoints", async () => {
    const user = userEvent.setup();
    render(<AccessTab endpoints={[memory, analytics]} grants={[]} {...baseProps()} />);

    await user.type(screen.getByPlaceholderText(/Search endpoints/), "analytics");

    expect(screen.getByRole("switch", { name: "Grant analytics" })).toBeInTheDocument();
    expect(screen.queryByRole("switch", { name: "Grant demo-memory" })).not.toBeInTheDocument();
  });

  it("the Granted filter pill narrows step 1 to only granted endpoints", async () => {
    const user = userEvent.setup();
    render(
      <AccessTab endpoints={[memory, analytics]} grants={[{ mcp: "demo-memory", tools: ["*"] }]} {...baseProps()} />,
    );

    await user.click(screen.getByRole("button", { name: /^Granted/ }));

    expect(screen.getByRole("switch", { name: "Grant demo-memory" })).toBeInTheDocument();
    expect(screen.queryByRole("switch", { name: "Grant analytics" })).not.toBeInTheDocument();
  });

  it("Select all grants every connected endpoint currently shown, skipping ones that can't connect", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<AccessTab endpoints={[memory, analytics, notConnected]} grants={[]} {...props} />);

    await user.click(screen.getByRole("button", { name: "Select all" }));

    expect(props.onGrantsChange).toHaveBeenCalledWith([
      { mcp: "demo-memory", tools: ["*"] },
      { mcp: "analytics", tools: ["*"] },
    ]);
  });

  it("opening in MCP connections calls onGoInstances without toggling the grant", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<AccessTab endpoints={[memory]} grants={[]} {...props} />);

    await user.click(screen.getByRole("button", { name: "Open demo-memory in MCP connections" }));

    expect(props.onGoInstances).toHaveBeenCalledOnce();
    expect(props.onGrantsChange).not.toHaveBeenCalled();
  });

  it("a granted endpoint defaults to all tools and lists them for narrowing", () => {
    render(
      <AccessTab endpoints={[memory]} grants={[{ mcp: "demo-memory", tools: ["*"] }]} {...baseProps()} />,
    );
    expect(screen.getByRole("switch", { name: "All tools for demo-memory" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("checkbox", { name: "add_observations" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "delete_entities" })).toBeChecked();
  });

  it("turning off All tools then unchecking one tool narrows the grant", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    const { rerender } = render(
      <AccessTab endpoints={[memory]} grants={[{ mcp: "demo-memory", tools: ["*"] }]} {...props} />,
    );

    await user.click(screen.getByRole("switch", { name: "All tools for demo-memory" }));
    const afterAllOff = props.onGrantsChange.mock.calls.at(-1)![0];
    rerender(<AccessTab endpoints={[memory]} grants={afterAllOff} {...props} />);

    await user.click(screen.getByRole("checkbox", { name: "delete_entities" }));
    const afterUncheck = props.onGrantsChange.mock.calls.at(-1)![0];
    rerender(<AccessTab endpoints={[memory]} grants={afterUncheck} {...props} />);

    expect(screen.getByRole("checkbox", { name: "add_observations" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "delete_entities" })).not.toBeChecked();
  });

  describe("step 2 -- response filters", () => {
    it("is always visible (the mockup's own 2-column layout), showing a hint until an endpoint is granted", () => {
      render(<AccessTab endpoints={[memory]} grants={[]} {...baseProps()} />);
      expect(screen.getByText(/Response filters/)).toBeInTheDocument();
      expect(screen.getByText("Select one or more endpoints.")).toBeInTheDocument();
    });

    it("prompts to save the user first when drafting a brand new one -- filters need a real AccessPolicy name to belong to", () => {
      render(
        <AccessTab
          endpoints={[memory]}
          grants={[{ mcp: "demo-memory", tools: ["*"] }]}
          {...baseProps()}
          userName={null}
        />,
      );
      expect(screen.getByText(/Save the user first/)).toBeInTheDocument();
    });

    it("shows one collapsed row per named filter this user owns on the endpoint, with a dropped-field-count badge", () => {
      const filters: FilterPolicy[] = [
        {
          name: "alice::Mask customer PII::add_observations",
          match: [],
          mcp: "demo-memory",
          tool: "add_observations",
          drop_fields: ["$.results", "$.entityName"],
        },
      ];
      render(
        <AccessTab
          endpoints={[memory]}
          grants={[{ mcp: "demo-memory", tools: ["*"] }]}
          {...baseProps()}
          allFilters={filters}
        />,
      );

      expect(screen.getByText("Mask customer PII")).toBeInTheDocument();
      expect(screen.getByText(/add_observations: \$\.results.*\[2\]/)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Edit Mask customer PII" })).toBeInTheDocument();
      // Collapsed -- the full picker isn't mounted until Edit is clicked.
      expect(screen.queryByText("Discovered response fields")).not.toBeInTheDocument();
    });

    it("a different user's filter on the same endpoint never shows up", () => {
      const filters: FilterPolicy[] = [
        { name: "bob::Something::add_observations", match: [], mcp: "demo-memory", tool: "add_observations", drop_fields: ["$.results"] },
      ];
      render(
        <AccessTab
          endpoints={[memory]}
          grants={[{ mcp: "demo-memory", tools: ["*"] }]}
          {...baseProps()}
          allFilters={filters}
        />,
      );
      expect(screen.queryByText("Something")).not.toBeInTheDocument();
    });

    it("editing an existing named filter updates its members in place, keeping the user-prefixed name and current match", async () => {
      const user = userEvent.setup();
      const props = baseProps();
      const filters: FilterPolicy[] = [
        {
          name: "alice::Mask customer PII::add_observations",
          match: [{ path: "$.role", pattern: "^old$" }],
          mcp: "demo-memory",
          tool: "add_observations",
          drop_fields: ["$.results"],
        },
      ];
      render(
        <AccessTab
          endpoints={[memory]}
          grants={[{ mcp: "demo-memory", tools: ["*"] }]}
          {...props}
          allFilters={filters}
        />,
      );

      await user.click(screen.getByRole("button", { name: "Edit Mask customer PII" }));
      // Adds a second tool to the same named filter -- the group now
      // spans two real FilterPolicy records sharing the "Mask customer
      // PII" label, same convention as ResponseFilterGroup's own
      // multi-tool groups.
      await user.click(screen.getByRole("button", { name: "Hide delete_entities $.ok" }));

      expect(props.onCreateFilter).toHaveBeenCalledWith({
        name: "alice::Mask customer PII::delete_entities",
        match: props.userMatch,
        mcp: "demo-memory",
        tool: "delete_entities",
        drop_fields: ["$.ok"],
      });
    });

    it("creating a new named filter mints a '<user>::<label>::<tool>' name", async () => {
      const user = userEvent.setup();
      const props = baseProps();
      render(
        <AccessTab endpoints={[memory]} grants={[{ mcp: "demo-memory", tools: ["*"] }]} {...props} />,
      );

      await user.click(screen.getByRole("button", { name: /Add response filter/ }));
      await user.type(screen.getByPlaceholderText("filter name"), "Mask PII");
      await user.click(screen.getByRole("button", { name: "Hide add_observations $.results" }));
      await user.click(screen.getByRole("button", { name: "Create filter" }));

      expect(props.onCreateFilter).toHaveBeenCalledWith({
        name: "alice::Mask PII::add_observations",
        match: props.userMatch,
        mcp: "demo-memory",
        tool: "add_observations",
        drop_fields: ["$.results"],
      });
    });

    it("deleting a named filter removes every one of its members", async () => {
      const user = userEvent.setup();
      const props = baseProps();
      const filters: FilterPolicy[] = [
        { name: "alice::Mask PII::add_observations", match: [], mcp: "demo-memory", tool: "add_observations", drop_fields: ["$.results"] },
        { name: "alice::Mask PII::delete_entities", match: [], mcp: "demo-memory", tool: "delete_entities", drop_fields: ["$.ok"] },
      ];
      render(
        <AccessTab
          endpoints={[memory]}
          grants={[{ mcp: "demo-memory", tools: ["*"] }]}
          {...props}
          allFilters={filters}
        />,
      );

      await user.click(screen.getByRole("button", { name: "Edit Mask PII" }));
      await user.click(screen.getByRole("button", { name: "Delete" }));

      expect(props.onDeleteFilter).toHaveBeenCalledWith("alice::Mask PII::add_observations");
      expect(props.onDeleteFilter).toHaveBeenCalledWith("alice::Mask PII::delete_entities");
    });

    it("the single search box narrows named filters across every granted endpoint at once", async () => {
      const user = userEvent.setup();
      const filters: FilterPolicy[] = [
        { name: "alice::Mask PII::add_observations", match: [], mcp: "demo-memory", tool: "add_observations", drop_fields: ["$.results"] },
        { name: "alice::Report scrub::get_report", match: [], mcp: "analytics", tool: "get_report", drop_fields: ["$.total"] },
      ];
      render(
        <AccessTab
          endpoints={[memory, analytics]}
          grants={[
            { mcp: "demo-memory", tools: ["*"] },
            { mcp: "analytics", tools: ["*"] },
          ]}
          {...baseProps()}
          allFilters={filters}
        />,
      );

      await user.type(screen.getByPlaceholderText("Search filters or fields"), "Report");

      expect(screen.getByText("Report scrub")).toBeInTheDocument();
      expect(screen.queryByText("Mask PII")).not.toBeInTheDocument();
    });

    it("rejects a typed filter name containing '::' -- reserved for internal grouping", async () => {
      const user = userEvent.setup();
      render(
        <AccessTab endpoints={[memory]} grants={[{ mcp: "demo-memory", tools: ["*"] }]} {...baseProps()} />,
      );

      await user.click(screen.getByRole("button", { name: /Add response filter/ }));
      await user.type(screen.getByPlaceholderText("filter name"), "bad::name");

      expect(screen.getByRole("button", { name: "Create filter" })).toBeDisabled();
    });
  });
});
