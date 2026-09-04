import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SchemaFieldPicker } from "./SchemaFieldPicker";

const SCHEMA = {
  type: "object",
  properties: {
    path: { type: "string" },
    email: { type: "string" },
    head: { type: "number" },
  },
};

describe("SchemaFieldPicker", () => {
  it("lists every field from the schema", () => {
    render(<SchemaFieldPicker schema={SCHEMA} />);
    expect(screen.getByText("$.path")).toBeInTheDocument();
    expect(screen.getByText("$.email")).toBeInTheDocument();
    expect(screen.getByText("$.head")).toBeInTheDocument();
  });

  it("search narrows the visible fields", async () => {
    const user = userEvent.setup();
    render(<SchemaFieldPicker schema={SCHEMA} />);

    await user.type(screen.getByPlaceholderText(/Search fields/), "email");

    expect(screen.getByText("$.email")).toBeInTheDocument();
    expect(screen.queryByText("$.path")).not.toBeInTheDocument();
  });

  it("shows a no-match message when the search has no hits", async () => {
    const user = userEvent.setup();
    render(<SchemaFieldPicker schema={SCHEMA} />);

    await user.type(screen.getByPlaceholderText(/Search fields/), "nope");

    expect(screen.getByText("No field matches that.")).toBeInTheDocument();
  });

  it("renders no toggle and nothing is clickable when onToggle is omitted (read-only mode)", () => {
    render(<SchemaFieldPicker schema={SCHEMA} />);
    expect(screen.queryAllByRole("button")).toHaveLength(0);
  });

  it("renders a toggle per field and calls onToggle when selected/onToggle are given", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(<SchemaFieldPicker schema={SCHEMA} selected={["$.email"]} onToggle={onToggle} />);

    expect(screen.getByRole("button", { name: "Stop hiding $.email" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Hide $.path" }));
    expect(onToggle).toHaveBeenCalledWith("$.path");
  });

  it("shows a fallback when the schema itself has no fields", () => {
    render(<SchemaFieldPicker schema={undefined} />);
    expect(screen.getByText("This tool doesn't advertise any fields.")).toBeInTheDocument();
  });
});
