import type { InputHTMLAttributes } from "react";

export type InputProps = InputHTMLAttributes<HTMLInputElement> & {
  mono?: boolean;
};

export function Input({ mono = false, className = "", ...props }: InputProps) {
  return (
    <input
      {...props}
      className={`w-full min-w-0 px-2.5 py-2 border border-border rounded-lg text-[12.5px] outline-none focus:border-accent ${mono ? "font-mono" : ""} ${className}`}
    />
  );
}
