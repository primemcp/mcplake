ADR-0020: Serve the Admin UI From Its Own Container
===================================================

:Status: Accepted
:Date: 2026-09-23

Context
-------

The admin SPA is built by a ``bun`` stage in ``deploy/demo/Dockerfile`` and
``go:embed``-ed into the gateway binary, which serves it from the control-plane
listener (``RegisterUIRoutes`` in ``webui.go``). That has one genuine virtue — the
gateway is a single self-contained artifact, and the UI is on the same origin as
the API it calls — and two costs:

- Any change to the frontend rebuilds the Go image, because the embedded ``dist/``
  is an input to ``go build``.
- The Go toolchain stage sits behind a ``bun install`` it does not otherwise need.

We want the SPA to ship as its own prebuilt image: compiled at image build time,
with the running container holding the assets and nothing that produced them.

The constraint that shapes the whole decision
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The UI calls the admin API with **relative** URLs (``fetch("/admin/mcps")``), and
:doc:`ADR-0014 </architecture/decisions/0014-admin-ui-oidc-pkce-login>`'s PKCE redirect URI names whatever
origin the operator browsed to.

Serve the SPA from a different origin than ``/admin/*`` and both break at once:

- every admin API call becomes cross-origin, which means a CORS allowlist on
  ``/admin/*`` — the most sensitive surface in the system, and one
  :doc:`ADR-0010 </architecture/decisions/0010-control-plane-admin-authentication>` exists to keep narrow;
- the bundle needs a configurable API base URL, which is a new piece of runtime
  configuration in a static artifact that has none today;
- the OIDC client needs a second registered redirect URI, and the UI needs to
  know which one it is.

None of that is worth paying to move a directory of files.

Decision
--------

**Ship the admin UI as its own image whose final layer is
``gcr.io/distroless/static``, containing one static Go binary and the built
``dist/`` — and make that binary the browser's single origin** by serving the
assets and reverse-proxying the admin API prefix to the gateway.

Three stages:

1. ``oven/bun:1`` runs ``bun run build``.
2. ``golang:1.27`` compiles ``cmd/webui`` with ``CGO_ENABLED=0``.
3. ``gcr.io/distroless/static-debian12:nonroot`` receives only the built assets
   and the binary.

The proxy lives in the same binary
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The alternative was a third container running nginx or Caddy in front of both.
One binary is better here: it keeps the demo at one added service rather than
two, avoids a second configuration language for what is a single route rule, and
``net/http/httputil.ReverseProxy`` is a few lines. It also means the thing that
knows the API prefix is the thing that serves the app that calls it.

Exactly one configured prefix is forwarded, never a pattern derived from the
request. An open proxy in front of an admin API would be a far bigger hole than
the origin split it was introduced to close.

Distroless, specifically
~~~~~~~~~~~~~~~~~~~~~~~~

This container is reachable from a browser *and* forwards to the admin API, so
it is the most exposed thing in the deployment. ``gcr.io/distroless/static`` has no
shell, no package manager and no libc: there is nothing in it to pivot to. The
``:nonroot`` tag runs as uid 65532, and the assets are world-readable, so no
ownership fixing is needed.

The gateway keeps its embedded copy
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

``go:embed`` and ``RegisterUIRoutes`` stay exactly as they are. Removing them would
break ``mcp-gateway`` as a single self-contained artifact, which is what a
non-compose deployment uses and what the README describes; it would also mean
the only way to see the UI is to run a second container.

So there are two ways to serve the UI and they are both supported. The compose
demo points at the new one because that is the shape a real install would use;
anyone running the binary directly keeps what they had.

An asset miss is a 404, not the shell
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The SPA needs history fallback — the frontend has no server-side router, so
``/mcps/postgres-ro`` has to arrive as ``index.html``. But a request with a file
extension that does not exist gets a 404 instead. Returning the shell for
``/assets/index-abc123.js`` hands the browser HTML with a JavaScript content type,
which surfaces as a syntax error rather than a clear miss — the classic symptom
of a stale or half-copied build, and a genuinely annoying hour to debug.

The gateway's embedded server does not make this distinction. That is a
pre-existing inconsistency, not one this ADR introduces; the new server is the
stricter of the two.

Consequences
------------

**Good:**

- The SPA is prebuilt into an image and served immediately; no frontend build
  runs at container start, and none ever did — but now a UI change does not
  rebuild the Go image either.
- The final image contains no build tooling, no shell and no package manager.
- One origin for the browser, so no CORS on the admin API, no API base URL in
  the bundle, and ADR-0014's redirect is untouched.
- A deployment that already has its own ingress can run the binary with no
  ``-api-target`` and route ``/admin/*`` itself.

**Bad / accepted:**

- Two ways to serve the same UI, which must not drift. They share one source
  tree and one ``bun run build``, so what can drift is the serving behaviour
  (the 404-vs-shell rule above), not the app.
- One more container in the demo, and one more image to build and publish.
- The proxy is a hop in front of the admin API. It forwards a fixed prefix and
  adds no headers, but it is still a component that can be misconfigured.
- ``cmd/webui`` is a new module in the workspace. It depends on nothing outside
  the standard library, which keeps that cheap.

Alternatives considered
-----------------------

**Separate origin plus CORS.** Rejected: see the constraint above. It trades a
few lines of proxying for a CORS allowlist on the admin API, runtime
configuration in a static bundle, and a second redirect URI.

**nginx or Caddy as a third container.** Both are excellent static servers, and
either would work. Rejected because the proxy rule is one line of Go we already
need a process for, and a third service plus a config file in another language
is more to keep correct than it saves.

**Drop ``go:embed`` and serve the UI only from the new container.** Rejected: it
breaks the single-binary deployment and makes the simplest way to run the
gateway strictly worse.

**Fix the bun layer caching and change nothing else.** This was on the table and
is the smaller change — the Dockerfile already copies ``package.json`` and the
lockfile on their own layer. Rejected in favour of the split, which also
decouples the two release artifacts; the caching improvement is folded into the
new Dockerfile anyway.

References
----------

- Issue #197
- :doc:`ADR-0010: Control-plane admin authentication </architecture/decisions/0010-control-plane-admin-authentication>`
- :doc:`ADR-0014: Admin UI OIDC + PKCE login </architecture/decisions/0014-admin-ui-oidc-pkce-login>` — the
  redirect URI the single-origin requirement protects.
- ``docs/features/admin-webui.rst``
