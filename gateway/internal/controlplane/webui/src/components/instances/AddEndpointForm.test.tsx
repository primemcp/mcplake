import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AddEndpointForm } from "./AddEndpointForm";

describe("AddEndpointForm", () => {
  it("disables Add until both name and url are filled", async () => {
    const user = userEvent.setup();
    render(<AddEndpointForm onCreate={vi.fn()} onCancel={vi.fn()} />);

    const addButton = screen.getByRole("button", { name: "Add" });
    expect(addButton).toBeDisabled();

    await user.type(screen.getByPlaceholderText("display name"), "postgres-ro");
    expect(addButton).toBeDisabled();

    await user.type(screen.getByPlaceholderText("https://… or lambda arn"), "https://pg.internal");
    expect(addButton).toBeEnabled();
  });

  it("calls onCreate with trimmed name/url on submit", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<AddEndpointForm onCreate={onCreate} onCancel={vi.fn()} />);

    await user.type(screen.getByPlaceholderText("display name"), "  postgres-ro  ");
    await user.type(screen.getByPlaceholderText("https://… or lambda arn"), "  https://pg.internal  ");
    await user.click(screen.getByRole("button", { name: "Add" }));

    expect(onCreate).toHaveBeenCalledWith("postgres-ro", "https://pg.internal");
  });

  it("shows an inline error instead of throwing when onCreate rejects", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockRejectedValue(new Error("name already registered"));
    render(<AddEndpointForm onCreate={onCreate} onCancel={vi.fn()} />);

    await user.type(screen.getByPlaceholderText("display name"), "postgres-ro");
    await user.type(screen.getByPlaceholderText("https://… or lambda arn"), "https://pg.internal");
    await user.click(screen.getByRole("button", { name: "Add" }));

    expect(await screen.findByText("name already registered")).toBeInTheDocument();
  });
});
