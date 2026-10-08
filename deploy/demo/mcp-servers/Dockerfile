# The demo's own remote MCP servers (project_tracker.py over Streamable HTTP,
# inventory.py over sse), each run as an ASGI app under uvicorn in its own
# container - see compose.yaml. One image serves both; compose picks the
# module and port per service.
#
# Build context is this directory. compose.yaml also bind-mounts it over
# /srv/mcp, so the scripts are editable without a rebuild, exactly like
# employee_directory.py in the gateway image.
FROM python:3.12-slim-bookworm
WORKDIR /srv/mcp
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
COPY project_tracker.py inventory.py ./
# Unprivileged: nothing here needs root, and it is a network listener.
USER nobody
# Overridden per service in compose.yaml.
CMD ["uvicorn", "project_tracker:app", "--host", "127.0.0.1", "--port", "8001"]
