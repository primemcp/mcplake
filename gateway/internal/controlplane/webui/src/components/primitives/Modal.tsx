import { useEffect, type ReactNode } from "react";

export type ModalProps = {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  /** "wide" for content that needs real screen space (e.g. the schema
   * graph) instead of the default form-sized dialog. */
  size?: "default" | "wide";
};

/**
 * Centered dialog for forms that need more room than the sidebar's inline
 * pattern gives them (e.g. Add MCP endpoint) — the mockup itself uses
 * inline `data-inline-form` panels everywhere, but a modal was requested
 * explicitly for this one interaction rather than being a fidelity choice.
 */
export function Modal({ open, onClose, title, children, size = "default" }: ModalProps) {
  useEffect(() => {
    if (!open) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-6"
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onClick={(e) => e.stopPropagation()}
        className={`w-full bg-surface border border-border rounded-2xl shadow-xl p-5 flex flex-col gap-3 ${
          size === "wide" ? "max-w-[min(1100px,92vw)] h-[85vh]" : "max-w-[440px]"
        }`}
      >
        <div className="flex items-center justify-between">
          <div className="text-[15px] font-semibold">{title}</div>
          <button
            type="button"
            aria-label="Close"
            onClick={onClose}
            className="border-0 bg-transparent cursor-pointer text-subtle text-sm p-1 leading-none"
          >
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}
