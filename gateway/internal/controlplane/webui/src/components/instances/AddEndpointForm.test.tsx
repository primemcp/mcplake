import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AddEndpointForm } from "./AddEndpointForm";

describe("AddEndpointForm", () => {
  it("disables Add until both name and command are filled", async () => {
    const user = userEvent.setup();
    render(<AddEndpointForm onCreate={vi.fn()} onCancel={vi.fn()} />);

    const addButton = screen.getByRole("button", { name: "Add endpoint" });
    expect(addButton).toBeDisabled();

    await user.type(screen.getByLabelText("Display name"), "postgres-ro");
    expect(addButton).toBeDisabled();

    await user.type(screen.getByLabelText("Command"), "mcp-server-postgres");
    expect(addButton).toBeEnabled();
  });

  it("calls onCreate with trimmed name/command and space-split arguments", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<AddEndpointForm onCreate={onCreate} onCancel={vi.fn()} />);

    await user.type(screen.getByLabelText("Display name"), "  postgres-ro  ");
    await user.type(screen.getByLabelText("Command"), "  mcp-server-postgres  ");
    await user.type(screen.getByLabelText(/Arguments/), "--read-only --db mcp");
    await user.click(screen.getByRole("button", { name: "Add endpoint" }));

    expect(onCreate).toHaveBeenCalledWith("postgres-ro", "mcp-server-postgres", [
      "--read-only",
      "--db",
      "mcp",
    ]);
  });

  it("calls onCreate with an empty argument list when none were given", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<AddEndpointForm onCreate={onCreate} onCancel={vi.fn()} />);

    await user.type(screen.getByLabelText("Display name"), "postgres-ro");
    await user.type(screen.getByLabelText("Command"), "mcp-server-postgres");
    await user.click(screen.getByRole("button", { name: "Add endpoint" }));

    expect(onCreate).toHaveBeenCalledWith("postgres-ro", "mcp-server-postgres", []);
  });

  it("shows an inline error instead of throwing when onCreate rejects", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockRejectedValue(new Error("name already registered"));
    render(<AddEndpointForm onCreate={onCreate} onCancel={vi.fn()} />);

    await user.type(screen.getByLabelText("Display name"), "postgres-ro");
    await user.type(screen.getByLabelText("Command"), "mcp-server-postgres");
    await user.click(screen.getByRole("button", { name: "Add endpoint" }));

    expect(await screen.findByText("name already registered")).toBeInTheDocument();
  });
});
