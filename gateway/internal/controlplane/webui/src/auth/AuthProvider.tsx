import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { api, configureAuth } from "../api/client";
import { AuthContext } from "./context";
import type { AuthConfig, LoginConfig } from "../api/types";
import { AuthBusyScreen, AuthUnavailableScreen, ForbiddenScreen, LoginScreen, LoginUnconfiguredScreen } from "./AuthScreens";
import { currentHref, redirectTo, redirectUri, stripQuery } from "./browser";
import { authorizeUrl, exchangeCode, loginConfigOf, readCallback, refreshWith, subjectOf } from "./oidc";
import { clearSession, isExpired, loadSession, savePending, saveSession, sessionFrom, takePending, type Session } from "./session";
import { newState, newVerifier } from "./pkce";

/**
 * Owns the admin UI's authentication: whether it is needed at all, running
 * the OIDC Authorization Code + PKCE flow when it is, and holding the
 * resulting session. See ADR-0014.
 *
 * It gates rendering deliberately — `children` appear only once the app is
 * allowed to call the admin API (signed in, or the gate is off). Rendering
 * the screens first and letting their own fetches 401 would flood the
 * operator with failure states for a situation that isn't an error.
 *
 * Its other half is the pair of hooks it installs into `api/client.ts`:
 * one supplies (and, when expired, silently renews) the bearer token; the
 * other turns the admin API's 401/403 into the right screen.
 */

type State =
  | { status: "loading" }
  | { status: "unavailable"; message: string }
  | { status: "disabled" }
  | { status: "unconfigured" }
  | { status: "anonymous"; login: LoginConfig; issuer?: string; message?: string }
  | { status: "redirecting"; login: LoginConfig; issuer?: string }
  | { status: "exchanging" }
  | { status: "authenticated"; subject: string | null }
  | { status: "forbidden"; subject: string | null };

