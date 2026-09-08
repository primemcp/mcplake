import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { ClaimRule } from "../../api/types";
import { TokenMatchTab } from "./TokenMatchTab";

function StatefulTokenMatchTab({ initial }: { initial: ClaimRule[] }) {
  const [match, setMatch] = useState(initial);
  return <TokenMatchTab match={match} onChange={setMatch} />;
}

describe("TokenMatchTab", () => {
  it("shows the empty-state warning when there are no conditions yet", () => {
    render(<TokenMatchTab match={[]} onChange={vi.fn()} />);
    expect(screen.getByText(/No condition yet/)).toBeInTheDocument();
  });

  it("adds a blank condition", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<TokenMatchTab match={[]} onChange={onChange} />);

    await user.click(screen.getByRole("button", { name: /Add condition/ }));

    expect(onChange).toHaveBeenCalledWith([{ path: "$.", pattern: "" }]);
  });

  it("edits a condition's path and regex independently", async () => {
    const user = userEvent.setup();
    render(<StatefulTokenMatchTab initial={[{ path: "$.role", pattern: "" }]} />);

    await user.type(screen.getByLabelText("Condition 1 regex"), "^analyst$");

    expect(screen.getByLabelText("Condition 1 path")).toHaveValue("$.role");
    expect(screen.getByLabelText("Condition 1 regex")).toHaveValue("^analyst$");
  });

  it("removes a condition", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <TokenMatchTab
        match={[
          { path: "$.role", pattern: "^analyst$" },
          { path: "$.org", pattern: "^acme$" },
        ]}
        onChange={onChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Remove condition 1" }));

    expect(onChange).toHaveBeenCalledWith([{ path: "$.org", pattern: "^acme$" }]);
  });

  it("shows the live condition count", () => {
    render(
      <TokenMatchTab
        match={[
          { path: "$.role", pattern: "^analyst$" },
          { path: "$.org", pattern: "^acme$" },
        ]}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByText("2 · AND")).toBeInTheDocument();
  });
});
