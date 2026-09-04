export type Tab = { id: string; label: string };

export type TabsProps = {
  tabs: Tab[];
  activeId: string;
  onChange: (id: string) => void;
};

/** The mockup's segmented tab control (Token match / Access / Request path
 * on the Users & access detail panel). Purely presentational — the caller
 * owns which panel is shown for activeId. */
export function Tabs({ tabs, activeId, onChange }: TabsProps) {
  return (
    <div role="tablist" className="flex gap-0.5 p-0.5 bg-bg border border-border rounded-lg">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          type="button"
          role="tab"
          aria-selected={tab.id === activeId}
          onClick={() => onChange(tab.id)}
          className={`px-3 py-1.5 rounded-md border-0 cursor-pointer text-[12.5px] font-medium ${
            tab.id === activeId ? "bg-surface text-ink" : "bg-transparent text-subtle"
          }`}
        >
          {tab.label}
        </button>
      ))}
    </div>
  );
}
