import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Modal } from "./Modal";

describe("Modal", () => {
  it("renders nothing when closed", () => {
    render(
      <Modal open={false} onClose={vi.fn()} title="Add MCP endpoint">
        <p>content</p>
      </Modal>,
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("renders its content and title when open", () => {
    render(
      <Modal open onClose={vi.fn()} title="Add MCP endpoint">
        <p>content</p>
      </Modal>,
    );
    expect(screen.getByRole("dialog", { name: "Add MCP endpoint" })).toBeInTheDocument();
    expect(screen.getByText("content")).toBeInTheDocument();
  });

  it("closes on backdrop click but not on a click inside the dialog", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="Add MCP endpoint">
        <p>content</p>
      </Modal>,
    );

    await user.click(screen.getByText("content"));
    expect(onClose).not.toHaveBeenCalled();

    await user.click(screen.getByRole("dialog").parentElement!);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("closes on the close button and on Escape", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="Add MCP endpoint">
        <p>content</p>
      </Modal>,
    );

    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledOnce();

    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
