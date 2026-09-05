export type ToggleProps = {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label?: string;
  "aria-label"?: string;
};

/** The mockup's recurring pill switch (enable/disable an endpoint, flip a
 * field-filter row, ...). */
export function Toggle({ checked, onChange, label, ...aria }: ToggleProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={aria["aria-label"] ?? label}
      onClick={() => onChange(!checked)}
      className="flex items-center gap-2 border-0 bg-transparent cursor-pointer p-0"
    >
      <span
        className={`flex w-[30px] h-[17px] rounded-full p-0.5 transition-colors ${checked ? "bg-accent justify-end" : "bg-border justify-start"}`}
      >
        <span className="w-[13px] h-[13px] rounded-full bg-white" />
      </span>
      {label && <span className="text-[11.5px] font-medium">{label}</span>}
    </button>
  );
}
