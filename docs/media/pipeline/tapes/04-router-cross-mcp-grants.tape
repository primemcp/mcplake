# Pipeline stage: ROUTER, per-MCP grants across two transports (ADR-0017)
#
# demo-reader grants alice employee-directory (stdio) and NOTHING else;
# admin's wildcard grant reaches every MCP, including demo-postgres
# (sse, a real server in its own container). Same claim-driven router,
# same code path, deciding access on two completely different transports.

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

Type "# alice: granted employee-directory only -- demo-postgres is a stranger's MCP to her"
Enter
Type `STATUS=$(call_gateway "$ALICE" demo-postgres list_schemas '{}'); show_last_call "$STATUS"`
Enter
Sleep 2s

Type "# mcplake-admin: wildcard grant reaches every MCP, stdio or sse alike"
Enter
Type `STATUS=$(call_gateway "$ADMIN" demo-postgres list_schemas '{}'); show_last_call "$STATUS"`
Enter
Sleep 3s
