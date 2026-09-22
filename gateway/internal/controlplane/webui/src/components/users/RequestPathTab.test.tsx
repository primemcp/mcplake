import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { AccessPolicy, FilterPolicy, MCPRegistration } from "../../api/types";
import type { User } from "../../api/users";
import { RequestPathTab } from "./RequestPathTab";

const memory: MCPRegistration = {
  name: "demo-memory",
  transport: "stdio",
  connect: { command: "memory" },
  status: "active",
  enabled: true,
  tools: {
    add_observations: { name: "add_observations" },
    delete_entities: { name: "delete_entities" },
  },
};

const analytics: MCPRegistration = {
  name: "analytics",
  transport: "stdio",
  connect: { command: "analytics" },
  status: "active",
  enabled: true,
  tools: { get_report: { name: "get_report" } },
};

const piiFilter: FilterPolicy = {
  name: "analyst-team::Mask PII::add_observations",
  match: [{ path: "$.role", pattern: "^analyst$" }],
  mcp: "demo-memory",
  tool: "add_observations",
  drop_fields: ["$.results[*].email", "$.id"],
  enabled: true,
};

const analyst: User = {
  name: "analyst-team",
  match: [{ path: "$.role", pattern: "^analyst$" }],
  grants: [{ mcp: "demo-memory", tools: ["add_observations"] }],
  filters: [piiFilter],
};

const analystPolicy: AccessPolicy = { name: analyst.name, match: analyst.match, grants: analyst.grants, enabled: true };

function baseProps() {
  return {
    user: analyst,
    endpoints: [memory, analytics],
    allAccessPolicies: [analystPolicy],
    allFilters: [piiFilter],
    draftDirty: false,
    onGoAccess: vi.fn(),
    onGoInstances: vi.fn(),
  };
}

async function replacePayload(user: ReturnType<typeof userEvent.setup>, text: string) {
  const box = screen.getByLabelText("Decoded JWT payload");
  await user.clear(box);
  // userEvent.type treats `{`/`[` as key descriptors -- paste avoids that.
  await user.click(box);
  await user.paste(text);
}

