import {
  colourKeywords,
  fontFamily,
  screens,
  semanticBorderColours,
  semanticColours,
  semanticTextColours,
  transitionDuration,
  transitionTimingFunction,
  typography,
  withAlpha,
} from './theme/tailwindTokens'

// Colours come only from design tokens (plan D1, D15): semantic names from
// theme/tailwindTokens.ts plus the CSS colour keywords. The Tailwind palette
// (gray-*, blue-*, …) does not exist here, so a raw palette class compiles to
// nothing — and lint (D21) rejects it before that.
const base = { ...colourKeywords, ...semanticColours }

/** @type {import('tailwindcss').Config} */
export default {
  // `dark` is set on <html> by the theme for dark schemes (prose-invert, third-party).
  darkMode: 'class',
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    screens,
    colors: base,
    textColor: { ...base, ...semanticTextColours },
    borderColor: { ...base, ...semanticBorderColours, DEFAULT: withAlpha('border-hairline') },
    ringColor: { ...base, DEFAULT: withAlpha('focus-ring') },
    placeholderColor: { ...base, ...semanticTextColours },
    extend: {
      fontFamily,
      transitionDuration,
      transitionTimingFunction,
      typography,
    },
  },
  plugins: [
    require("tailwindcss-animate"),
    require("@tailwindcss/typography"),
  ],
}
