import AppLayoutClient from "./AppLayoutClient";
import { getPublicSystemConfig } from "@/lib/public-config";
import { resolveWorkbenchTheme, workbenchThemeScript } from "@/lib/workbench-theme";

export default async function AppLayout({ children }: { children: React.ReactNode }) {
  const config = await getPublicSystemConfig();
  const defaultTheme = resolveWorkbenchTheme(null, config.workbench_default_theme);
  return <>
    <script dangerouslySetInnerHTML={{ __html: workbenchThemeScript(defaultTheme) }} />
    <AppLayoutClient defaultTheme={defaultTheme}>{children}</AppLayoutClient>
  </>;
}
