# Pipeline stage: AUTH (auth/validator.go)
#
# Three demo users authenticate against the same Keycloak realm; the
# `role` claim each token carries is the one thing router/filter act on
# downstream. Shows that alice/bob/mcplake-admin are just three JWTs that
# differ in exactly one claim -- nothing else about the pipeline branches
# on identity, only on claims.

Output ../out/01-auth-three-tokens.gif
Set Shell "bash"
Set FontSize 18
Set Width 1200
Set Height 650
Set Theme "Dracula"
Set TypingSpeed 40ms

Type "source lib/env.sh"
Enter
Sleep 500ms

Type `ALICE=$(token_for alice); role_claim_of "$ALICE"`
Enter
Sleep 1s

Type `BOB=$(token_for bob); role_claim_of "$BOB"`
Enter
Sleep 1s

Type `ADMIN=$(token_for mcplake-admin); role_claim_of "$ADMIN"`
Enter
Sleep 2s
