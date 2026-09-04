import type { ButtonHTMLAttributes } from "react";

type Variant = "primary" | "secondary" | "danger";

const variantClasses: Record<Variant, string> = {
  primary: "bg-ink text-white hover:bg-ink/90",
  secondary: "bg-surface text-body border border-border hover:border-accent hover:text-accent",
  danger: "bg-surface text-danger border border-danger-border hover:bg-danger-bg",
};

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
};

export function Button({ variant = "primary", className = "", ...props }: ButtonProps) {
  return (
    <button
      {...props}
      className={`px-3.5 py-2 rounded-lg text-[12.5px] font-medium cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${variantClasses[variant]} ${className}`}
    />
  );
}
