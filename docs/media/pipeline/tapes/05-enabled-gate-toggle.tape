# Pipeline stage: the Enabled gate, after authorization
# (cache/registry.go's Enabled flag, gateway/internal/toolcall.go)
#
# mcplake-admin flips employee-directory off via PATCH /admin/mcps/:name
# (a reversible soft-disable, not the destructive DELETE). alice's next
# call -- same token, same tool, same grant -- now gets 403 mcp_disabled
# instead of 200. Re-enabling needs no reconnect: the registration, its
# cached tools and its live client were never touched.

Output ../out/05-enabled-gate-toggle.gif
Set Shell "bash"
Set FontSize 16
Set Width 1300
Set Height 750
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

Type "# alice, granted and enabled -- the ordinary case"
Enter
Type `STATUS=$(call_gateway "$ALICE" employee-directory list_departments '{}'); show_last_call "$STATUS"`
Enter
Sleep 2s

Type "# mcplake-admin disables the MCP -- reversible, keeps the cached schema"
Enter
Type `set_enabled "$ADMIN" employee-directory false`
Enter
Sleep 2s

Type "# same alice, same grant -- but the gate right after authorization now says no"
Enter
Type `STATUS=$(call_gateway "$ALICE" employee-directory list_departments '{}'); show_last_call "$STATUS"`
Enter
Sleep 2s

Type "# and re-enabling needs no reconnect"
Enter
Type `set_enabled "$ADMIN" employee-directory true`
Enter
Sleep 3s
