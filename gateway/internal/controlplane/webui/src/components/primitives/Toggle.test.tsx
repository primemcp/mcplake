import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { Toggle } from "./Toggle";

function ControlledToggle() {
  const [checked, setChecked] = useState(false);
  return <Toggle checked={checked} onChange={setChecked} label={checked ? "on" : "off"} />;
}

describe("Toggle", () => {
  it("flips and reports its new state on click", async () => {
    const user = userEvent.setup();
    render(<ControlledToggle />);

    const toggle = screen.getByRole("switch");
    expect(toggle).toHaveAttribute("aria-checked", "false");
    expect(screen.getByText("off")).toBeInTheDocument();

    await user.click(toggle);

    expect(toggle).toHaveAttribute("aria-checked", "true");
    expect(screen.getByText("on")).toBeInTheDocument();
  });
});
