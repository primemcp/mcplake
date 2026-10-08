import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TransportPicker } from "./TransportPicker";

describe("TransportPicker", () => {
  it("offers every transport the gateway implements, all selectable", () => {
    render(<TransportPicker value="stdio" onChange={vi.fn()} />);

    for (const t of ["stdio", "http", "sse"]) {
      expect(screen.getByRole("radio", { name: t })).toBeEnabled();
    }
  });

  it("marks the current transport as checked and the others not", () => {
    render(<TransportPicker value="http" onChange={vi.fn()} />);

    expect(screen.getByRole("radio", { name: "http" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "stdio" })).not.toBeChecked();
    expect(screen.getByRole("radio", { name: "sse" })).not.toBeChecked();
  });

  it("reports the transport that was clicked", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<TransportPicker value="stdio" onChange={onChange} />);

    await user.click(screen.getByRole("radio", { name: "sse" }));

    expect(onChange).toHaveBeenCalledWith("sse");
  });

  it("describes what each transport is, rather than why it cannot be used", () => {
    render(<TransportPicker value="stdio" onChange={vi.fn()} />);

    // It used to caption sse/http "Not implemented by the gateway yet".
    // ADR-0017 implemented both, so a tooltip saying so would be wrong.
    expect(screen.getByRole("radio", { name: "http" })).toHaveAttribute(
      "title",
      expect.stringContaining("Streamable HTTP"),
    );
    for (const t of ["stdio", "http", "sse"]) {
      expect(screen.getByRole("radio", { name: t })).not.toHaveAttribute(
        "title",
        expect.stringContaining("Not implemented"),
      );
    }
  });

  it("disables the whole control while a save is in flight", () => {
    render(<TransportPicker value="stdio" onChange={vi.fn()} disabled />);

    for (const t of ["stdio", "http", "sse"]) {
      expect(screen.getByRole("radio", { name: t })).toBeDisabled();
    }
  });
});
