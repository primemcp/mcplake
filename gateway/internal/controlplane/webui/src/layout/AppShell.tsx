import type { ReactNode } from "react";
import { Sidebar, type SidebarProps } from "./Sidebar";

export type AppShellProps = SidebarProps & {
  children: ReactNode;
};

export function AppShell({ children, ...sidebarProps }: AppShellProps) {
  return (
    <div className="flex h-screen min-h-[760px] w-full overflow-hidden">
      <Sidebar {...sidebarProps} />
      <main className="flex-1 min-w-0 flex flex-col overflow-hidden">{children}</main>
    </div>
  );
}
