import assert from "node:assert/strict";
import test from "node:test";
import vm from "node:vm";
import { resolveWorkbenchTheme, workbenchThemeScript } from "./workbench-theme.ts";

test("new and untouched users follow admin, existing manual choices win", () => {
  for (const siteDefault of ["dark", "light"]) {
    for (const stored of [null, undefined, "", "invalid", "system", "dark", "light"]) {
      const expected = stored === "dark" || stored === "light" ? stored : siteDefault;
      assert.equal(resolveWorkbenchTheme(stored, siteDefault), expected);
      let applied;
      vm.runInNewContext(workbenchThemeScript(siteDefault), {
        localStorage: { getItem: (key) => { assert.equal(key, "theme"); return stored; }, setItem: () => { throw Error("must not persist a default"); } },
        document: { documentElement: { classList: { toggle: (key, value) => { assert.equal(key, "dark"); applied = value; } } } },
      });
      assert.equal(applied, expected === "dark");
    }
  }
  assert.equal(resolveWorkbenchTheme(null, undefined), "dark");
  assert.equal(resolveWorkbenchTheme(null, "<script>bad</script>"), "dark");
});

test("blocked storage still applies the admin default; bootstrap contains no unsafe config", () => {
  let applied;
  vm.runInNewContext(workbenchThemeScript("light"), {
    localStorage: { getItem: () => { throw Error("blocked"); } },
    document: { documentElement: { classList: { toggle: (_, value) => { applied = value; } } } },
  });
  assert.equal(applied, false);
  assert.ok(!workbenchThemeScript("</script><script>alert(1)</script>").includes("</script>"));
});

test("unselected users follow later changes without turning the default into a preference", () => {
  assert.equal(resolveWorkbenchTheme(null, "dark"), "dark");
  assert.equal(resolveWorkbenchTheme(null, "light"), "light");
  assert.equal(resolveWorkbenchTheme("dark", "light"), "dark");
  assert.equal(resolveWorkbenchTheme("light", "dark"), "light");
});
