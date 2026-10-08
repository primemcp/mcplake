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

  it("calls onCreate with a stdio request, trimmed, with space-split arguments", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<AddEndpointForm onCreate={onCreate} onCancel={vi.fn()} />);

    await user.type(screen.getByLabelText("Display name"), "  postgres-ro  ");
    await user.type(screen.getByLabelText("Command"), "  mcp-server-postgres  ");
    await user.type(screen.getByLabelText(/Arguments/), "--read-only --db mcp");
    await user.click(screen.getByRole("button", { name: "Add endpoint" }));

    expect(onCreate).toHaveBeenCalledWith({
      name: "postgres-ro",
      transport: "stdio",
      connect: { command: "mcp-server-postgres", arguments: ["--read-only", "--db", "mcp"] },
    });
  });

  it("omits arguments entirely when none were given", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<AddEndpointForm onCreate={onCreate} onCancel={vi.fn()} />);

    await user.type(screen.getByLabelText("Display name"), "postgres-ro");
    await user.type(screen.getByLabelText("Command"), "mcp-server-postgres");
    await user.click(screen.getByRole("button", { name: "Add endpoint" }));

    expect(onCreate).toHaveBeenCalledWith({
      name: "postgres-ro",
      transport: "stdio",
      connect: { command: "mcp-server-postgres", arguments: undefined },
    });
  });

  it("swaps command/arguments for a URL when a remote transport is picked", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<AddEndpointForm onCreate={onCreate} onCancel={vi.fn()} />);

    await user.click(screen.getByRole("radio", { name: "http" }));

    expect(screen.queryByLabelText("Command")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/Arguments/)).not.toBeInTheDocument();
    expect(screen.getByLabelText("URL")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Display name"), "hosted-crm");
    await user.type(screen.getByLabelText("URL"), "https://mcp.crm.example.com/mcp");
    await user.click(screen.getByRole("button", { name: "Add endpoint" }));

    expect(onCreate).toHaveBeenCalledWith({
      name: "hosted-crm",
      transport: "http",
      connect: { url: "https://mcp.crm.example.com/mcp" },
    });
  });

  it("sends only the picked transport's fields, never the other's leftovers", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<AddEndpointForm onCreate={onCreate} onCancel={vi.fn()} />);

    // Fill in a command, change your mind, fill in a URL. The backend
    // rejects a command on an http entry outright (config.validateMCP), so
    // carrying the abandoned value along would turn a form the operator
    // filled in correctly into a 400.
    await user.type(screen.getByLabelText("Display name"), "hosted-crm");
    await user.type(screen.getByLabelText("Command"), "mcp-server-postgres");
    await user.click(screen.getByRole("radio", { name: "http" }));
    await user.type(screen.getByLabelText("URL"), "https://mcp.crm.example.com/mcp");
    await user.click(screen.getByRole("button", { name: "Add endpoint" }));

    const req = onCreate.mock.calls[0][0];
    expect(req.connect).toEqual({ url: "https://mcp.crm.example.com/mcp" });
    expect(req.connect.command).toBeUndefined();
  });

  it("keeps what you typed when you switch transport and switch back", async () => {
    const user = userEvent.setup();
    render(<AddEndpointForm onCreate={vi.fn()} onCancel={vi.fn()} />);

    await user.type(screen.getByLabelText("Command"), "mcp-server-postgres");
    await user.click(screen.getByRole("radio", { name: "http" }));
    await user.click(screen.getByRole("radio", { name: "stdio" }));

    expect(screen.getByLabelText("Command")).toHaveValue("mcp-server-postgres");
  });

  it("requires a URL, not a command, once a remote transport is picked", async () => {
    const user = userEvent.setup();
    render(<AddEndpointForm onCreate={vi.fn()} onCancel={vi.fn()} />);

    await user.type(screen.getByLabelText("Display name"), "hosted-crm");
    await user.type(screen.getByLabelText("Command"), "mcp-server-postgres");
    expect(screen.getByRole("button", { name: "Add endpoint" })).toBeEnabled();

    await user.click(screen.getByRole("radio", { name: "sse" }));
    expect(screen.getByRole("button", { name: "Add endpoint" })).toBeDisabled();

    await user.type(screen.getByLabelText("URL"), "https://mcp.example.com/sse");
    expect(screen.getByRole("button", { name: "Add endpoint" })).toBeEnabled();
  });

  it("states the URL rule up front rather than making you submit to find it", async () => {
    const user = userEvent.setup();
    render(<AddEndpointForm onCreate={vi.fn()} onCancel={vi.fn()} />);

    await user.click(screen.getByRole("radio", { name: "http" }));

    expect(screen.getByText(/loopback host/i)).toBeInTheDocument();
  });

  it("shows a rejected URL against the URL field, not as a loose banner", async () => {
    const user = userEvent.setup();
    const onCreate = vi
      .fn()
      .mockRejectedValue(
        new Error(
          'adminservice: mcp registration failed: mcp: Config.URL: must use https (got "http://mcp.example.com/mcp"); plaintext http is accepted only for a loopback host',
        ),
      );
    render(<AddEndpointForm onCreate={onCreate} onCancel={vi.fn()} />);

    await user.click(screen.getByRole("radio", { name: "http" }));
    await user.type(screen.getByLabelText("Display name"), "hosted-crm");
    await user.type(screen.getByLabelText("URL"), "http://mcp.example.com/mcp");
    await user.click(screen.getByRole("button", { name: "Add endpoint" }));

    expect(await screen.findByText(/must use https/)).toBeInTheDocument();
    expect(screen.getByLabelText("URL")).toHaveAttribute("aria-invalid", "true");
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
