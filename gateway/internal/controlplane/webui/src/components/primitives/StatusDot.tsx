export type Status = "active" | "connecting" | "unreachable" | "unknown";

const statusColor: Record<Status, string> = {
  active: "bg-success",
  connecting: "bg-accent",
  unreachable: "bg-danger",
  unknown: "bg-muted",
};

export const statusTextColor: Record<Status, string> = {
  active: "text-success",
  connecting: "text-accent",
  unreachable: "text-danger",
  unknown: "text-muted",
};

export function statusFromString(raw: string): Status {
  if (raw === "active" || raw === "connecting" || raw === "unreachable") return raw;
  return "unknown";
}

export type StatusDotProps = {
  status: Status;
  label?: string;
};

export function StatusDot({ status, label }: StatusDotProps) {
  return (
    <span className="inline-flex items-center gap-1.5 text-[11px] font-medium">
      <span className={`w-1.5 h-1.5 rounded-full ${statusColor[status]}`} />
      {label}
    </span>
  );
}
