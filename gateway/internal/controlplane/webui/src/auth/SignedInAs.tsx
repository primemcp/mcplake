import { useAuth } from "./context";

/**
 * Who the operator is signed in as, plus the way out. Lives in the
 * sidebar footer next to the gateway's own status, and renders nothing at
 * all when admin auth is off — there is no identity to show then, and an
 * empty "signed in as —" block would only raise questions.
 */
export function SignedInAs() {
  const auth = useAuth();
  if (!auth) return null;

  return (
    <div className="px-2.5 py-2.5 rounded-[10px] bg-bg flex flex-col gap-1.5">
      <div className="flex flex-col gap-0.5 min-w-0">
        <div className="text-[10.5px] font-semibold tracking-wide uppercase text-muted">Signed in</div>
        <div className="text-[11.5px] font-mono text-body truncate" title={auth.subject ?? undefined}>
          {auth.subject ?? "unidentified"}
        </div>
      </div>
      <button
        type="button"
        onClick={auth.signOut}
        className="self-start border-0 bg-transparent p-0 cursor-pointer text-[11.5px] font-medium text-accent hover:underline"
      >
        Sign out
      </button>
    </div>
  );
}
