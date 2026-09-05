import { render, screen } from "@testing-library/react";
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

  it("reports the owning tool and dropped paths once a field is toggled", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<AllToolsFieldPicker tools={TOOLS} onChange={onChange} />);

    await user.click(screen.getByRole("button", { name: "Hide get_user $.salary" }));

    expect(onChange).toHaveBeenCalledWith("get_user", ["$.salary"]);
  });

  it("collapses to just the active tool's fields once one is selected, and merges everything back once cleared", async () => {
    const user = userEvent.setup();
    render(<AllToolsFieldPicker tools={TOOLS} onChange={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "Hide get_user $.salary" }));
    expect(screen.queryByText("$.count")).not.toBeInTheDocument();
    expect(screen.getByText(/Scoped to/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "switch tool" }));
    expect(screen.getByText("$.count")).toBeInTheDocument();
    expect(screen.queryByText(/Scoped to/)).not.toBeInTheDocument();
  });

  it("toggling the active tool's only dropped field back off clears the scope too", async () => {
    const user = userEvent.setup();
    render(<AllToolsFieldPicker tools={TOOLS} onChange={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "Hide get_user $.salary" }));
    await user.click(screen.getByRole("button", { name: "Stop hiding get_user $.salary" }));

    expect(screen.getByText("$.count")).toBeInTheDocument();
    expect(screen.queryByText(/Scoped to/)).not.toBeInTheDocument();
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

    it("collapses nested fields by default, showing a depth badge instead", () => {
      render(<AllToolsFieldPicker tools={NESTED_TOOLS} onChange={vi.fn()} />);

      expect(screen.getByText("[1]")).toBeInTheDocument();
      expect(screen.queryByText("$.entities[*].name")).not.toBeInTheDocument();
    });

    it("reveals children on expand, and search bypasses collapse entirely", async () => {
      const user = userEvent.setup();
      render(<AllToolsFieldPicker tools={NESTED_TOOLS} onChange={vi.fn()} />);

      await user.click(screen.getByRole("button", { name: "Expand create_entities $.entities" }));
      expect(screen.getByText("$.entities[*].name")).toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: "Collapse create_entities $.entities" }));
      expect(screen.queryByText("$.entities[*].name")).not.toBeInTheDocument();

      // Still collapsed, but a search finds the hidden nested field anyway.
      await user.type(screen.getByPlaceholderText(/Search fields/), "name");
      expect(screen.getByText("$.entities[*].name")).toBeInTheDocument();
    });
  });
});
