import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { Tabs } from "./Tabs";

const TABS = [
  { id: "match", label: "Token match" },
  { id: "access", label: "Access" },
];

function TabbedPanel() {
  const [activeId, setActiveId] = useState("match");
  return (
    <div>
      <Tabs tabs={TABS} activeId={activeId} onChange={setActiveId} />
      <div>{activeId === "match" ? "Match panel" : "Access panel"}</div>
    </div>
  );
}

describe("Tabs", () => {
  it("switches the active panel on click", async () => {
    const user = userEvent.setup();
    render(<TabbedPanel />);

    expect(screen.getByText("Match panel")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Token match" })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    await user.click(screen.getByRole("tab", { name: "Access" }));

    expect(screen.getByText("Access panel")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Access" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Token match" })).toHaveAttribute(
      "aria-selected",
      "false",
    );
  });
});
