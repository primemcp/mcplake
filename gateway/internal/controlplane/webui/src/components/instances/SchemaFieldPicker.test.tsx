import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SchemaFieldPicker } from "./SchemaFieldPicker";

const SCHEMA = {
  type: "object",
  properties: {
    path: { type: "string" },
    email: { type: "string", description: "PII" },
    head: { type: "number" },
  },
};

describe("SchemaFieldPicker", () => {
  it("lists every field from the schema, with its description alongside the type", () => {
    render(<SchemaFieldPicker schema={SCHEMA} selected={[]} onToggle={vi.fn()} />);
    expect(screen.getByText("$.path")).toBeInTheDocument();
    expect(screen.getByText("$.email")).toBeInTheDocument();
    expect(screen.getByText("string · PII")).toBeInTheDocument();
  });

  it("shows the field count and optional meta line in the heading", () => {
    render(<SchemaFieldPicker schema={SCHEMA} selected={[]} onToggle={vi.fn()} meta="tools/list · pg-ro" />);
    expect(screen.getByText("Discovered response fields")).toBeInTheDocument();
    expect(screen.getByText("3 fields")).toBeInTheDocument();
    expect(screen.getByText("tools/list · pg-ro")).toBeInTheDocument();
  });

  it("search narrows the visible fields", async () => {
    const user = userEvent.setup();
    render(<SchemaFieldPicker schema={SCHEMA} selected={[]} onToggle={vi.fn()} />);

    await user.type(screen.getByPlaceholderText(/Search fields/), "email");

    expect(screen.getByText("$.email")).toBeInTheDocument();
    expect(screen.queryByText("$.path")).not.toBeInTheDocument();
  });

  it("shows a no-match message when the search has no hits", async () => {
    const user = userEvent.setup();
    render(<SchemaFieldPicker schema={SCHEMA} selected={[]} onToggle={vi.fn()} />);

    await user.type(screen.getByPlaceholderText(/Search fields/), "nope");

    expect(screen.getByText("No field matches that.")).toBeInTheDocument();
  });

  it("renders a toggle per field and calls onToggle when clicked", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(<SchemaFieldPicker schema={SCHEMA} selected={["$.email"]} onToggle={onToggle} />);

    expect(screen.getByRole("button", { name: "Stop hiding $.email" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Hide $.path" }));
    expect(onToggle).toHaveBeenCalledWith("$.path");
  });

  it("says all fields pass through when nothing is toggled off", () => {
    render(<SchemaFieldPicker schema={SCHEMA} selected={[]} onToggle={vi.fn()} />);
    expect(screen.getByText(/All 3 fields pass through/)).toBeInTheDocument();
  });

  it("reports how many fields are removed once some are toggled off", () => {
    render(<SchemaFieldPicker schema={SCHEMA} selected={["$.email"]} onToggle={vi.fn()} />);
    expect(screen.getByText("1 of 3 fields removed from the response")).toBeInTheDocument();
  });

  it("shows a fallback when the schema itself has no fields", () => {
    render(<SchemaFieldPicker schema={undefined} selected={[]} onToggle={vi.fn()} />);
    expect(screen.getByText("This tool doesn't advertise any fields.")).toBeInTheDocument();
  });
});
