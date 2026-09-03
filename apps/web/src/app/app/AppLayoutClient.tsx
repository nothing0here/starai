"use client";

import { usePathname } from "next/navigation";
import { useLayoutEffect } from "react";
import { AppShell } from "@/components/AppShell";
import { ForcedAnnouncementModal } from "@/components/ForcedAnnouncementModal";
import { resolveWorkbenchTheme, type WorkbenchTheme } from "@/lib/workbench-theme";

export default function AppLayoutClient({ children, defaultTheme }: { children: React.ReactNode; defaultTheme: WorkbenchTheme }) {
  const pathname = usePathname();
  const selectedModelCode = pathname.startsWith("/app/models/")
    ? pathname.split("/").pop()
    : undefined;
  const selectedAgentCode = pathname.startsWith("/app/agents/")
    ? pathname.split("/").pop()
    : undefined;

  useLayoutEffect(() => {
    const apply = () => {
      let preference: string | null = null;
      try { preference = localStorage.getItem("theme"); } catch { /* Storage may be disabled. */ }
      document.documentElement.classList.toggle("dark", resolveWorkbenchTheme(preference, defaultTheme) === "dark");
    };
    apply();
    const onStorage = (event: StorageEvent) => { if (event.key === "theme" || event.key === null) apply(); };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, [defaultTheme]);

  return (
    <>
      <AppShell selectedModelCode={selectedModelCode} selectedAgentCode={selectedAgentCode}>{children}</AppShell>
      <ForcedAnnouncementModal />
    </>
  );
}
