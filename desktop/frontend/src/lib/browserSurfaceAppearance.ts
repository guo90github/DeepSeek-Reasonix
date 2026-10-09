import { app } from "./bridge";
import { normalizeThemePreference, normalizeThemeStyleForTheme } from "./theme";

/**
 * The independent browser window is its own renderer and mounts without the app
 * runtime, so nothing ever applied the configured appearance there: initTheme()
 * runs while the preference is still the "auto" default, applyTheme removes
 * data-theme, and the light fallback paints the whole window white on a light OS.
 */
export async function applyBrowserSurfaceAppearance(): Promise<void> {
  const settings = await app.DesktopStartupSettings();
  const theme = normalizeThemePreference(settings.desktopTheme);
  const { applyConfiguredBaseAppearance } = await import("./themePack");
  applyConfiguredBaseAppearance(theme, normalizeThemeStyleForTheme(settings.desktopThemeStyle, theme));
}
