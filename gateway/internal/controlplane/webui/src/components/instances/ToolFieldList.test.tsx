import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ToolFieldList } from "./ToolFieldList";

describe("ToolFieldList", () => {
  it("lists each schema field with its path and type", () => {
    render(
      <ToolFieldList
        schema={{
          type: "object",
          properties: { path: { type: "string" }, head: { type: "number" } },
        }}
      />,
    );

    expect(screen.getByText("$.path")).toBeInTheDocument();
    expect(screen.getByText("$.head")).toBeInTheDocument();
    expect(screen.getByText("number")).toBeInTheDocument();
  });

  it("shows a fallback when the tool has no parameters", () => {
    render(<ToolFieldList schema={undefined} />);
    expect(screen.getByText("No parameters.")).toBeInTheDocument();
  });
});