describe("RequestPathTab", () => {
  it("opens on the user's first granted endpoint and tool, with a payload that satisfies their conditions", () => {
    render(<RequestPathTab {...baseProps()} />);

    expect(screen.getByLabelText("Endpoint")).toHaveValue("demo-memory");
    expect(screen.getByLabelText("Tool")).toHaveValue("add_observations");
    expect(JSON.parse((screen.getByLabelText("Decoded JWT payload") as HTMLTextAreaElement).value)).toMatchObject({
      role: "analyst",
    });
  });

  it("says plainly that this is a simulation against saved policies, not a live trace", () => {
    render(<RequestPathTab {...baseProps()} />);

    expect(screen.getByText(/simulation against the policies as currently saved/i)).toBeInTheDocument();
    expect(screen.queryByText(/unsaved changes/i)).not.toBeInTheDocument();
  });

  it("warns when the panel has unsaved edits the simulation can't see", () => {
    render(<RequestPathTab {...baseProps()} draftDirty />);

    expect(screen.getByText(/unsaved changes/i)).toBeInTheDocument();
  });

  it("a granted call: the trace passes every stage and lists the fields the matching filter drops", () => {
    render(<RequestPathTab {...baseProps()} />);

    const detail = within(screen.getByTestId("path-detail"));
    expect(detail.getByTestId("path-badge")).toHaveTextContent("200");
    expect(detail.getByTestId("path-badge")).not.toHaveClass("text-danger");
    expect(detail.getByText("Auth Validator").nextSibling).toHaveTextContent(/verified/);
    expect(detail.getByText("Access Check").nextSibling).toHaveTextContent(/granted/);
    expect(detail.getByText("Response Filter").nextSibling).toHaveTextContent(/2 fields dropped/);

    // The Access Check node is the one "deciding" on the granted path --
    // accent, not alert.
    expect(screen.getByRole("button", { name: "Access Check" })).not.toHaveClass("border-danger");
  });

  it("a call the user has no grant for is denied at Access Check with the alert treatment", async () => {
    const user = userEvent.setup();
    render(<RequestPathTab {...baseProps()} />);

    await user.selectOptions(screen.getByLabelText("Tool"), "delete_entities");

    const badge = within(screen.getByTestId("path-detail")).getByTestId("path-badge");
    expect(badge).toHaveTextContent("403");
    expect(badge).toHaveClass("text-danger");
    expect(badge).toHaveClass("bg-danger-bg");

    const access = screen.getByRole("button", { name: "Access Check" });
    expect(access).toHaveClass("border-danger");
    expect(access.style.animation).toContain("rejectpulse");

    const detail = within(screen.getByTestId("path-detail"));
    expect(detail.getByText("Access Check").nextSibling).toHaveTextContent(/403/);
    expect(detail.getByText("Response Filter").nextSibling).toHaveTextContent(/skipped/);
  });

  it("claims that match no policy at all are denied too, and the Access Check detail says which policies didn't match", async () => {
    const user = userEvent.setup();
    render(<RequestPathTab {...baseProps()} />);

    await replacePayload(user, JSON.stringify({ role: "guest" }));
    await user.click(screen.getByRole("button", { name: "Access Check" }));

    const detail = within(screen.getByTestId("path-detail"));
    expect(detail.getByTestId("path-badge")).toHaveTextContent(/403/);
    expect(detail.getByText("analyst-team").nextSibling).toHaveTextContent(/no match/);
  });

  it("a payload that isn't a JSON object is rejected at Auth Validator with 401, and nothing downstream runs", async () => {
    const user = userEvent.setup();
    render(<RequestPathTab {...baseProps()} />);

    await replacePayload(user, "{not json");

    const detail = within(screen.getByTestId("path-detail"));
    expect(detail.getByTestId("path-badge")).toHaveTextContent("401");
    expect(detail.getByTestId("path-badge")).toHaveClass("text-danger");
    expect(detail.getByText("Auth Validator").nextSibling).toHaveTextContent(/401/);
    expect(detail.getByText("Access Check").nextSibling).toHaveTextContent(/skipped/);

    const auth = screen.getByRole("button", { name: "Auth Validator" });
    expect(auth).toHaveClass("border-danger");
    expect(auth.style.animation).toContain("rejectpulse");
  });

  it("selecting the Response Filter node shows which filters ran and every dropped field", async () => {
    const user = userEvent.setup();
    render(<RequestPathTab {...baseProps()} />);

    await user.click(screen.getByRole("button", { name: "Response Filter" }));

    const detail = within(screen.getByTestId("path-detail"));
    expect(detail.getByText("Mask PII").nextSibling).toHaveTextContent(/applied/);
    expect(detail.getByText("$.results[*].email")).toBeInTheDocument();
    expect(detail.getByText("$.id")).toBeInTheDocument();
  });

  it("selecting an endpoint node explains the user's grant on it and links to the Access tab", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<RequestPathTab {...props} />);

    await user.click(screen.getByRole("button", { name: "MCP endpoint analytics" }));

    const detail = within(screen.getByTestId("path-detail"));
    expect(detail.getByTestId("path-badge")).toHaveTextContent(/not granted/);
    await user.click(detail.getByRole("button", { name: /Grant access/ }));
    expect(props.onGoAccess).toHaveBeenCalled();
  });

  it("with no endpoints registered, shows the empty state instead of a simulation", () => {
    render(<RequestPathTab {...baseProps()} endpoints={[]} />);

    expect(screen.getByText("No MCP endpoints")).toBeInTheDocument();
    expect(screen.queryByLabelText("Endpoint")).not.toBeInTheDocument();
  });
  it("editing an input snaps the detail panel back to the request trace, so a new outcome is never hidden behind a stale node selection", async () => {
    const user = userEvent.setup();
    render(<RequestPathTab {...baseProps()} />);

    await user.click(screen.getByRole("button", { name: "Response Filter" }));
    expect(within(screen.getByTestId("path-detail")).getByText("Response Filter")).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText("Tool"), "delete_entities");

    const detail = within(screen.getByTestId("path-detail"));
    expect(detail.getByText("Denied request")).toBeInTheDocument();
    expect(detail.getByTestId("path-badge")).toHaveTextContent("403");
  });
});

describe("RequestPathTab: rules the simulator cannot evaluate", () => {
  const unsupported = { path: "$.resource_access[?(@.roles)]", pattern: "gateway-admin" };

  it("says so plainly instead of claiming the gateway would fail the request", async () => {
    const props = baseProps();
    render(
      <RequestPathTab
        {...props}
        allAccessPolicies={[
          { name: "keycloak-roles", match: [unsupported], grants: [], enabled: true },
          analystPolicy,
        ]}
      />,
    );

    const caveat = await screen.findByTestId("path-incomplete");
    expect(caveat).toHaveTextContent(/could not be simulated/i);
    expect(caveat).toHaveTextContent("keycloak-roles");
    expect(screen.queryByText(/fails the request rather than guess/)).not.toBeInTheDocument();
  });

  // The whole point of the fix: one unreadable policy must not hide the
  // real outcome that the other policies produce.
  it("still reports the outcome the evaluable policies produce", () => {
    const props = baseProps();
    render(
      <RequestPathTab
        {...props}
        allAccessPolicies={[
          { name: "keycloak-roles", match: [unsupported], grants: [], enabled: true },
          analystPolicy,
        ]}
      />,
    );

    expect(within(screen.getByTestId("path-detail")).getByTestId("path-badge")).toHaveTextContent("200");
  });

  it("shows no caveat when every rule is evaluable", () => {
    render(<RequestPathTab {...baseProps()} />);

    expect(screen.queryByTestId("path-incomplete")).not.toBeInTheDocument();
  });
});
