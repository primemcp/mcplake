"""Demo MCP server reached over the legacy **HTTP+SSE** transport (`sse`).

The counterpart to project_tracker.py, which covers Streamable HTTP. Many
MCP servers in the wild still only speak sse, so the demo keeps one it owns
alongside the third-party postgres-mcp: its own container, reached by the
gateway at http://localhost:8002/sse (ADR-0017).

An ASGI app served by uvicorn:

    uvicorn inventory:app --host 127.0.0.1 --port 8002

Data is in-memory and fictional.
"""

from fastmcp import FastMCP
from fastmcp.exceptions import ToolError
from pydantic import BaseModel

mcp = FastMCP("inventory")


class Item(BaseModel):
    sku: str
    name: str
    warehouse: str
    quantity: int
    unit_cost_usd: float


class ItemList(BaseModel):
    # Named field, not a bare list - see project_tracker.py's ProjectList.
    items: list[Item]


ITEMS = [
    Item(sku="KB-101", name="Mechanical keyboard", warehouse="Rotterdam", quantity=42, unit_cost_usd=61.5),
    Item(sku="MS-220", name="Wireless mouse", warehouse="Rotterdam", quantity=0, unit_cost_usd=18.0),
    Item(sku="MN-270", name="27-inch monitor", warehouse="Leipzig", quantity=13, unit_cost_usd=189.0),
    Item(sku="DK-310", name="USB-C dock", warehouse="Leipzig", quantity=7, unit_cost_usd=94.25),
]


@mcp.tool
def list_items() -> ItemList:
    """List every stocked item."""
    return ItemList(items=ITEMS)


@mcp.tool
def get_item(sku: str) -> Item:
    """Look up one item by SKU."""
    for item in ITEMS:
        if item.sku == sku:
            return item
    raise ToolError(f"no item with sku {sku}")


@mcp.tool
def out_of_stock() -> list[str]:
    """List the SKUs with no stock left."""
    return [item.sku for item in ITEMS if item.quantity == 0]


app = mcp.http_app(transport="sse", path="/sse")
