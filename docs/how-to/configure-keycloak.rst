Configure Keycloak as the Identity Provider
===========================================

This guide sets up a Keycloak realm so that its access tokens carry what the
gateway needs: the right audience, and claims your policies can match. The
same ideas apply to any OIDC provider; the menu names below are Keycloak 26's.

What the gateway needs from a token
-----------------------------------

- ``iss`` — the realm's issuer, ``https://<keycloak>/realms/<realm>``.
- ``aud`` — a value you choose for the gateway, e.g. ``mcplake``.
- One or more claims describing who the caller is, for your policies: realm
  roles, groups, or both.

1. Create the realm and roles or groups
---------------------------------------

Create a realm (or use an existing one), then model access with **realm roles**
or **groups** — both are managed in Keycloak's own UI and need no custom user
attributes:

- Realm roles such as ``mcplake-admin``, ``analysts``, ``platform``. Keycloak
  puts them in every access token under ``realm_access.roles``.
- Groups such as ``/engineering`` — these need the mapper in step 3.

Assign users (and service accounts, below) to them.

.. tip::

   Prefer roles or groups over custom user attributes. Keycloak 26's declared
   user profile manages which attributes users may have, and an attribute
   outside the profile schema can block sign-in with an "Account is not
   fully set up" error — the demo hit exactly this with its ``role``
   attribute (:issue:`209`) and works around it by disabling the
   ``VERIFY_PROFILE`` required action in
   :repo:`deploy/demo/keycloak/mcplake-realm.json` rather than declaring a
   full user-profile schema for three fixed demo users.

2. Add the gateway's audience
-----------------------------

The gateway does not sign anyone in on the data plane; it only needs tokens
whose ``aud`` names it. Create an **audience mapper** on every client that will
request tokens for the gateway (step 4 and 5), or once on a shared client scope
assigned to them:

- *Client scopes* → *Create client scope* ``mcplake-audience`` (type *Default*).
- In it, *Mappers* → *Configure a new mapper* → **Audience**:
  *Included Custom Audience* ``mcplake``, *Add to access token* on.

3. Put groups in the token (optional)
-------------------------------------

If your policies match groups, add a mapper to the same client scope:

- *Configure a new mapper* → **Group Membership**: *Token Claim Name*
  ``groups``, *Full group path* **off**, *Add to access token* on.

Tokens then carry ``"groups": ["engineering", ...]``, matched with
``path = "$.groups[*]"``.

4. A client for agents
----------------------

For an agent or service that calls tools on its own behalf:

- *Clients* → *Create client*, e.g. ``reporting-agent``.
- *Client authentication* **on**, *Service accounts roles* **on**, other flows
  off.
- *Client scopes*: add ``mcplake-audience``.
- *Service account roles*: assign the realm roles the agent should have.

The agent obtains tokens with the client-credentials grant:

.. code-block:: bash

   curl -s -X POST https://sso.example.com/realms/main/protocol/openid-connect/token \
     -d grant_type=client_credentials \
     -d client_id=reporting-agent -d client_secret="$SECRET" | jq -r .access_token

For tools used by people (an editor, a desktop assistant), have the person's
own sign-in produce the token, so the gateway sees *their* roles.

5. A client for the admin web UI
--------------------------------

- *Clients* → *Create client* ``mcplake-admin-ui``.
- *Client authentication* **off** (a public client), *Standard flow* on, all
  other flows off.
- *Valid redirect URIs*: the URL operators open, with a trailing slash — for
  example ``https://gateway.internal:8081/``. *Web origins*: the same origin.
- *Advanced* → *Proof Key for Code Exchange Code Challenge Method*: ``S256``.
- *Client scopes*: add ``mcplake-audience``.

6. Configure the gateway
------------------------

.. code-block:: toml

   [oidc]
   jwks_url = "https://sso.example.com/realms/main/protocol/openid-connect/certs"
   issuer = "https://sso.example.com/realms/main"
   audience = "mcplake"

   [[admin_auth.match]]
   path = "$.realm_access.roles[*]"
   pattern = "^mcplake-admin$"

   [admin_auth.login]
   client_id = "mcplake-admin-ui"
   authorization_endpoint = "https://sso.example.com/realms/main/protocol/openid-connect/auth"
   token_endpoint = "https://sso.example.com/realms/main/protocol/openid-connect/token"

and write policies against the claims:

.. code-block:: toml

   [[access_policies]]
   name = "analysts-query"
   [[access_policies.match]]
   path = "$.realm_access.roles[*]"
   pattern = "^analysts$"
   [[access_policies.grants]]
   mcp = "postgres-ro"
   tools = ["*"]

Check a token
-------------

Decode a token (the middle, base64url-encoded segment) and confirm the
``iss``, ``aud`` and role or group claims are what your configuration
expects:

.. code-block:: bash

   echo "$TOKEN" | cut -d. -f2 | tr '_-' '/+' | base64 -d 2>/dev/null | jq '{iss, aud, realm_access, groups}'

A working example
-----------------

The demo imports a complete realm, :repo:`deploy/demo/keycloak/mcplake-realm.json`,
with an audience mapper and a ``role`` claim — see
:doc:`/getting-started/demo`.
