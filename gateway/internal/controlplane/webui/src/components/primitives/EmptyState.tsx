import type { ReactNode } from "react";

export type EmptyStateProps = {
  title: string;
  subtitle?: string;
  action?: ReactNode;
};

export function EmptyState({ title, subtitle, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center gap-2 px-4 py-10 text-center border border-dashed border-line rounded-xl text-subtle">
      <p className="text-[12.5px] font-medium text-ink">{title}</p>
      {subtitle && <p className="text-[11.5px]">{subtitle}</p>}
      {action}
    </div>
  );
}
