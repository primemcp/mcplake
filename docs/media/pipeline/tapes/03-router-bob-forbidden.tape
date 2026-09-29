# Pipeline stage: ROUTER / access policy deny path (router/router.go,
# gateway/internal/toolcall.go)
#
# bob authenticates fine -- Keycloak hands him a perfectly valid token --
# but role=guest matches no [[access_policies]] entry in
# deploy/demo/config.toml, so the pipeline stops at authorization and
# never reaches the MCP at all. 403 forbidden, same tool alice can call.

Output ../out/03-router-bob-forbidden.gif
Set Shell "bash"
Set FontSize 18
Set Width 1200
Set Height 600
Set Theme "Dracula"
Set TypingSpeed 40ms

Type "source lib/env.sh"
Enter
Sleep 500ms

Type `BOB=$(token_for bob); role_claim_of "$BOB"`
Enter
Sleep 1s

Type "# a perfectly valid token -- just not one any access_policy grants"
Enter
Type `STATUS=$(call_gateway "$BOB" employee-directory get_employee '{"employee_id":2}'); show_last_call "$STATUS"`
Enter
Sleep 3s
