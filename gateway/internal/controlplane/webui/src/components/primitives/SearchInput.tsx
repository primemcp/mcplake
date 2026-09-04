export type SearchInputProps = {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
};

/** The mockup's recurring search box: a magnifier glyph, the field, and a
 * clear button that only appears once there's something to clear. */
export function SearchInput({ value, onChange, placeholder }: SearchInputProps) {
  return (
    <div className="flex items-center gap-2 px-2.5 border border-border rounded-lg bg-[#fbfbfc]">
      <span className="text-[11.5px] text-muted font-mono">⌕</span>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="flex-1 min-w-0 py-2 border-0 bg-transparent text-xs outline-none"
      />
      {value !== "" && (
        <button
          type="button"
          aria-label="Clear search"
          onClick={() => onChange("")}
          className="border-0 bg-transparent cursor-pointer text-xs text-muted p-0"
        >
          ✕
        </button>
      )}
    </div>
  );
}
