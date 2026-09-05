import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TransportPicker } from "./TransportPicker";

describe("TransportPicker", () => {
  it("shows stdio enabled and sse/http visibly disabled, not hidden", () => {
    render(<TransportPicker />);

    expect(screen.getByRole("button", { name: "stdio" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "sse" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "http" })).toBeDisabled();
  });

  it("explains why sse/http are disabled via a title tooltip", () => {
    render(<TransportPicker />);

    expect(screen.getByRole("button", { name: "sse" })).toHaveAttribute(
      "title",
      expect.stringContaining("Not implemented"),
    );
  });
});
