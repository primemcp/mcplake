import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { annotateFields, flattenSchema } from "../../lib/schema";
import { SchemaTree } from "./SchemaTree";

const NESTED_ROWS = annotateFields(
  flattenSchema({
    type: "object",
    properties: {
      entities: {
        type: "array",
        items: { type: "object", properties: { name: { type: "string" }, id: { type: "string" } } },
      },
    },
  }),
);

describe("SchemaTree", () => {
  it("shows every field fully expanded, including nested ones, with no search applied", () => {
    render(<SchemaTree rows={NESTED_ROWS} selected={[]} onToggle={vi.fn()} />);

    expect(screen.getByText("$.entities")).toBeInTheDocument();
    expect(screen.getByText("$.entities[*].name")).toBeInTheDocument();
    expect(screen.getByText("$.entities[*].id")).toBeInTheDocument();
  });

  it("a field's checkbox is checked when it passes through and unchecked once dropped", () => {
    render(<SchemaTree rows={NESTED_ROWS} selected={["$.entities[*].id"]} onToggle={vi.fn()} />);

    expect(screen.getByRole("checkbox", { name: "$.entities[*].name" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "$.entities[*].id" })).not.toBeChecked();
  });

  it("toggling a checkbox reports that field's path", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(<SchemaTree rows={NESTED_ROWS} selected={[]} onToggle={onToggle} />);

    await user.click(screen.getByRole("checkbox", { name: "$.entities[*].name" }));

    expect(onToggle).toHaveBeenCalledWith("$.entities[*].name");
  });

  it("dropping a branch shows the dropped-descendant count on it, without hiding the descendants themselves", () => {
    render(<SchemaTree rows={NESTED_ROWS} selected={["$.entities"]} onToggle={vi.fn()} />);

    expect(screen.getByText("-2")).toBeInTheDocument();
    // Still fully visible -- dropping doesn't collapse the tree.
    expect(screen.getByText("$.entities[*].name")).toBeInTheDocument();
    expect(screen.getByText("$.entities[*].id")).toBeInTheDocument();
  });

  it("searching highlights matching rows without hiding the rest", async () => {
    const user = userEvent.setup();
    render(<SchemaTree rows={NESTED_ROWS} selected={[]} onToggle={vi.fn()} />);

    await user.type(screen.getByPlaceholderText(/Search fields/), "name");

    const match = screen.getByText("$.entities[*].name");
    const nonMatch = screen.getByText("$.entities[*].id");
    expect(match.closest("label")).toHaveClass("bg-warn-soft");
    expect(nonMatch.closest("label")).not.toHaveClass("bg-warn-soft");
    // Non-matching rows stay visible -- search highlights, it doesn't filter.
    expect(nonMatch).toBeInTheDocument();
  });

  it("scrolls the first match into view -- a highlight nobody can see (scrolled off-screen in a big schema) isn't useful", async () => {
    const user = userEvent.setup();
    const scrollIntoView = vi.fn();
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(scrollIntoView);
    render(<SchemaTree rows={NESTED_ROWS} selected={[]} onToggle={vi.fn()} />);

    await user.type(screen.getByPlaceholderText(/Search fields/), "name");

    expect(scrollIntoView).toHaveBeenCalled();
    vi.restoreAllMocks();
  });
});
