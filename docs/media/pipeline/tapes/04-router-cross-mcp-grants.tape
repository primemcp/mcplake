# Pipeline stage: ROUTER, per-MCP grants across two transports (ADR-0017)
#
# demo-reader grants alice employee-directory (stdio) and NOTHING else;
# admin's wildcard grant reaches every MCP, including inventory -- a
# second demo-owned MCP reached over sse, its own container. Same
# claim-driven router, same code path, deciding access on two completely
# different transports.
#
# Targets inventory, not demo-postgres: demo-postgres (crystaldba/
# postgres-mcp:0.3.0) cannot register at all in this environment -- the
# gateway's go-sdk client always probes `server/discover` (SEP-2575) on
# connect, and that image's SSE session crashes instead of answering with
# a JSON-RPC error, so even the legacy-initialize fallback fails on the
# same dead connection. See docs/media/pipeline/README.md's "Known
# issues" section. inventory is a FastMCP server the demo owns, on the
# same sse transport, and isn't affected.

Output ../out/04-router-cross-mcp-grants.gif
Set Shell "bash"
Set FontSize 16
Set Width 1300
Set Height 700
Set Theme "Dracula"
Set TypingSpeed 40ms

Type "source lib/env.sh"
Enter
Sleep 500ms

Type `ALICE=$(token_for alice)`
Enter
Type `ADMIN=$(token_for mcplake-admin)`
Enter
Sleep 500ms

Type "# alice: granted employee-directory only -- inventory is a stranger's MCP to her"
Enter
Type `STATUS=$(call_gateway "$ALICE" inventory list_items '{}'); show_last_call "$STATUS"`
Enter
Sleep 2s

Type "# mcplake-admin: wildcard grant reaches every MCP, stdio or sse alike"
Enter
Type `STATUS=$(call_gateway "$ADMIN" inventory list_items '{}'); show_last_call "$STATUS"`
Enter
Sleep 3s
