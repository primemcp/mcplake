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

describe("SchemaGraph (via AllToolsFieldPicker's eye button)", () => {
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
});
