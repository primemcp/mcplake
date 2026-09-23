"""Dummy MCP server for the mcplake compose demo.

Deliberately not a passthrough over a real datastore (contrast with the
official postgres reference server this replaced — see docs/DEMO.md):
several tools, each with its own declared response shape, exist
specifically so filter_policies (ADR-0015) has real per-field structure to
strip from. Seed data is in-memory and fictional; nothing here talks to
Postgres — the demo's Postgres container backs the *gateway's own*
[persistence], not this MCP (see deploy/demo/config.toml).

Run over stdio (FastMCP's default transport), so this runs as a subprocess
inside the gateway's own container. That follows from the transport, not
from a limit of the gateway: since ADR-0017 mcp.Client also speaks http and
sse, and the demo's other MCP (postgres-mcp, see compose.yaml) is a
separate container reached over the network. Having one of each is the
point — the gateway's behaviour does not vary by transport.
"""

from fastmcp import FastMCP
from fastmcp.exceptions import ToolError
from pydantic import BaseModel

mcp = FastMCP("employee-directory")


class Employee(BaseModel):
    id: int
    name: str
    department: str
    title: str
    email: str
    # Sensitive fields drop_fields strips for role=db-reader in
    # deploy/demo/config.toml's filter_policies, but not for role=admin.
    salary_usd: int
    ssn_last4: str


class EmployeeList(BaseModel):
    employees: list[Employee]


EMPLOYEES: list[Employee] = [
    Employee(id=1, name="Priya Natarajan", department="Engineering", title="Staff Engineer",
             email="priya.natarajan@example.com", salary_usd=185000, ssn_last4="4821"),
    Employee(id=2, name="Marcus Webb", department="Engineering", title="Engineering Manager",
             email="marcus.webb@example.com", salary_usd=198000, ssn_last4="0193"),
    Employee(id=3, name="Sofia Reyes", department="Sales", title="Account Executive",
             email="sofia.reyes@example.com", salary_usd=112000, ssn_last4="7745"),
    Employee(id=4, name="Tomasz Wojcik", department="Sales", title="VP of Sales",
             email="tomasz.wojcik@example.com", salary_usd=210000, ssn_last4="3360"),
    Employee(id=5, name="Aiko Tanaka", department="Finance", title="Financial Analyst",
             email="aiko.tanaka@example.com", salary_usd=98000, ssn_last4="9012"),
]


@mcp.tool
def list_employees() -> EmployeeList:
    """List every employee in the directory.

    Returns a named `employees` field rather than a bare list: FastMCP
    wraps a bare `list[Model]` return as {"result": [...]} in
    structuredContent but leaves the human-readable text copy of the same
    result as a bare array — two different shapes for the same data, one
    of which a drop_fields path like "$.result[*].ssn_last4" would silently
    fail to match, leaking the field through the copy it didn't cover. An
    explicit wrapper model keeps both copies identically shaped, so one
    path (deploy/demo/config.toml's "$.employees[*].ssn_last4") strips it
    from both.
    """
    return EmployeeList(employees=EMPLOYEES)


@mcp.tool
def get_employee(employee_id: int) -> Employee:
    """Look up one employee by id."""
    for employee in EMPLOYEES:
        if employee.id == employee_id:
            return employee
    raise ToolError(f"no employee with id {employee_id}")


@mcp.tool
def list_departments() -> list[str]:
    """List every department name. No sensitive fields here — nothing for
    a filter_policies entry to strip, unlike the two tools above."""
    seen: list[str] = []
    for employee in EMPLOYEES:
        if employee.department not in seen:
            seen.append(employee.department)
    return seen


if __name__ == "__main__":
    mcp.run()
