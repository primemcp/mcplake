import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MCPRegistration } from "../../api/types";
import { EndpointDetail } from "./EndpointDetail";

const ENDPOINT: MCPRegistration = {
  name: "local-fs",
  transport: "stdio",
  connect: { command: "bunx", arguments: ["-y", "server-filesystem"] },
  status: "active",
  enabled: true,
  tools: { read_file: { name: "read_file" } },
};

function baseProps() {
  return {
    onUpdate: vi.fn().mockResolvedValue(undefined),
    onRemove: vi.fn().mockResolvedValue(undefined),
    onSetEnabled: vi.fn().mockResolvedValue(undefined),
  };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("EndpointDetail", () => {
  it("shows the Enabled toggle by default, matching the mockup's on-state colors", () => {
    render(<EndpointDetail endpoint={ENDPOINT} {...baseProps()} />);
    const toggle = screen.getByRole("switch", { name: "Disable local-fs" });
    expect(toggle).toHaveAttribute("aria-checked", "true");
    expect(screen.getByText("Enabled")).toBeInTheDocument();
  });

  it("clicking the toggle calls onSetEnabled with the flipped value -- it no longer removes anything", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<EndpointDetail endpoint={ENDPOINT} {...props} />);

    await user.click(screen.getByRole("switch", { name: "Disable local-fs" }));

    expect(props.onSetEnabled).toHaveBeenCalledWith("local-fs", false);
    expect(props.onRemove).not.toHaveBeenCalled();
  });

  it("reflects a disabled endpoint: red toggle, disabled status line, 'Enable' label", () => {
    const disabled: MCPRegistration = { ...ENDPOINT, enabled: false };
    render(<EndpointDetail endpoint={disabled} {...baseProps()} />);

    const toggle = screen.getByRole("switch", { name: "Enable local-fs" });
    expect(toggle).toHaveAttribute("aria-checked", "false");
    expect(screen.getByText("Disabled")).toBeInTheDocument();
    expect(
      screen.getByText("Disabled — requests to this endpoint are rejected and grants are suspended"),
    ).toBeInTheDocument();
  });

  it("surfaces a failed toggle instead of silently reverting", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    props.onSetEnabled.mockRejectedValueOnce(new Error("mcp is not registered"));
    render(<EndpointDetail endpoint={ENDPOINT} {...props} />);

    await user.click(screen.getByRole("switch", { name: "Disable local-fs" }));

    expect(await screen.findByText("mcp is not registered")).toBeInTheDocument();
  });

  it("opens the edit panel with the endpoint's current command/arguments", async () => {
    const user = userEvent.setup();
    render(<EndpointDetail endpoint={ENDPOINT} {...baseProps()} />);

    await user.click(screen.getByRole("button", { name: "Edit endpoint" }));

    expect(screen.getByPlaceholderText("command")).toHaveValue("bunx");
    expect(screen.getByPlaceholderText("arguments (space-separated)")).toHaveValue("-y server-filesystem");
  });

  it("saves with transport fixed to stdio and the endpoint's current enabled flag carried through", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<EndpointDetail endpoint={ENDPOINT} {...props} />);

    await user.click(screen.getByRole("button", { name: "Edit endpoint" }));
    await user.click(screen.getByRole("button", { name: "Save endpoint" }));

    expect(props.onUpdate).toHaveBeenCalledWith({
      name: "local-fs",
      transport: "stdio",
      connect: { command: "bunx", arguments: ["-y", "server-filesystem"] },
      enabled: true,
    });
  });

  it("saving an edit on a disabled endpoint keeps it disabled, rather than re-enabling by omission", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    const disabled: MCPRegistration = { ...ENDPOINT, enabled: false };
    render(<EndpointDetail endpoint={disabled} {...props} />);

    await user.click(screen.getByRole("button", { name: "Edit endpoint" }));
    await user.click(screen.getByRole("button", { name: "Save endpoint" }));

    expect(props.onUpdate).toHaveBeenCalledWith(expect.objectContaining({ enabled: false }));
  });

  it("deleting lives in the edit panel now, two-step confirm, and calls onRemove only after confirming", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<EndpointDetail endpoint={ENDPOINT} {...props} />);

    await user.click(screen.getByRole("button", { name: "Edit endpoint" }));
    await user.click(screen.getByRole("button", { name: "Delete endpoint" }));

    expect(props.onRemove).not.toHaveBeenCalled();
    expect(
      screen.getByText("Deleting removes this endpoint, its filters and every grant on it — for all users."),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Confirm delete" }));

    expect(props.onRemove).toHaveBeenCalledWith("local-fs");
  });

  it("canceling the edit panel resets the delete confirmation", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<EndpointDetail endpoint={ENDPOINT} {...props} />);

    await user.click(screen.getByRole("button", { name: "Edit endpoint" }));
    await user.click(screen.getByRole("button", { name: "Delete endpoint" }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await user.click(screen.getByRole("button", { name: "Edit endpoint" }));

    expect(screen.getByRole("button", { name: "Delete endpoint" })).toBeInTheDocument();
    expect(props.onRemove).not.toHaveBeenCalled();
  });
});
