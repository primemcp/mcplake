import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ToolSchema } from "../../api/types";
import { AllToolsFieldPicker } from "./AllToolsFieldPicker";

const TOOLS: Record<string, ToolSchema> = {
  get_user: {
    name: "get_user",
    output_schema: { type: "object", properties: { name: { type: "string" }, salary: { type: "number" } } },
  },
  list_users: {
    name: "list_users",
    output_schema: { type: "object", properties: { count: { type: "number" } } },
  },
};

describe("AllToolsFieldPicker", () => {
  it("merges every tool's fields into one list with no tool-select step", () => {
    render(<AllToolsFieldPicker tools={TOOLS} onChange={vi.fn()} />);

    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    expect(screen.getByText("$.name")).toBeInTheDocument();
    expect(screen.getByText("$.salary")).toBeInTheDocument();
    expect(screen.getByText("$.count")).toBeInTheDocument();
  });

  it("reports the dropped fields grouped by tool", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<AllToolsFieldPicker tools={TOOLS} onChange={onChange} />);

    await user.click(screen.getByRole("button", { name: "Hide get_user $.salary" }));

    expect(onChange).toHaveBeenLastCalledWith({ get_user: ["$.salary"] });
  });

  it("reports the correct (tool, path) pair even when a tool name itself contains a space", async () => {
    // Tool names come straight from whatever the MCP server advertises via
    // list_tools() -- nothing validates their charset -- so a naive
    // space-delimited composite key would misparse this into
    // tool="get", path="user $.salary" instead.
    const user = userEvent.setup();
    const onChange = vi.fn();
    const spacedTools: Record<string, ToolSchema> = {
      "get user": {
        name: "get user",
        output_schema: { type: "object", properties: { salary: { type: "number" } } },
      },
    };
    render(<AllToolsFieldPicker tools={spacedTools} onChange={onChange} />);

    await user.click(screen.getByRole("button", { name: "Hide get user $.salary" }));

    expect(onChange).toHaveBeenLastCalledWith({ "get user": ["$.salary"] });
  });

  it("allows toggling fields across multiple tools at once -- a filter spanning several tools is a valid selection now", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<AllToolsFieldPicker tools={TOOLS} onChange={onChange} />);

    await user.click(screen.getByRole("button", { name: "Hide get_user $.salary" }));
    await user.click(screen.getByRole("button", { name: "Hide list_users $.count" }));

    // Neither tool's fields ever leave the list, and the switch stays enabled.
    expect(screen.getByText("$.name")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Stop hiding list_users $.count" })).toBeEnabled();
    expect(onChange).toHaveBeenLastCalledWith({ get_user: ["$.salary"], list_users: ["$.count"] });
  });

  it("prefills from the `initial` prop, e.g. when editing an existing multi-tool group", () => {
    render(
      <AllToolsFieldPicker
        tools={TOOLS}
        onChange={vi.fn()}
        initial={{ get_user: ["$.salary"], list_users: ["$.count"] }}
      />,
    );

    expect(screen.getByRole("button", { name: "Stop hiding get_user $.salary" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Stop hiding list_users $.count" })).toBeInTheDocument();
  });

  it("search narrows the merged list across every tool", async () => {
    const user = userEvent.setup();
    render(<AllToolsFieldPicker tools={TOOLS} onChange={vi.fn()} />);

    await user.type(screen.getByPlaceholderText(/Search fields/), "count");

    expect(screen.getByText("$.count")).toBeInTheDocument();
    expect(screen.queryByText("$.salary")).not.toBeInTheDocument();
  });

  it("shows a fallback when no tool has any discovered fields", () => {
    render(<AllToolsFieldPicker tools={{ noop: { name: "noop" } }} onChange={vi.fn()} />);
    expect(screen.getByText("No tools have discovered response fields yet.")).toBeInTheDocument();
  });

  describe("with a nested schema", () => {
    const NESTED_TOOLS: Record<string, ToolSchema> = {
      create_entities: {
        name: "create_entities",
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

    it("never expands nested fields inline -- browsing shows only top-level fields, with a clickable descendant-count badge", () => {
      render(<AllToolsFieldPicker tools={NESTED_TOOLS} onChange={vi.fn()} />);

      expect(screen.queryByText("$.entities[*].name")).not.toBeInTheDocument();
      const badge = screen.getByRole("button", { name: "View schema graph for create_entities" });
      expect(badge).toHaveTextContent("[1]");
    });

    it("search still reaches nested fields, flat (no stair-step indent) since there's no expand state any more", async () => {
      const user = userEvent.setup();
      render(<AllToolsFieldPicker tools={NESTED_TOOLS} onChange={vi.fn()} />);

      await user.type(screen.getByPlaceholderText(/Search fields/), "name");

      const hit = screen.getByText("$.entities[*].name");
      expect(hit).toBeInTheDocument();
      expect(hit.closest("div[style]")).toHaveStyle({ paddingLeft: "0px" });
    });

    it("clicking the descendant-count badge opens the schema graph for that tool", async () => {
      const user = userEvent.setup();
      render(<AllToolsFieldPicker tools={NESTED_TOOLS} onChange={vi.fn()} />);

      await user.click(screen.getByRole("button", { name: "View schema graph for create_entities" }));

      expect(screen.getByRole("dialog", { name: "Schema graph" })).toBeInTheDocument();
    });

    it("shows how many descendants are currently dropped, in red, once one is toggled off via the graph", async () => {
      const user = userEvent.setup();
      render(<AllToolsFieldPicker tools={NESTED_TOOLS} onChange={vi.fn()} />);

      await user.click(screen.getByRole("button", { name: "View schema graph for create_entities" }));
      const edge = await waitFor(() => {
        const el = document.querySelector(".react-flow__edge");
        expect(el).toBeTruthy();
        return el!;
      });
      await user.click(edge);

      const badge = screen.getByRole("button", { name: "View schema graph for create_entities" });
      expect(badge).toHaveTextContent("[1] -1");
    });

    it("propagates the dropped count to every descendant once an ancestor is dropped, not just the one explicitly toggled", async () => {
      const user = userEvent.setup();
      const propagationTools: Record<string, ToolSchema> = {
        get_root: {
          name: "get_root",
          output_schema: {
            type: "object",
            properties: {
              root: {
                type: "object",
                properties: {
                  mid: { type: "object", properties: { a: { type: "string" }, b: { type: "string" } } },
                },
              },
            },
          },
        },
      };
      render(<AllToolsFieldPicker tools={propagationTools} onChange={vi.fn()} />);

      await user.click(screen.getByRole("button", { name: "View schema graph for get_root" }));
      const edge = await waitFor(() => {
        const el = document.querySelector('[data-id="$.root->$.root.mid"]');
        expect(el).toBeTruthy();
        return el!;
      });
      // Drop "mid" itself -- never touching its "a"/"b" children.
      await user.click(edge);

      // $.root has 3 descendants total (mid, mid.a, mid.b); dropping "mid"
      // removes its whole subtree on the real backend, so all 3 are
      // effectively dropped, not just the 1 explicitly-clicked "mid" edge.
      const badge = screen.getByRole("button", { name: "View schema graph for get_root" });
      expect(badge).toHaveTextContent("[3] -3");
    });
  });
});
