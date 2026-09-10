import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AccessPolicy } from "../../api/types";
import { UsersScreen } from "./UsersScreen";

const ACCESS_POLICIES: AccessPolicy[] = [
  { name: "analyst-team", match: [{ path: "$.role", pattern: "^analyst$" }], grants: [], enabled: true },
  { name: "admin-team", match: [{ path: "$.role", pattern: "^admin$" }], grants: [], enabled: true },
];

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
}

function stubFetch(createdAccessPolicies: AccessPolicy[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (url === "/admin/mcps") return Promise.resolve(jsonResponse([]));
      if (url === "/admin/filter-policies") return Promise.resolve(jsonResponse([]));
      if (url === "/admin/access-policies" && (!init || init.method === undefined)) {
        return Promise.resolve(jsonResponse([...ACCESS_POLICIES, ...createdAccessPolicies]));
      }
      if (url === "/admin/access-policies" && init?.method === "POST") {
        const body = JSON.parse(init.body as string) as AccessPolicy;
        createdAccessPolicies.push(body);
        return Promise.resolve(jsonResponse(body));
      }
      throw new Error(`unexpected fetch: ${url} ${init?.method}`);
    }),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

describe("UsersScreen", () => {
  it("selecting a user and the Access tab survives a simulated reload", async () => {
    const user = userEvent.setup();
    stubFetch([]);
    const { unmount } = render(<UsersScreen onGoInstances={vi.fn()} />);

    await user.click(await screen.findByRole("button", { name: /analyst-team/ }));
    await user.click(screen.getByRole("tab", { name: /Access/ }));
    expect(screen.getByRole("tab", { name: /Access/ })).toHaveAttribute("aria-selected", "true");

    // A reload remounts the whole tree fresh -- simulate that rather than
    // relying on any in-memory state surviving.
    unmount();
    stubFetch([]);
    render(<UsersScreen onGoInstances={vi.fn()} />);

    await waitFor(() => expect(screen.getByRole("tab", { name: /Access/ })).toHaveAttribute("aria-selected", "true"));
    expect(screen.getByRole("button", { name: "Delete user" })).toBeInTheDocument();
  });

  it("drops a persisted selection that points at a user who no longer exists", async () => {
    localStorage.setItem("mcplake:users:selection", JSON.stringify({ name: "ghost-team", tab: "access" }));
    stubFetch([]);
    render(<UsersScreen onGoInstances={vi.fn()} />);

    await screen.findByRole("button", { name: /analyst-team/ });
    expect(screen.getByText("No user selected")).toBeInTheDocument();
  });

  it("switching the selected user does not leak the previous one's edited draft", async () => {
    const user = userEvent.setup();
    stubFetch([]);
    render(<UsersScreen onGoInstances={vi.fn()} />);

    await user.click(await screen.findByRole("button", { name: /analyst-team/ }));
    await user.clear(await screen.findByLabelText("Condition 1 regex"));
    await user.type(screen.getByLabelText("Condition 1 regex"), "^lead$");

    await user.click(screen.getByRole("button", { name: /admin-team/ }));

    expect(await screen.findByLabelText("Condition 1 regex")).toHaveValue("^admin$");
  });

  it("adding a new user end to end creates a real access policy and selects it", async () => {
    const user = userEvent.setup();
    const created: AccessPolicy[] = [];
    stubFetch(created);
    render(<UsersScreen onGoInstances={vi.fn()} />);

    await user.click(await screen.findByRole("button", { name: /Add user/ }));
    await user.type(screen.getByPlaceholderText("user name"), "readonly-team");
    await user.click(screen.getByRole("button", { name: /Add condition/ }));
    await user.clear(screen.getByLabelText("Condition 1 path"));
    await user.type(screen.getByLabelText("Condition 1 path"), "$.role");
    await user.type(screen.getByLabelText("Condition 1 regex"), "^viewer$");
    await user.click(screen.getByRole("button", { name: "Save user" }));

    await waitFor(() => expect(created).toHaveLength(1));
    expect(created[0]).toMatchObject({
      name: "readonly-team",
      match: [{ path: "$.role", pattern: "^viewer$" }],
    });
    await waitFor(() => expect(screen.getByRole("button", { name: "Delete user" })).toBeInTheDocument());
  });
});
