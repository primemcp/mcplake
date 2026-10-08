import { Button } from "./Button";

export type ErrorNoticeProps = {
  message?: string;
  onRetry: () => void;
};

/** Scoped, inline "the API call this section depends on failed" state —
 * never a full-screen crash. See the Admin UI design doc's "Error
 * handling": every data hook exposes {error, retry}, and each section
 * renders this instead of its content when error is set. */
export function ErrorNotice({ message, onRetry }: ErrorNoticeProps) {
  return (
    <div className="flex items-center justify-between gap-3 p-3 border border-danger-border bg-danger-bg rounded-lg text-[11.5px] text-danger">
      <span>{message ?? "Couldn't reach the gateway admin API."}</span>
      <Button variant="danger" onClick={onRetry}>
        Retry
      </Button>
    </div>
  );
}
