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

  it("disables other tools' fields once one is selected, and re-enables everything once cleared", async () => {
    const user = userEvent.setup();
    render(<AllToolsFieldPicker tools={TOOLS} onChange={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "Hide get_user $.salary" }));
    expect(screen.getByRole("button", { name: "Hide list_users $.count" })).toBeDisabled();

    // Toggling the same field back off clears the active tool entirely.
    await user.click(screen.getByRole("button", { name: "Stop hiding get_user $.salary" }));
    expect(screen.getByRole("button", { name: "Hide list_users $.count" })).toBeEnabled();
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
});
