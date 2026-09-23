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

    expect(screen.getByLabelText("Command")).toHaveValue("bunx");
    expect(screen.getByLabelText("Arguments (optional, space-separated)")).toHaveValue(
      "-y server-filesystem",
    );
  });

  it("saves with the endpoint's own transport and its current enabled flag carried through", async () => {
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

  // ADR-0017. Before it, this panel pinned transport to "stdio" on save and
  // had no URL field at all, because the backend rejected anything else.
  const REMOTE: MCPRegistration = {
    name: "hosted-crm",
    transport: "http",
    connect: { url: "https://mcp.crm.example.com/mcp" },
    status: "active",
    enabled: true,
    tools: {},
  };

  it("shows a remote endpoint's URL where a stdio one shows its command line", () => {
    const { unmount } = render(<EndpointDetail endpoint={ENDPOINT} {...baseProps()} />);
    expect(screen.getByText("bunx -y server-filesystem")).toBeInTheDocument();
    unmount();

    render(<EndpointDetail endpoint={REMOTE} {...baseProps()} />);
    expect(screen.getByText("https://mcp.crm.example.com/mcp")).toBeInTheDocument();
  });

  it("reports the endpoint's real transport, not a hardcoded 'stdio'", () => {
    render(<EndpointDetail endpoint={REMOTE} {...baseProps()} />);

    expect(screen.getByText("http")).toBeInTheDocument();
    expect(screen.queryByText("stdio")).not.toBeInTheDocument();
  });

  it("opens the edit panel on a remote endpoint with its URL, and no command field", async () => {
    const user = userEvent.setup();
    render(<EndpointDetail endpoint={REMOTE} {...baseProps()} />);

    await user.click(screen.getByRole("button", { name: "Edit endpoint" }));

    expect(screen.getByLabelText("URL")).toHaveValue("https://mcp.crm.example.com/mcp");
    expect(screen.queryByLabelText("Command")).not.toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "http" })).toBeChecked();
  });

  it("saves a remote endpoint with its url and without a stale command", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<EndpointDetail endpoint={REMOTE} {...props} />);

    await user.click(screen.getByRole("button", { name: "Edit endpoint" }));
    await user.clear(screen.getByLabelText("URL"));
    await user.type(screen.getByLabelText("URL"), "https://mcp.crm.example.com/v2/mcp");
    await user.click(screen.getByRole("button", { name: "Save endpoint" }));

    expect(props.onUpdate).toHaveBeenCalledWith({
      name: "hosted-crm",
      transport: "http",
      connect: { url: "https://mcp.crm.example.com/v2/mcp" },
      enabled: true,
    });
  });

  it("converts a stdio endpoint to a remote one, dropping the command it no longer uses", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<EndpointDetail endpoint={ENDPOINT} {...props} />);

    await user.click(screen.getByRole("button", { name: "Edit endpoint" }));
    await user.click(screen.getByRole("radio", { name: "sse" }));
    await user.type(screen.getByLabelText("URL"), "https://mcp.example.com/sse");
    await user.click(screen.getByRole("button", { name: "Save endpoint" }));

    expect(props.onUpdate).toHaveBeenCalledWith({
      name: "local-fs",
      transport: "sse",
      connect: { url: "https://mcp.example.com/sse" },
      enabled: true,
    });
  });

  it("will not save a remote endpoint with an empty URL", async () => {
    const user = userEvent.setup();
    render(<EndpointDetail endpoint={ENDPOINT} {...baseProps()} />);

    await user.click(screen.getByRole("button", { name: "Edit endpoint" }));
    await user.click(screen.getByRole("radio", { name: "http" }));

    expect(screen.getByRole("button", { name: "Save endpoint" })).toBeDisabled();
  });

  it("shows a rejected URL against the URL field", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    props.onUpdate.mockRejectedValueOnce(
      new Error('mcp: Config.URL: must use https (got "http://mcp.example.com/mcp")'),
    );
    render(<EndpointDetail endpoint={REMOTE} {...props} />);

    await user.click(screen.getByRole("button", { name: "Edit endpoint" }));
    await user.click(screen.getByRole("button", { name: "Save endpoint" }));

    expect(await screen.findByText(/must use https/)).toBeInTheDocument();
    expect(screen.getByLabelText("URL")).toHaveAttribute("aria-invalid", "true");
  });
});
