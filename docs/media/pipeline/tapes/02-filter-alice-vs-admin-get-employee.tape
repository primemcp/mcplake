# Pipeline stage: FILTER (filter/filter.go, ADR-0015)
#
# Same MCP, same tool, same employee -- called as alice (role=db-reader)
# then as mcplake-admin (role=admin). Both get 200; alice's response has
# salary_usd/ssn_last4 stripped by hide-sensitive-fields-get-employee,
# admin's doesn't, because that filter_policy's match rule never fires for
# role=admin. The side-by-side IS the point: filtering is claims-driven,
# not a per-user flag.

Output ../out/02-filter-alice-vs-admin-get-employee.gif
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

Type "# alice -- role=db-reader"
Enter
Type `STATUS=$(call_gateway "$ALICE" employee-directory get_employee '{"employee_id":2}'); show_last_call "$STATUS"`
Enter
Sleep 2s

Type "# mcplake-admin -- role=admin, same tool, same employee"
Enter
Type `STATUS=$(call_gateway "$ADMIN" employee-directory get_employee '{"employee_id":2}'); show_last_call "$STATUS"`
Enter
Sleep 3s
