/**
 * Typed SVG icon loader.
 *
 * Each icon is imported as a raw SVG string via Vite's `?raw` suffix.
 * The `SvgIconName` type is auto-derived from the map keys, so adding a new
 * SVG file here automatically makes it available to `<Icon name="...">`.
 */
import arrowDown from "./arrow-down.svg?raw";
import arrowUp from "./arrow-up.svg?raw";

export const SVG_ICONS = {
  "arrow-down": arrowDown,
  "arrow-up": arrowUp,
} as const;

export type SvgIconName = keyof typeof SVG_ICONS;
