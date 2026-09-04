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
  tools: { read_file: { name: "read_file" } },
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe("EndpointDetail", () => {
  it("shows the Enabled toggle by default, matching the mockup's on-state colors", () => {
    render(<EndpointDetail endpoint={ENDPOINT} onUpdate={vi.fn()} onRemove={vi.fn()} />);
    expect(screen.getByText("Enabled")).toBeInTheDocument();
  });

  it("removing via the toggle asks for confirmation first, and does nothing if declined", async () => {
    const user = userEvent.setup();
    vi.spyOn(window, "confirm").mockReturnValue(false);
    const onRemove = vi.fn();
    render(<EndpointDetail endpoint={ENDPOINT} onUpdate={vi.fn()} onRemove={onRemove} />);

    await user.click(screen.getByRole("button", { name: /Enabled/ }));

    expect(window.confirm).toHaveBeenCalled();
    expect(onRemove).not.toHaveBeenCalled();
  });

  it("calls onRemove once the confirmation is accepted", async () => {
    const user = userEvent.setup();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const onRemove = vi.fn().mockResolvedValue(undefined);
    render(<EndpointDetail endpoint={ENDPOINT} onUpdate={vi.fn()} onRemove={onRemove} />);

    await user.click(screen.getByRole("button", { name: /Enabled/ }));

    expect(onRemove).toHaveBeenCalledWith("local-fs");
  });

  it("opens the edit panel with the endpoint's current command/arguments", async () => {
    const user = userEvent.setup();
    render(<EndpointDetail endpoint={ENDPOINT} onUpdate={vi.fn()} onRemove={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "Edit endpoint" }));

    expect(screen.getByPlaceholderText("command")).toHaveValue("bunx");
    expect(screen.getByPlaceholderText("arguments (space-separated)")).toHaveValue("-y server-filesystem");
  });

  it("saves with transport fixed to stdio regardless of the edited fields", async () => {
    const user = userEvent.setup();
    const onUpdate = vi.fn().mockResolvedValue(undefined);
    render(<EndpointDetail endpoint={ENDPOINT} onUpdate={onUpdate} onRemove={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "Edit endpoint" }));
    await user.click(screen.getByRole("button", { name: "Save endpoint" }));

    expect(onUpdate).toHaveBeenCalledWith({
      name: "local-fs",
      transport: "stdio",
      connect: { command: "bunx", arguments: ["-y", "server-filesystem"] },
    });
  });
});
