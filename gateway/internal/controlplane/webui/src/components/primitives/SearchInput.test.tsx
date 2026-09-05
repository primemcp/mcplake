import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useMemo, useState } from "react";
import { describe, expect, it } from "vitest";
import { SearchInput } from "./SearchInput";

const ITEMS = ["postgres-ro", "postgres-rw", "filesystem"];

function SearchableList() {
  const [query, setQuery] = useState("");
  const hits = useMemo(
    () => ITEMS.filter((item) => item.toLowerCase().includes(query.toLowerCase())),
    [query],
  );
  return (
    <div>
      <SearchInput value={query} onChange={setQuery} placeholder="Search" />
      <ul>
        {hits.map((item) => (
          <li key={item}>{item}</li>
        ))}
      </ul>
    </div>
  );
}

describe("SearchInput", () => {
  it("narrows a list as the query changes", async () => {
    const user = userEvent.setup();
    render(<SearchableList />);

    expect(screen.getAllByRole("listitem")).toHaveLength(3);

    await user.type(screen.getByPlaceholderText("Search"), "postgres");
    expect(screen.getAllByRole("listitem").map((el) => el.textContent)).toEqual([
      "postgres-ro",
      "postgres-rw",
    ]);
  });

  it("clears the query via the clear button once there's something to clear", async () => {
    const user = userEvent.setup();
    render(<SearchableList />);

    await user.type(screen.getByPlaceholderText("Search"), "filesystem");
    expect(screen.getAllByRole("listitem")).toHaveLength(1);

    await user.click(screen.getByRole("button", { name: "Clear search" }));
    expect(screen.getAllByRole("listitem")).toHaveLength(3);
  });
});
