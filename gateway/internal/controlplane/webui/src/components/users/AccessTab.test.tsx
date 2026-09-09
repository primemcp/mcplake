import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { FilterPolicy, Grant, MCPRegistration } from "../../api/types";
import type { UserFieldsByEndpoint } from "../../api/users";
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
    onFieldsByEndpointChange: vi.fn(),
    allFilters: [] as FilterPolicy[],
    onGoInstances: vi.fn(),
  };
}

function StatefulAccessTab({
  endpoints,
  initialGrants,
  initialFields,
  allFilters,
}: {
  endpoints: MCPRegistration[];
  initialGrants: Grant[];
  initialFields: UserFieldsByEndpoint;
  allFilters?: FilterPolicy[];
}) {
  const [grants, setGrants] = useState(initialGrants);
  const [fieldsByEndpoint, setFieldsByEndpoint] = useState(initialFields);
  return (
    <AccessTab
      endpoints={endpoints}
      grants={grants}
      onGrantsChange={setGrants}
      fieldsByEndpoint={fieldsByEndpoint}
      onFieldsByEndpointChange={setFieldsByEndpoint}
      allFilters={allFilters ?? []}
      onGoInstances={vi.fn()}
    />
  );
}

describe("AccessTab", () => {
  it("shows an empty state when there are no endpoints to grant", () => {
    render(<AccessTab endpoints={[]} grants={[]} fieldsByEndpoint={{}} {...baseProps()} />);
    expect(screen.getByText(/No MCP connections/)).toBeInTheDocument();
  });

  it("lists every endpoint in step 1, ungranted by default", () => {
    render(<AccessTab endpoints={[memory, analytics]} grants={[]} fieldsByEndpoint={{}} {...baseProps()} />);
    expect(screen.getByRole("switch", { name: "Grant demo-memory" })).toHaveAttribute("aria-checked", "false");
    expect(screen.getByRole("switch", { name: "Grant analytics" })).toHaveAttribute("aria-checked", "false");
  });

  it("granting a connected endpoint adds a wildcard grant", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<AccessTab endpoints={[memory]} grants={[]} fieldsByEndpoint={{}} {...props} />);

    await user.click(screen.getByRole("switch", { name: "Grant demo-memory" }));

    expect(props.onGrantsChange).toHaveBeenCalledWith([{ mcp: "demo-memory", tools: ["*"] }]);
  });

  it("a not-connected endpoint can't be granted", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<AccessTab endpoints={[notConnected]} grants={[]} fieldsByEndpoint={{}} {...props} />);

    const row = screen.getByRole("switch", { name: "Grant flaky-mcp" });
    expect(row).toBeDisabled();
    await user.click(row);
    expect(props.onGrantsChange).not.toHaveBeenCalled();
  });

  it("revoking an endpoint removes its grant and its saved field selection", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(
      <AccessTab
        endpoints={[memory]}
        grants={[{ mcp: "demo-memory", tools: ["*"] }]}
        fieldsByEndpoint={{ "demo-memory": { add_observations: ["$.results"] } }}
        {...props}
      />,
    );

    await user.click(screen.getByRole("switch", { name: "Grant demo-memory" }));

    expect(props.onGrantsChange).toHaveBeenCalledWith([]);
    expect(props.onFieldsByEndpointChange).toHaveBeenCalledWith({});
  });

  it("search narrows step 1 to matching endpoints", async () => {
    const user = userEvent.setup();
    render(<AccessTab endpoints={[memory, analytics]} grants={[]} fieldsByEndpoint={{}} {...baseProps()} />);

    await user.type(screen.getByPlaceholderText(/Search endpoints/), "analytics");

    expect(screen.getByRole("switch", { name: "Grant analytics" })).toBeInTheDocument();
    expect(screen.queryByRole("switch", { name: "Grant demo-memory" })).not.toBeInTheDocument();
  });

  it("the Granted filter pill narrows step 1 to only granted endpoints", async () => {
    const user = userEvent.setup();
    render(
      <AccessTab
        endpoints={[memory, analytics]}
        grants={[{ mcp: "demo-memory", tools: ["*"] }]}
        fieldsByEndpoint={{}}
        {...baseProps()}
      />,
    );

    await user.click(screen.getByRole("button", { name: /^Granted/ }));

    expect(screen.getByRole("switch", { name: "Grant demo-memory" })).toBeInTheDocument();
    expect(screen.queryByRole("switch", { name: "Grant analytics" })).not.toBeInTheDocument();
  });

  it("Select all grants every connected endpoint currently shown, skipping ones that can't connect", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<AccessTab endpoints={[memory, analytics, notConnected]} grants={[]} fieldsByEndpoint={{}} {...props} />);

    await user.click(screen.getByRole("button", { name: "Select all" }));

    expect(props.onGrantsChange).toHaveBeenCalledWith([
      { mcp: "demo-memory", tools: ["*"] },
      { mcp: "analytics", tools: ["*"] },
    ]);
  });

  it("opening in MCP connections calls onGoInstances without toggling the grant", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<AccessTab endpoints={[memory]} grants={[]} fieldsByEndpoint={{}} {...props} />);

    await user.click(screen.getByRole("button", { name: "Open demo-memory in MCP connections" }));

    expect(props.onGoInstances).toHaveBeenCalledOnce();
    expect(props.onGrantsChange).not.toHaveBeenCalled();
  });

  it("a granted endpoint defaults to all tools and lists them for narrowing", () => {
    render(
      <AccessTab
        endpoints={[memory]}
        grants={[{ mcp: "demo-memory", tools: ["*"] }]}
        fieldsByEndpoint={{}}
        {...baseProps()}
      />,
    );
    expect(screen.getByRole("switch", { name: "All tools for demo-memory" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("checkbox", { name: "add_observations" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "delete_entities" })).toBeChecked();
  });

  it("turning off All tools then unchecking one tool narrows the grant", async () => {
    const user = userEvent.setup();
    render(
      <StatefulAccessTab endpoints={[memory]} initialGrants={[{ mcp: "demo-memory", tools: ["*"] }]} initialFields={{}} />,
    );

    await user.click(screen.getByRole("switch", { name: "All tools for demo-memory" }));
    await user.click(screen.getByRole("checkbox", { name: "delete_entities" }));

    expect(screen.getByRole("checkbox", { name: "add_observations" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "delete_entities" })).not.toBeChecked();
  });

  it("step 2 only shows a filter card for granted endpoints, scoped to their granted tools", () => {
    render(
      <AccessTab
        endpoints={[memory, analytics]}
        grants={[{ mcp: "demo-memory", tools: ["add_observations"] }]}
        fieldsByEndpoint={{}}
        {...baseProps()}
      />,
    );

    const step2 = screen.getByText(/Response filters/).closest("section")!;
    expect(within(step2).getByRole("button", { name: /demo-memory/ })).toBeInTheDocument();
    expect(within(step2).queryByRole("button", { name: /^analytics/ })).not.toBeInTheDocument();
    expect(within(step2).getByText("$.results")).toBeInTheDocument();
    expect(within(step2).queryByText(/delete_entities/)).not.toBeInTheDocument();
  });

  it("step 2 is always visible (the mockup's own 2-column layout), showing a hint instead of a filter card until an endpoint is granted", () => {
    render(<AccessTab endpoints={[memory]} grants={[]} fieldsByEndpoint={{}} {...baseProps()} />);
    expect(screen.getByText(/Response filters/)).toBeInTheDocument();
    expect(screen.getByText("Select one or more endpoints.")).toBeInTheDocument();
  });

  it("dropping a field in step 2 reports the per-endpoint field selection", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(
      <AccessTab endpoints={[memory]} grants={[{ mcp: "demo-memory", tools: ["*"] }]} fieldsByEndpoint={{}} {...props} />,
    );

    await user.click(screen.getByRole("button", { name: "Hide add_observations $.results" }));

    expect(props.onFieldsByEndpointChange).toHaveBeenCalledWith({ "demo-memory": { add_observations: ["$.results"] } });
  });

  it("offers other existing filters on the same endpoint as a starting template, and applying one prefills the picker", async () => {
    const user = userEvent.setup();
    const template: FilterPolicy = {
      name: "other-user::demo-memory::add_observations",
      match: [{ path: "$.role", pattern: "^other$" }],
      mcp: "demo-memory",
      tool: "add_observations",
      drop_fields: ["$.results"],
    };
    render(
      <StatefulAccessTab
        endpoints={[memory]}
        initialGrants={[{ mcp: "demo-memory", tools: ["*"] }]}
        initialFields={{}}
        allFilters={[template]}
      />,
    );

    await user.selectOptions(
      screen.getByRole("combobox", { name: "Start from an existing filter on demo-memory" }),
      template.name,
    );

    expect(screen.getByRole("button", { name: "Stop hiding add_observations $.results" })).toBeInTheDocument();
  });
});