const SESSION_ENDED = "Your session is no longer valid. Sign in again.";

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<State>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);

  // The token/config the installed hooks read. Refs, not state: they are
  // consulted from outside React (every admin request) and must never be
  // a render behind.
  const sessionRef = useRef<Session | null>(null);
  const loginRef = useRef<LoginConfig | null>(null);
  const refreshing = useRef<Promise<string | null> | null>(null);
  // StrictMode runs effects twice in development; the authorization code
  // is single-use, so the second run must not try to redeem it again.
  const bootstrapped = useRef(false);

  const applySession = useCallback((session: Session) => {
    sessionRef.current = session;
    saveSession(session);
  }, []);

  const endSession = useCallback((message: string) => {
    sessionRef.current = null;
    clearSession();
    const login = loginRef.current;
    setState(login ? { status: "anonymous", login, message } : { status: "unconfigured" });
  }, []);

  const getToken = useCallback(async (): Promise<string | null> => {
    const session = sessionRef.current;
    if (!session) return null;
    if (!isExpired(session, Date.now())) return session.accessToken;

    // Several screens fetch in parallel on mount; one expired token must
    // produce one refresh, not one per request.
    if (refreshing.current) return refreshing.current;

    const login = loginRef.current;
    if (!login || session.refreshToken === null) {
      endSession("Your session expired. Sign in again.");
      return null;
    }
    refreshing.current = (async () => {
      try {
        const tokens = await refreshWith(login, session.refreshToken as string);
        const next = sessionFrom(tokens, Date.now(), session.refreshToken);
        applySession(next);
        setState({ status: "authenticated", subject: subjectOf(next.accessToken) });
        return next.accessToken;
      } catch {
        endSession("Your session expired. Sign in again.");
        return null;
      } finally {
        refreshing.current = null;
      }
    })();
    return refreshing.current;
  }, [applySession, endSession]);

  const onAuthFailure = useCallback(
    (status: number) => {
      if (status === 403) {
        // Not an authentication problem: the token is fine, its claims
        // just aren't admin claims. Keep the session so the screen can
        // say who this is (see ForbiddenScreen).
        setState({ status: "forbidden", subject: subjectOf(sessionRef.current?.accessToken ?? "") });
        return;
      }
      endSession(SESSION_ENDED);
    },
    [endSession],
  );

  const signIn = useCallback(async (login: LoginConfig, issuer?: string) => {
    setState({ status: "redirecting", login, issuer });
    const verifier = newVerifier();
    const state = newState();
    savePending({ verifier, state });
    redirectTo(await authorizeUrl(login, { verifier, state, redirectUri: redirectUri() }));
  }, []);

  const signOut = useCallback(() => {
    // Local only: the provider's own session is deliberately left alone
    // (RP-initiated logout is ADR-0014 follow-up work).
    endSession("");
  }, [endSession]);

  useEffect(() => {
    if (bootstrapped.current) return;
    bootstrapped.current = true;

    let cancelled = false;
    const authenticate = async () => {
      let config: AuthConfig;
      try {
        config = await api.authConfig();
      } catch (err) {
        if (!cancelled) {
          setState({ status: "unavailable", message: err instanceof Error ? err.message : "Request failed." });
        }
        return;
      }
      if (cancelled) return;

      if (!config.auth_required) {
        configureAuth(null);
        setState({ status: "disabled" });
        return;
      }

      const login = loginConfigOf(config);
      loginRef.current = login;
      configureAuth({ getToken, onAuthFailure });
      if (!login) {
        setState({ status: "unconfigured" });
        return;
      }

      const callback = readCallback(currentHref());
      if (callback.kind !== "none") {
        const pending = takePending();
        // Consume the code from the address bar before anything can fail,
        // so a reload never replays it.
        stripQuery();
        if (callback.kind === "error") {
          setState({ status: "anonymous", login, issuer: config.issuer, message: callback.message });
          return;
        }
        if (!pending || pending.state !== callback.state) {
          setState({
            status: "anonymous",
            login,
            issuer: config.issuer,
            message: "That sign-in could not be verified. Start again from this page.",
          });
          return;
        }
        setState({ status: "exchanging" });
        try {
          const tokens = await exchangeCode(login, {
            code: callback.code,
            verifier: pending.verifier,
            redirectUri: redirectUri(),
          });
          const session = sessionFrom(tokens, Date.now());
          applySession(session);
          setState({ status: "authenticated", subject: subjectOf(session.accessToken) });
        } catch (err) {
          setState({
            status: "anonymous",
            login,
            issuer: config.issuer,
            message: err instanceof Error ? err.message : "Sign-in failed.",
          });
        }
        return;
      }

      const stored = loadSession();
      if (stored && !(isExpired(stored, Date.now()) && stored.refreshToken === null)) {
        sessionRef.current = stored;
        setState({ status: "authenticated", subject: subjectOf(stored.accessToken) });
        return;
      }
      clearSession();
      setState({ status: "anonymous", login, issuer: config.issuer });
    };

    void authenticate();
    return () => {
      cancelled = true;
    };
    // `attempt` re-runs this after a Retry; the callbacks are stable.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [attempt]);

  useEffect(() => () => configureAuth(null), []);

  switch (state.status) {
    case "loading":
      return <AuthBusyScreen message="Checking…" />;
    case "exchanging":
      return <AuthBusyScreen message="Signing you in…" />;
    case "unavailable":
      return (
        <AuthUnavailableScreen
          message={state.message}
          onRetry={() => {
            bootstrapped.current = false;
            setState({ status: "loading" });
            setAttempt((n) => n + 1);
          }}
        />
      );
    case "unconfigured":
      return <LoginUnconfiguredScreen />;
    case "anonymous":
    case "redirecting":
      return (
        <LoginScreen
          issuer={state.issuer}
          error={state.status === "anonymous" ? state.message || undefined : undefined}
          busy={state.status === "redirecting"}
          onSignIn={() => void signIn(state.login, state.issuer)}
        />
      );
    case "forbidden":
      return <ForbiddenScreen subject={state.subject} onSignOut={signOut} />;
    case "disabled":
      return <AuthContext.Provider value={null}>{children}</AuthContext.Provider>;
    case "authenticated":
      return (
        <AuthContext.Provider value={{ subject: state.subject, signOut }}>{children}</AuthContext.Provider>
      );
  }
}
