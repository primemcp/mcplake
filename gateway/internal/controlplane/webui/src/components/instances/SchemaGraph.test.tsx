import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ToolSchema } from "../../api/types";
import { AllToolsFieldPicker } from "./AllToolsFieldPicker";

const NESTED_TOOLS: Record<string, ToolSchema> = {
  get_deep: {
    name: "get_deep",
    output_schema: {
      type: "object",
      properties: {
        entities: {
          type: "array",
          items: { type: "object", properties: { name: { type: "string" } } },
        },
      },
    },
  },
};

const THREE_LEVEL_TOOLS: Record<string, ToolSchema> = {
  get_root: {
    name: "get_root",
    output_schema: {
      type: "object",
      properties: {
        root: {
          type: "object",
          properties: {
            user: {
              type: "object",
              properties: { id: { type: "string" }, name: { type: "string" } },
            },
          },
        },
      },
    },
  },
};

describe("SchemaGraph (via AllToolsFieldPicker's descendant-count badge)", () => {
  it("opens the graph modal and renders a node per field", async () => {
    const user = userEvent.setup();
    render(<AllToolsFieldPicker tools={NESTED_TOOLS} onChange={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "View schema graph for get_deep" }));

    expect(screen.getByRole("dialog", { name: "Schema graph" })).toBeInTheDocument();
    // The graph shows every field, including ones collapsed in the list.
    expect(screen.getByText("entities")).toBeInTheDocument();
    expect(screen.getByText("name")).toBeInTheDocument();
  });

  it("clicking an edge toggles the target field via the same onChange callback", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<AllToolsFieldPicker tools={NESTED_TOOLS} onChange={onChange} />);

    await user.click(screen.getByRole("button", { name: "View schema graph for get_deep" }));

    const edge = await waitFor(() => {
      const el = document.querySelector(".react-flow__edge");
      expect(el).toBeTruthy();
      return el!;
    });
    await user.click(edge);

    expect(onChange).toHaveBeenLastCalledWith({ get_deep: ["$.entities[*].name"] });
  });

  it("has an explicit toggle button on each edge, distinct from clicking the line -- and clicking it doesn't also double-toggle via the line's own click handler", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<AllToolsFieldPicker tools={NESTED_TOOLS} onChange={onChange} />);

    await user.click(screen.getByRole("button", { name: "View schema graph for get_deep" }));
    await waitFor(() => expect(document.querySelector(".react-flow__edge")).toBeTruthy());

    await user.click(screen.getByTitle("Exclude this field"));

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenLastCalledWith({ get_deep: ["$.entities[*].name"] });
    expect(screen.getByTitle("Include this field")).toBeInTheDocument();
  });

  it("dropping a node dashes every descendant edge too, not just the one clicked -- the real backend removes the whole subtree via one JSONPath", async () => {
    const user = userEvent.setup();
    render(<AllToolsFieldPicker tools={THREE_LEVEL_TOOLS} onChange={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "View schema graph for get_root" }));
    await waitFor(() => expect(document.querySelector(".react-flow__edge")).toBeTruthy());

    // Drop "user" itself -- never touching its "id"/"name" children.
    const userEdge = document.querySelector('[data-id="$.root->$.root.user"]');
    expect(userEdge).toBeTruthy();
    await user.click(userEdge!);

    const idEdgePath = document.querySelector('[data-id="$.root.user->$.root.user.id"] .react-flow__edge-path');
    const nameEdgePath = document.querySelector(
      '[data-id="$.root.user->$.root.user.name"] .react-flow__edge-path',
    );
    expect(idEdgePath).toHaveStyle({ stroke: "var(--color-track-off)" });
    expect(nameEdgePath).toHaveStyle({ stroke: "var(--color-track-off)" });
  });

  it("enabling a nested field blocked by a dropped ancestor un-blocks just that path, keeping its sibling branches dropped", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<AllToolsFieldPicker tools={THREE_LEVEL_TOOLS} onChange={onChange} />);

    await user.click(screen.getByRole("button", { name: "View schema graph for get_root" }));
    await waitFor(() => expect(document.querySelector(".react-flow__edge")).toBeTruthy());

    // Drop "user" first -- this hides both "id" and "name" as a side effect.
    const userEdge = document.querySelector('[data-id="$.root->$.root.user"]');
    await user.click(userEdge!);
    expect(onChange).toHaveBeenLastCalledWith({ get_root: ["$.root.user"] });

    // Now enable "id" specifically, without ever touching "name" directly.
    const idEdge = document.querySelector('[data-id="$.root.user->$.root.user.id"]');
    await user.click(idEdge!);

    // "user" can no longer be dropped as a whole -- it has to stay "open"
    // for "id" to be reachable -- but "name" was never asked for, so it's
    // the one that ends up explicitly dropped, preserving what was
    // visually hidden a moment ago.
    expect(onChange).toHaveBeenLastCalledWith({ get_root: ["$.root.user.name"] });

    const idEdgePath = document.querySelector('[data-id="$.root.user->$.root.user.id"] .react-flow__edge-path');
    const nameEdgePath = document.querySelector(
      '[data-id="$.root.user->$.root.user.name"] .react-flow__edge-path',
    );
    expect(idEdgePath).toHaveStyle({ stroke: "var(--color-success)" });
    expect(nameEdgePath).toHaveStyle({ stroke: "var(--color-track-off)" });
  });
});
