import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { MCPRegistration } from "../../api/types";
import type { User } from "../../api/users";
import { UserDetail } from "./UserDetail";

const memory: MCPRegistration = {
  name: "demo-memory",
  transport: "stdio",
  connect: { command: "memory" },
  status: "active",
  tools: {
    add_observations: {
      name: "add_observations",
      output_schema: { type: "object", properties: { id: { type: "string" } } },
    },
  },
};

function baseProps() {
  return {
    endpoints: [memory],
    allFilters: [],
    onGoInstances: vi.fn(),
    onCreateAccessPolicy: vi.fn().mockResolvedValue(undefined),
    onUpdateAccessPolicy: vi.fn().mockResolvedValue(undefined),
    onDeleteAccessPolicy: vi.fn().mockResolvedValue(undefined),
    onCreateFilter: vi.fn().mockResolvedValue(undefined),
    onUpdateFilter: vi.fn().mockResolvedValue(undefined),
    onDeleteFilter: vi.fn().mockResolvedValue(undefined),
    onSaved: vi.fn(),
    onDeleted: vi.fn(),
  };
}

describe("UserDetail", () => {
  it("creating a new user: Save is disabled until a name and a condition exist", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<UserDetail user={null} {...props} />);

    expect(screen.getByRole("button", { name: /Save/ })).toBeDisabled();

    await user.type(screen.getByPlaceholderText("user name"), "analyst-team");
    expect(screen.getByRole("button", { name: /Save/ })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: /Add condition/ }));
    await user.type(screen.getByLabelText("Condition 1 path"), "$.role");
    await user.type(screen.getByLabelText("Condition 1 regex"), "^analyst$");

    expect(screen.getByRole("button", { name: /Save/ })).toBeEnabled();
  });

  it("saving a new user creates the access policy with the drafted name, match and grants", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<UserDetail user={null} {...props} />);

    await user.type(screen.getByPlaceholderText("user name"), "analyst-team");
    await user.click(screen.getByRole("button", { name: /Add condition/ }));
    await user.clear(screen.getByLabelText("Condition 1 path"));
    await user.type(screen.getByLabelText("Condition 1 path"), "$.role");
    await user.type(screen.getByLabelText("Condition 1 regex"), "^analyst$");
    await user.click(screen.getByRole("button", { name: /^Save/ }));

    expect(props.onCreateAccessPolicy).toHaveBeenCalledWith({
      name: "analyst-team",
      match: [{ path: "$.role", pattern: "^analyst$" }],
      grants: [],
    });
    expect(props.onUpdateAccessPolicy).not.toHaveBeenCalled();
    expect(props.onSaved).toHaveBeenCalledWith("analyst-team");
  });

  it("editing an existing user: Save starts disabled, the name isn't re-editable, and the Access tab carries its live grant count", () => {
    const existing: User = {
      name: "analyst-team",
      match: [{ path: "$.role", pattern: "^analyst$" }],
      grants: [{ mcp: "demo-memory", tools: ["*"] }],
      filters: [],
    };
    render(<UserDetail user={existing} {...baseProps()} />);

    // The name is shown once, in the list row on the left (per the real
    // mockup) -- this panel doesn't repeat it as its own heading.
    expect(screen.queryByPlaceholderText("user name")).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Access 1" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Save/ })).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Discard" })).not.toBeInTheDocument();
  });

  it("editing a condition on an existing user enables Save and Discard, and Discard reverts it", async () => {
    const user = userEvent.setup();
    const existing: User = {
      name: "analyst-team",
      match: [{ path: "$.role", pattern: "^analyst$" }],
      grants: [],
      filters: [],
    };
    render(<UserDetail user={existing} {...baseProps()} />);

    await user.clear(screen.getByLabelText("Condition 1 regex"));
    await user.type(screen.getByLabelText("Condition 1 regex"), "^lead$");
    expect(screen.getByRole("button", { name: /^Save/ })).toBeEnabled();

    await user.click(screen.getByRole("button", { name: "Discard" }));
    expect(screen.getByLabelText("Condition 1 regex")).toHaveValue("^analyst$");
    expect(screen.getByRole("button", { name: /^Save/ })).toBeDisabled();
  });

  it("saving an existing user updates the access policy in place, never creates", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    const existing: User = {
      name: "analyst-team",
      match: [{ path: "$.role", pattern: "^analyst$" }],
      grants: [],
      filters: [],
    };
    render(<UserDetail user={existing} {...props} />);

    await user.clear(screen.getByLabelText("Condition 1 regex"));
    await user.type(screen.getByLabelText("Condition 1 regex"), "^lead$");
    await user.click(screen.getByRole("button", { name: /^Save/ }));

    expect(props.onUpdateAccessPolicy).toHaveBeenCalledWith("analyst-team", {
      name: "analyst-team",
      match: [{ path: "$.role", pattern: "^lead$" }],
      grants: [],
    });
    expect(props.onCreateAccessPolicy).not.toHaveBeenCalled();
  });

  it("granting an endpoint on the Access tab and saving creates a filter carrying the user's match", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    const existing: User = {
      name: "analyst-team",
      match: [{ path: "$.role", pattern: "^analyst$" }],
      grants: [],
      filters: [],
    };
    render(<UserDetail user={existing} {...props} />);

    await user.click(screen.getByRole("tab", { name: "Access 0" }));
    await user.click(screen.getByRole("switch", { name: "Grant demo-memory" }));
    await user.click(screen.getByRole("button", { name: /^Save/ }));

    expect(props.onUpdateAccessPolicy).toHaveBeenCalledWith("analyst-team", {
      name: "analyst-team",
      match: [{ path: "$.role", pattern: "^analyst$" }],
      grants: [{ mcp: "demo-memory", tools: ["*"] }],
    });
  });

  it("saving reconciles filters: keeps an existing member's name, drops a deselected one", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    const existing: User = {
      name: "analyst-team",
      match: [{ path: "$.role", pattern: "^analyst$" }],
      grants: [{ mcp: "demo-memory", tools: ["*"] }],
      filters: [
        { name: "analyst-team::demo-memory::add_observations", match: [], mcp: "demo-memory", tool: "add_observations", drop_fields: [] },
      ],
    };
    render(<UserDetail user={existing} {...props} />);

    await user.click(screen.getByRole("tab", { name: "Access 1" }));
    await user.click(screen.getByRole("button", { name: "Hide add_observations $.id" }));
    await user.click(screen.getByRole("button", { name: /^Save/ }));

    expect(props.onUpdateFilter).toHaveBeenCalledWith("analyst-team::demo-memory::add_observations", {
      name: "analyst-team::demo-memory::add_observations",
      match: [{ path: "$.role", pattern: "^analyst$" }],
      mcp: "demo-memory",
      tool: "add_observations",
      drop_fields: ["$.id"],
    });
    expect(props.onCreateFilter).not.toHaveBeenCalled();
    expect(props.onDeleteFilter).not.toHaveBeenCalled();
  });

  it("deleting an existing user removes its access policy and every one of its filters", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    const existing: User = {
      name: "analyst-team",
      match: [{ path: "$.role", pattern: "^analyst$" }],
      grants: [{ mcp: "demo-memory", tools: ["*"] }],
      filters: [
        { name: "analyst-team::demo-memory::add_observations", match: [], mcp: "demo-memory", tool: "add_observations", drop_fields: [] },
      ],
    };
    render(<UserDetail user={existing} {...props} />);

    await user.click(screen.getByRole("button", { name: "Delete user" }));

    expect(props.onDeleteAccessPolicy).toHaveBeenCalledWith("analyst-team");
    expect(props.onDeleteFilter).toHaveBeenCalledWith("analyst-team::demo-memory::add_observations");
    expect(props.onDeleted).toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it("does not show a Delete button when creating a new user", () => {
    render(<UserDetail user={null} {...baseProps()} />);
    expect(screen.queryByRole("button", { name: "Delete user" })).not.toBeInTheDocument();
  });
});
