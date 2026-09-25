"""Demo MCP server reached over **Streamable HTTP** (the `http` transport).

The compose demo's other MCPs cover stdio (employee_directory.py, a
subprocess of the gateway) and sse (inventory.py, and the third-party
postgres-mcp). This one exists so the demo has an `http` endpoint too: its
own container, reached by the gateway over the network at
http://localhost:8001/mcp (ADR-0017).

It is an ASGI app, served by uvicorn rather than by FastMCP's own runner:

    uvicorn project_tracker:app --host 127.0.0.1 --port 8001

which is how you would deploy a real one - behind whatever ASGI server and
middleware the platform already runs. Data is in-memory and fictional.
"""

from fastmcp import FastMCP
from fastmcp.exceptions import ToolError
from pydantic import BaseModel

mcp = FastMCP("project-tracker")


class Task(BaseModel):
    id: int
    title: str
    status: str
    assignee: str


class Project(BaseModel):
    id: int
    name: str
    owner: str
    budget_usd: int
    tasks: list[Task]


class ProjectList(BaseModel):
    # A named field rather than a bare list, for the same reason as
    # employee_directory.py's EmployeeList: FastMCP wraps a bare list as
    # {"result": [...]} in structuredContent but not in the text copy, and a
    # filter path has to match both.
    projects: list[Project]


PROJECTS = [
    Project(
        id=1,
        name="Gateway rollout",
        owner="Alice Moreau",
        budget_usd=120_000,
        tasks=[
            Task(id=11, title="Pick an OIDC provider", status="done", assignee="Alice Moreau"),
            Task(id=12, title="Write access policies", status="in_progress", assignee="Bob Tanaka"),
        ],
    ),
    Project(
        id=2,
        name="Data warehouse migration",
        owner="Chen Wei",
        budget_usd=340_000,
        tasks=[
            Task(id=21, title="Inventory existing jobs", status="done", assignee="Chen Wei"),
            Task(id=22, title="Dual-write period", status="todo", assignee="Dana Okafor"),
        ],
    ),
]


@mcp.tool
def list_projects() -> ProjectList:
    """List every project with its tasks."""
    return ProjectList(projects=PROJECTS)


@mcp.tool
def get_project(project_id: int) -> Project:
    """Look up one project by id."""
    for project in PROJECTS:
        if project.id == project_id:
            return project
    raise ToolError(f"no project with id {project_id}")


@mcp.tool
def tasks_by_status(status: str) -> list[Task]:
    """List tasks across all projects with the given status (todo, in_progress, done)."""
    return [task for project in PROJECTS for task in project.tasks if task.status == status]


app = mcp.http_app(transport="http", path="/mcp")
