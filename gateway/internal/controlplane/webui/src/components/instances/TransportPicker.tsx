import type { MCPTransport } from "../../api/types";
import { TRANSPORTS, TRANSPORT_DESCRIPTIONS } from "../../lib/transport";

export type TransportPickerProps = {
  value: MCPTransport;
  onChange: (transport: MCPTransport) => void;
  /** Disables the whole control while a save is in flight. */
  disabled?: boolean;
};

/**
 * A real segmented control over every transport the gateway implements.
 *
 * It used to render `sse` and `http` as permanently disabled buttons
 * captioned "Not implemented by the gateway yet — only stdio is", which was
 * true: cache.Registry.Register rejected anything but stdio from the day it
 * was written. ADR-0017 implemented both, so the caption had become a lie
 * about a control that worked.
 */
export function TransportPicker({ value, onChange, disabled = false }: TransportPickerProps) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-[10.5px] font-semibold tracking-wide uppercase text-muted">
        Transport
      </span>
      <div
        role="radiogroup"
        aria-label="Transport"
        className="flex gap-0.5 p-0.5 bg-surface border border-border rounded-lg"
      >
        {TRANSPORTS.map((t) => {
          const active = t === value;
          return (
            <button
              key={t}
              type="button"
              role="radio"
              aria-checked={active}
              disabled={disabled}
              title={TRANSPORT_DESCRIPTIONS[t]}
              onClick={() => onChange(t)}
              className={`px-2.5 py-1 border-0 rounded-md text-[11.5px] font-medium font-mono disabled:opacity-50 disabled:cursor-not-allowed ${
                active
                  ? "bg-ink text-white cursor-default"
                  : "bg-transparent text-muted cursor-pointer hover:text-body"
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
