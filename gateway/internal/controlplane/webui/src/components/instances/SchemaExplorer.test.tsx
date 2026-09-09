import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { annotateFields, flattenSchema } from "../../lib/schema";
import { SchemaExplorer } from "./SchemaExplorer";

const ROWS = annotateFields(
  flattenSchema({
    type: "object",
    properties: { entities: { type: "array", items: { type: "object", properties: { name: { type: "string" } } } } },
  }),
);

describe("SchemaExplorer", () => {
  it("defaults to the Graph view", async () => {
    render(<SchemaExplorer rows={ROWS} selected={[]} onToggle={vi.fn()} />);

    await new Promise((r) => setTimeout(r, 0));
    expect(document.querySelector(".react-flow")).toBeTruthy();
    expect(screen.queryByPlaceholderText(/Search fields/)).not.toBeInTheDocument();
  });

  it("switching to Tree shows the checkbox tree instead of the graph", async () => {
    const user = userEvent.setup();
    render(<SchemaExplorer rows={ROWS} selected={[]} onToggle={vi.fn()} />);

    await user.click(screen.getByRole("tab", { name: "Tree" }));

    expect(screen.getByPlaceholderText(/Search fields/)).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "$.entities[*].name" })).toBeInTheDocument();
    expect(document.querySelector(".react-flow")).not.toBeInTheDocument();
  });

  it("toggling a field in Tree view calls onToggle with its path", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(<SchemaExplorer rows={ROWS} selected={[]} onToggle={onToggle} />);

    await user.click(screen.getByRole("tab", { name: "Tree" }));
    await user.click(screen.getByRole("checkbox", { name: "$.entities[*].name" }));

    expect(onToggle).toHaveBeenCalledWith("$.entities[*].name");
  });

  it("switching back to Graph restores it", async () => {
    const user = userEvent.setup();
    render(<SchemaExplorer rows={ROWS} selected={[]} onToggle={vi.fn()} />);

    await user.click(screen.getByRole("tab", { name: "Tree" }));
    await user.click(screen.getByRole("tab", { name: "Graph" }));

    await new Promise((r) => setTimeout(r, 0));
    expect(document.querySelector(".react-flow")).toBeTruthy();
  });
});
