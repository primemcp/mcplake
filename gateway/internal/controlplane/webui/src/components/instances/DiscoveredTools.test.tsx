import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { ToolSchema } from "../../api/types";
import { DiscoveredTools } from "./DiscoveredTools";

const TOOLS: ToolSchema[] = [
  {
    name: "read_file",
    input_schema: { type: "object", properties: { path: { type: "string" } } },
  },
  {
    name: "write_file",
    input_schema: {
      type: "object",
      properties: { path: { type: "string" }, content: { type: "string" } },
    },
  },
];

describe("DiscoveredTools", () => {
  it("renders one search box for the whole section, not one per tool", () => {
    render(<DiscoveredTools tools={TOOLS} />);
    expect(screen.getAllByPlaceholderText("Search tools or fields")).toHaveLength(1);
  });

  it("shows every tool and all its fields with no query", () => {
    render(<DiscoveredTools tools={TOOLS} />);
    expect(screen.getByText("read_file")).toBeInTheDocument();
    expect(screen.getByText("write_file")).toBeInTheDocument();
    expect(screen.getByText("$.content")).toBeInTheDocument();
  });

  it("a field match narrows to matching tools, showing only the matching field", async () => {
    const user = userEvent.setup();
    render(<DiscoveredTools tools={TOOLS} />);

    await user.type(screen.getByPlaceholderText("Search tools or fields"), "content");

    expect(screen.getByText("write_file")).toBeInTheDocument();
    expect(screen.queryByText("read_file")).not.toBeInTheDocument();
    expect(screen.getByText("$.content")).toBeInTheDocument();
  });

  it("a name match shows the tool with its full, unfiltered field list", async () => {
    const user = userEvent.setup();
    render(<DiscoveredTools tools={TOOLS} />);

    await user.type(screen.getByPlaceholderText("Search tools or fields"), "read_file");

    expect(screen.getByText("read_file")).toBeInTheDocument();
    expect(screen.getByText("$.path")).toBeInTheDocument();
    expect(screen.queryByText("write_file")).not.toBeInTheDocument();
  });

  it("shows a no-match state when nothing matches", async () => {
    const user = userEvent.setup();
    render(<DiscoveredTools tools={TOOLS} />);

    await user.type(screen.getByPlaceholderText("Search tools or fields"), "nope");

    expect(screen.getByText("No tool matches that.")).toBeInTheDocument();
  });

  it("shows the empty state when nothing has been discovered yet", () => {
    render(<DiscoveredTools tools={[]} />);
    expect(
      screen.getByText("No tools discovered yet — the endpoint may still be connecting."),
    ).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search tools or fields")).not.toBeInTheDocument();
  });
});
