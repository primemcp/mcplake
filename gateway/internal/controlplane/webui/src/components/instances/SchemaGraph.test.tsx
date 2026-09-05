import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SchemaFieldPicker } from "./SchemaFieldPicker";

const NESTED_SCHEMA = {
  type: "object",
  properties: {
    entities: {
      type: "array",
      items: { type: "object", properties: { name: { type: "string" } } },
    },
  },
};

describe("SchemaGraph (via SchemaFieldPicker's eye button)", () => {
  it("opens the graph modal and renders a node per field", async () => {
    const user = userEvent.setup();
    render(<SchemaFieldPicker schema={NESTED_SCHEMA} selected={[]} onToggle={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "View schema graph" }));

    expect(screen.getByRole("dialog", { name: "Schema graph" })).toBeInTheDocument();
    // The graph shows every field, including ones collapsed in the list.
    expect(screen.getByText("entities")).toBeInTheDocument();
    expect(screen.getByText("name")).toBeInTheDocument();
  });

  it("clicking an edge toggles the target field via the same onToggle callback", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(<SchemaFieldPicker schema={NESTED_SCHEMA} selected={[]} onToggle={onToggle} />);

    await user.click(screen.getByRole("button", { name: "View schema graph" }));

    const edge = await waitFor(() => {
      const el = document.querySelector(".react-flow__edge");
      expect(el).toBeTruthy();
      return el!;
    });
    await user.click(edge);

    expect(onToggle).toHaveBeenCalledWith("$.entities[*].name");
  });
});
