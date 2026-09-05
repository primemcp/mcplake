const TRANSPORTS = ["stdio", "sse", "http"] as const;

/**
 * Shown explicitly rather than silently defaulted in code: stdio is the
 * only transport cache.Registry.Register implements on the real backend
 * ("unsupported transport ... only stdio is implemented"), so sse/http are
 * rendered as real, visible, disabled options rather than hidden entirely
 * — matches the mockup's segmented transport control, with the two
 * non-functional options grayed out and labeled why.
 */
export function TransportPicker() {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-[10.5px] font-semibold tracking-wide uppercase text-muted">
        Transport
      </span>
      <div className="flex gap-0.5 p-0.5 bg-surface border border-border rounded-lg">
        {TRANSPORTS.map((t) => {
          const active = t === "stdio";
          return (
            <button
              key={t}
              type="button"
              disabled={!active}
              title={active ? undefined : "Not implemented by the gateway yet — only stdio is"}
              className={`px-2.5 py-1 border-0 rounded-md text-[11.5px] font-medium font-mono ${
                active
                  ? "bg-ink text-white cursor-default"
                  : "bg-transparent text-muted cursor-not-allowed opacity-60"
              }`}
            >
              {t}
            </button>
          );
        })}
      </div>
    </div>
  );
}
