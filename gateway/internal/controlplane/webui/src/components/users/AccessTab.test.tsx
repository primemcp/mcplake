import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { Grant, MCPRegistration } from "../../api/types";
import type { UserFieldsByEndpoint } from "../../api/users";
import { AccessTab } from "./AccessTab";

const memory: MCPRegistration = {
  name: "demo-memory",
  transport: "stdio",
  connect: { command: "memory" },
  status: "connected",
  tools: {
    add_observations: {
      name: "add_observations",
      output_schema: {
        type: "object",
        properties: { results: { type: "array", items: { type: "object", properties: { id: { type: "string" } } } } },
      },
    },
    delete_entities: { name: "delete_entities", output_schema: { type: "object", properties: { ok: { type: "boolean" } } } },
  },
};

const analytics: MCPRegistration = {
  name: "analytics",
  transport: "stdio",
  connect: { command: "analytics" },
  status: "connected",
  tools: { get_report: { name: "get_report", output_schema: { type: "object", properties: { total: { type: "number" } } } } },
};

function StatefulAccessTab({
  endpoints,
  initialGrants,
  initialFields,
}: {
  endpoints: MCPRegistration[];
  initialGrants: Grant[];
  initialFields: UserFieldsByEndpoint;
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
    />
  );
}

describe("AccessTab", () => {
  it("shows an empty state when there are no endpoints to grant", () => {
    render(
      <AccessTab endpoints={[]} grants={[]} onGrantsChange={vi.fn()} fieldsByEndpoint={{}} onFieldsByEndpointChange={vi.fn()} />,
    );
    expect(screen.getByText(/No MCP connections/)).toBeInTheDocument();
  });

  it("lists every endpoint in step 1, ungranted by default", () => {
    render(
      <AccessTab
        endpoints={[memory, analytics]}
        grants={[]}
        onGrantsChange={vi.fn()}
        fieldsByEndpoint={{}}
        onFieldsByEndpointChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("switch", { name: "Grant demo-memory" })).toHaveAttribute("aria-checked", "false");
    expect(screen.getByRole("switch", { name: "Grant analytics" })).toHaveAttribute("aria-checked", "false");
  });

  it("granting an endpoint adds a wildcard grant", async () => {
    const user = userEvent.setup();
    const onGrantsChange = vi.fn();
    render(
      <AccessTab
        endpoints={[memory]}
        grants={[]}
        onGrantsChange={onGrantsChange}
        fieldsByEndpoint={{}}
        onFieldsByEndpointChange={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("switch", { name: "Grant demo-memory" }));

    expect(onGrantsChange).toHaveBeenCalledWith([{ mcp: "demo-memory", tools: ["*"] }]);
  });

  it("revoking an endpoint removes its grant and its saved field selection", async () => {
    const user = userEvent.setup();
    const onGrantsChange = vi.fn();
    const onFieldsByEndpointChange = vi.fn();
    render(
      <AccessTab
        endpoints={[memory]}
        grants={[{ mcp: "demo-memory", tools: ["*"] }]}
        onGrantsChange={onGrantsChange}
        fieldsByEndpoint={{ "demo-memory": { add_observations: ["$.results"] } }}
        onFieldsByEndpointChange={onFieldsByEndpointChange}
      />,
    );

    await user.click(screen.getByRole("switch", { name: "Grant demo-memory" }));

    expect(onGrantsChange).toHaveBeenCalledWith([]);
    expect(onFieldsByEndpointChange).toHaveBeenCalledWith({});
  });

  it("a granted endpoint defaults to all tools and lists them for narrowing", () => {
    render(
      <AccessTab
        endpoints={[memory]}
        grants={[{ mcp: "demo-memory", tools: ["*"] }]}
        onGrantsChange={vi.fn()}
        fieldsByEndpoint={{}}
        onFieldsByEndpointChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("switch", { name: "All tools for demo-memory" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("checkbox", { name: "add_observations" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "delete_entities" })).toBeChecked();
  });

  it("turning off All tools then unchecking one tool narrows the grant", async () => {
    const user = userEvent.setup();
    render(
      <StatefulAccessTab
        endpoints={[memory]}
        initialGrants={[{ mcp: "demo-memory", tools: ["*"] }]}
        initialFields={{}}
      />,
    );

    await user.click(screen.getByRole("switch", { name: "All tools for demo-memory" }));
    await user.click(screen.getByRole("checkbox", { name: "delete_entities" }));

    expect(screen.getByRole("checkbox", { name: "add_observations" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "delete_entities" })).not.toBeChecked();
  });

  it("step 2 only shows a filter picker for granted endpoints, scoped to their granted tools", () => {
    render(
      <AccessTab
        endpoints={[memory, analytics]}
        grants={[{ mcp: "demo-memory", tools: ["add_observations"] }]}
        onGrantsChange={vi.fn()}
        fieldsByEndpoint={{}}
        onFieldsByEndpointChange={vi.fn()}
      />,
    );

    const step2 = screen.getByText(/Response filters/).closest("section")!;
    expect(within(step2).getByText("demo-memory")).toBeInTheDocument();
    expect(within(step2).queryByText("analytics")).not.toBeInTheDocument();
    expect(within(step2).getByText("$.results")).toBeInTheDocument();
    expect(within(step2).queryByText(/delete_entities/)).not.toBeInTheDocument();
  });

  it("step 2 shows no filter section at all until at least one endpoint is granted", () => {
    render(
      <AccessTab endpoints={[memory]} grants={[]} onGrantsChange={vi.fn()} fieldsByEndpoint={{}} onFieldsByEndpointChange={vi.fn()} />,
    );
    expect(screen.queryByText(/Response filters/)).not.toBeInTheDocument();
  });

  it("dropping a field in step 2 reports the per-endpoint field selection", async () => {
    const user = userEvent.setup();
    const onFieldsByEndpointChange = vi.fn();
    render(
      <AccessTab
        endpoints={[memory]}
        grants={[{ mcp: "demo-memory", tools: ["*"] }]}
        onGrantsChange={vi.fn()}
        fieldsByEndpoint={{}}
        onFieldsByEndpointChange={onFieldsByEndpointChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Hide add_observations $.results" }));

    expect(onFieldsByEndpointChange).toHaveBeenCalledWith({ "demo-memory": { add_observations: ["$.results"] } });
  });
});
