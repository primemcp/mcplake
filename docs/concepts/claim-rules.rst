Claim Rules
===========

Every decision the gateway makes about a caller is made by **claim rules**:
small tests against the claims in the caller's JWT. Access policies, filter
policies and admin authentication all use the same rules.

A rule
------

A rule has two parts:

``path``
   A `JSONPath <https://www.rfc-editor.org/rfc/rfc9535>`_ expression selecting
   a value from the token's claims, such as ``$.role``,
   ``$.groups[*]`` or ``$.realm_access.roles[*]``.

``pattern``
   A regular expression (Go ``regexp`` syntax, RE2) the selected value must
   match.

.. code-block:: toml

   [[access_policies.match]]
   path = "$.role"
   pattern = "^db-(reader|writer)$"

Given the claims

.. code-block:: json

   {
     "sub": "alice",
     "role": "db-reader",
     "groups": ["staff", "oncall-db"],
     "realm_access": { "roles": ["offline_access", "analyst"] },
     "tenant": 1000042
   }

these rules evaluate as follows:

.. list-table::
   :header-rows: 1
   :widths: 30 30 10 30

   * - ``path``
     - ``pattern``
     - Result
     - Why
   * - ``$.role``
     - ``^db-reader$``
     - match
     - exact string match
   * - ``$.role``
     - ``reader``
     - match
     - patterns are not anchored: this means "contains"
   * - ``$.groups[*]``
     - ``^oncall-``
     - match
     - any element of a list may match
   * - ``$.groups``
     - ``^staff$``
     - match
     - a path that selects a list is tested element by element too
   * - ``$.realm_access.roles[*]``
     - ``^admin$``
     - no match
     - no element matches
   * - ``$.department``
     - ``.*``
     - no match
     - a missing claim never matches, whatever the pattern
   * - ``$.tenant``
     - ``^1.000042e\+06$``
     - match
     - numbers are compared in Go's ``%v`` form — see below

Rules in detail
~~~~~~~~~~~~~~~

- **Anchor your patterns.** Matching is a regular-expression *search*.
  ``admin`` matches ``not-admin``; ``^admin$`` does not. Almost every rule
  should start with ``^`` and end with ``$``.
- **Lists match if any element matches.** Use ``[*]`` or select the list
  itself; both work.
- **A missing claim is simply no match**, not an error.
- **Non-string values are converted to strings before matching:** ``true`` /
  ``false`` for booleans, and numbers the way Go prints a ``float64`` — which
  switches to exponent form from 1,000,000 upward (``1000042`` becomes
  ``1.000042e+06``). Prefer string claims for identifiers; if you must match a
  large number, write the pattern for its exponent form.
- Invalid JSONPath or regular expressions are rejected when the configuration
  is loaded (the gateway refuses to start, naming the entry) or when a policy
  is saved through the admin API (``400 invalid_policy``).

Combining rules
---------------

A policy's ``match`` is a list of rules, and **all** of them must match (AND):

.. code-block:: toml

   [[access_policies]]
   name = "eu-analysts"
   [[access_policies.match]]
   path = "$.groups[*]"
   pattern = "^analysts$"
   [[access_policies.match]]
   path = "$.region"
   pattern = "^eu-"

For OR, write the alternatives into one pattern (``^(hr|payroll)$``), or create
two policies — access policies are additive, so a caller granted by either one
is granted.

.. warning::

   A policy with **no** ``match`` rules matches **every authenticated caller**.
   That is occasionally what you want (a tool everyone may use), but make it
   deliberate.

Choosing claims
---------------

The gateway can only use what your identity provider puts in the token. Most
providers can add group membership or application roles to access tokens —
for example Keycloak's realm or client roles (``$.realm_access.roles[*]``),
Entra ID's ``roles`` and ``groups`` claims, or a custom claim from an Auth0
action. Decide which claim expresses the distinction you need, then write
rules against its exact shape in a real token.

The admin web UI's **Request path** view simulates a request for a decoded
token payload you paste in, against the saved policies; see
:doc:`/features/admin-webui`.

Reference
---------

- :doc:`/architecture/decisions/0002-jsonpath-regexp-claim-rule-engine` — why
  JSONPath and regular expressions.
- :doc:`access-policies`, :doc:`response-filtering` and
  :doc:`authentication` — where rules are used.
