import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { MCPRegistration } from "../../api/types";
import { EndpointList } from "./EndpointList";

const ENDPOINTS: MCPRegistration[] = [
  { name: "postgres-ro", transport: "stdio", connect: { command: "pg-ro" }, status: "active", enabled: true },
  { name: "postgres-rw", transport: "stdio", connect: { command: "pg-rw" }, status: "active", enabled: true },
  { name: "filesystem", transport: "stdio", connect: { command: "fs" }, status: "unreachable", enabled: true },
];

describe("EndpointList", () => {
  it("search narrows the visible endpoints by name", async () => {
    const user = userEvent.setup();
    render(
      <EndpointList
        endpoints={ENDPOINTS}
        loading={false}
        error={null}
        onRetry={vi.fn()}
        selectedName={null}
        onSelect={vi.fn()}
        onCreate={vi.fn()}
      />,
    );

    expect(screen.getAllByRole("button", { name: /postgres|filesystem/ })).toHaveLength(3);

    await user.type(screen.getByPlaceholderText("Search connections, URLs"), "postgres");

    const remaining = screen.getAllByRole("button", { name: /postgres|filesystem/ });
    expect(remaining).toHaveLength(2);
    expect(screen.queryByText("filesystem")).not.toBeInTheDocument();
  });

  it("shows the error notice and retries on click when loading failed", async () => {
    const user = userEvent.setup();
    const onRetry = vi.fn();
    render(
      <EndpointList
        endpoints={[]}
        loading={false}
        error={new Error("boom")}
        onRetry={onRetry}
        selectedName={null}
        onSelect={vi.fn()}
        onCreate={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("opens the add-endpoint dialog centered, not inline, and closes it after a successful add", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(
      <EndpointList
        endpoints={ENDPOINTS}
        loading={false}
        error={null}
        onRetry={vi.fn()}
        selectedName={null}
        onSelect={vi.fn()}
        onCreate={onCreate}
      />,
    );

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Add MCP endpoint/ }));
    expect(screen.getByRole("dialog", { name: "Add MCP endpoint" })).toBeInTheDocument();

    await user.type(screen.getByLabelText("Display name"), "new-mcp");
    await user.type(screen.getByLabelText("Command"), "mcp-server-new");
    await user.click(screen.getByRole("button", { name: "Add endpoint" }));

    expect(onCreate).toHaveBeenCalledWith("new-mcp", "mcp-server-new", []);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows a disabled endpoint's row as 'disabled', not its real connection status", () => {
    const withDisabled: MCPRegistration[] = [
      { ...ENDPOINTS[0], enabled: false },
      ENDPOINTS[1],
    ];
    render(
      <EndpointList
        endpoints={withDisabled}
        loading={false}
        error={null}
        onRetry={vi.fn()}
        selectedName={null}
        onSelect={vi.fn()}
        onCreate={vi.fn()}
      />,
    );

    expect(screen.getByText("disabled")).toBeInTheDocument();
    expect(screen.getByText("active")).toBeInTheDocument();
  });
});
