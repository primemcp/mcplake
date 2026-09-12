import type { ReactNode } from "react";
import { Button } from "../components/primitives/Button";
import { Card } from "../components/primitives/Card";

/**
 * The full-window states the app can be in before (or instead of) showing
 * the admin screens: signing in, refusing to, or unable to tell. Kept in
 * one file because they are one visual family — the same centred card,
 * the same brand block as the sidebar's, differing only in what they say
 * and what (if anything) the operator can do about it.
 */

function AuthShell({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="h-full flex items-center justify-center bg-bg p-6">
      <Card className="w-[380px] max-w-full p-6 flex flex-col gap-4">
        <div className="flex items-center gap-2.5">
          <div className="w-7 h-7 rounded-lg bg-ink grid place-items-center text-white font-mono text-[13px] font-medium">
            g
          </div>
          <div className="flex flex-col leading-tight">
            <div className="text-[13.5px] font-semibold">Gateway</div>
            <div className="text-[11px] text-muted">MCP · Phase 1</div>
          </div>
        </div>
        <h1 className="m-0 text-[17px] font-semibold tracking-tight">{title}</h1>
        {children}
      </Card>
    </div>
  );
}

/** A provider-side or exchange failure, shown above the sign-in button.
 * Always the provider's own words where there are any — that text is what
 * tells an operator whether to fix their client registration or just
 * retry. */
function Problem({ message }: { message: string }) {
  return (
    <p className="m-0 px-3 py-2.5 rounded-lg border border-danger-border bg-danger-bg text-[11.5px] text-danger leading-[1.5]">
      {message}
    </p>
  );
}

export type LoginScreenProps = {
  issuer?: string;
  error?: string;
  busy: boolean;
  onSignIn: () => void;
};

export function LoginScreen({ issuer, error, busy, onSignIn }: LoginScreenProps) {
  return (
    <AuthShell title="Sign in to continue">
      <p className="m-0 text-[12.5px] text-subtle leading-[1.55]">
        The gateway's admin API requires an administrator token. You'll be sent to{" "}
        <span className="font-mono text-body">{issuer ?? "your identity provider"}</span> and returned here once you've
        signed in.
      </p>
      {error && <Problem message={error} />}
      <Button onClick={onSignIn} disabled={busy}>
        {busy ? "Signing in…" : "Sign in"}
      </Button>
      <p className="m-0 text-[11px] text-muted leading-[1.5]">
        Your account must also satisfy the gateway's <span className="font-mono">admin_auth.match</span> rules.
      </p>
    </AuthShell>
  );
}

/**
 * The 403 state: authentication succeeded, authorization did not. Offering
 * "Sign in" here would loop forever — the same principal would get the same
 * answer — so the only way forward is signing out and back in as someone
 * whose claims satisfy admin_auth.match.
 */
export function ForbiddenScreen({ subject, onSignOut }: { subject: string | null; onSignOut: () => void }) {
  return (
    <AuthShell title="You're not an admin here">
      <p className="m-0 text-[12.5px] text-subtle leading-[1.55]">
        You signed in as{" "}
        <span className="font-mono text-body">{subject ?? "an unidentified principal"}</span>, but those claims don't
        satisfy the gateway's <span className="font-mono">admin_auth.match</span> rules, so the admin API returned 403.
      </p>
      <p className="m-0 text-[11px] text-muted leading-[1.5]">
        Signing in again as the same user won't change this. Ask whoever runs the gateway for the claim its rules
        expect, or sign in as a different account.
      </p>
      <Button variant="secondary" onClick={onSignOut}>
        Sign out
      </Button>
    </AuthShell>
  );
}

/**
 * Admin auth is enforced but `[admin_auth.login]` is unset, so there is
 * literally nowhere to redirect to. A dead "Sign in" button would be worse
 * than saying what to fix.
 */
export function LoginUnconfiguredScreen() {
  return (
    <AuthShell title="Sign-in isn't configured">
      <p className="m-0 text-[12.5px] text-subtle leading-[1.55]">
        This gateway requires an admin token, but no login flow is configured, so this UI has no way to obtain one.
      </p>
      <p className="m-0 text-[11.5px] text-body leading-[1.55]">
        Add an <span className="font-mono">[admin_auth.login]</span> section (client_id, authorization_endpoint,
        token_endpoint) to the gateway's config and restart it.
      </p>
    </AuthShell>
  );
}

export function AuthUnavailableScreen({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <AuthShell title="Can't reach the gateway">
      <p className="m-0 text-[12.5px] text-subtle leading-[1.55]">
        The admin API didn't answer, so it's unknown whether a sign-in is needed.
      </p>
      <Problem message={message} />
      <Button variant="secondary" onClick={onRetry}>
        Retry
      </Button>
    </AuthShell>
  );
}

export function AuthBusyScreen({ message }: { message: string }) {
  return (
    <div className="h-full flex items-center justify-center bg-bg text-[12.5px] text-subtle">{message}</div>
  );
}
