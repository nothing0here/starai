export type WorkbenchTheme = "dark" | "light";

// The existing `theme` key was written only by a manual toggle. Never persist
// the admin default there: absence must continue to mean “follow the site”.
export function resolveWorkbenchTheme(preference: unknown, siteDefault: unknown): WorkbenchTheme {
  if (preference === "dark" || preference === "light") return preference;
  return siteDefault === "light" ? "light" : "dark";
}

export function workbenchThemeScript(siteDefault: unknown): string {
  const fallback = resolveWorkbenchTheme(null, siteDefault);
  return `(()=>{let t='${fallback}';try{const p=localStorage.getItem('theme');if(p==='dark'||p==='light')t=p}catch{}document.documentElement.classList.toggle('dark',t==='dark')})()`;
}
