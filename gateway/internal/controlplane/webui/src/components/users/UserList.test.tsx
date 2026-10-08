import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { User } from "../../api/users";
import { UserList } from "./UserList";

const USERS: User[] = [
  {
    name: "analyst-team",
    match: [{ path: "$.role", pattern: "^analyst$" }],
    grants: [{ mcp: "demo-memory", tools: ["*"] }],
    filters: [],
  },
  {
    name: "admin-team",
    match: [{ path: "$.role", pattern: "^admin$" }],
    grants: [],
    filters: [],
  },
  {
    name: "orphaned",
    match: [],
    grants: [],
    filters: [],
  },
];

describe("UserList", () => {
  it("search narrows the visible users by name", async () => {
    const user = userEvent.setup();
    render(
      <UserList
        users={USERS}
        loading={false}
        error={null}
        onRetry={vi.fn()}
        selectedName={null}
        onSelect={vi.fn()}
        onAddNew={vi.fn()}
      />,
    );

    expect(screen.getAllByRole("button", { name: /-team|orphaned/ })).toHaveLength(3);

    await user.type(screen.getByPlaceholderText("Search users"), "analyst");

    expect(screen.getAllByRole("button", { name: /-team|orphaned/ })).toHaveLength(1);
    expect(screen.queryByText("admin-team")).not.toBeInTheDocument();
  });

  it("shows the error notice and retries on click when loading failed", async () => {
    const user = userEvent.setup();
    const onRetry = vi.fn();
    render(
      <UserList users={[]} loading={false} error={new Error("boom")} onRetry={onRetry} selectedName={null} onSelect={vi.fn()} onAddNew={vi.fn()} />,
    );

    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("selecting a user calls onSelect with its name", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(
      <UserList users={USERS} loading={false} error={null} onRetry={vi.fn()} selectedName={null} onSelect={onSelect} onAddNew={vi.fn()} />,
    );

    await user.click(screen.getByRole("button", { name: /admin-team/ }));

    expect(onSelect).toHaveBeenCalledWith("admin-team");
  });

  it("flags a user with no match conditions as matching every token", () => {
    render(
      <UserList users={USERS} loading={false} error={null} onRetry={vi.fn()} selectedName={null} onSelect={vi.fn()} onAddNew={vi.fn()} />,
    );
    expect(screen.getByText(/no conditions/i)).toBeInTheDocument();
  });

  it("clicking Add user calls onAddNew", async () => {
    const user = userEvent.setup();
    const onAddNew = vi.fn();
    render(
      <UserList users={USERS} loading={false} error={null} onRetry={vi.fn()} selectedName={null} onSelect={vi.fn()} onAddNew={onAddNew} />,
    );

    await user.click(screen.getByRole("button", { name: /Add user/ }));

    expect(onAddNew).toHaveBeenCalledOnce();
  });

  it("shows an empty state when there are no users yet", () => {
    render(<UserList users={[]} loading={false} error={null} onRetry={vi.fn()} selectedName={null} onSelect={vi.fn()} onAddNew={vi.fn()} />);
    expect(screen.getByText("No users yet.")).toBeInTheDocument();
  });
});
