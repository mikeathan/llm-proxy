/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      keyframes: {
        'alert-glow': {
          '0%, 100%': { boxShadow: '0 0 0 0 color-mix(in srgb, var(--color-live, #22c55e) 45%, transparent)' },
          '50%': { boxShadow: '0 0 0 3px color-mix(in srgb, var(--color-live, #22c55e) 0%, transparent)' },
        },
      },
      animation: {
        'alert-glow': 'alert-glow 2s ease-in-out infinite',
      },
    },
  },
  plugins: [
    require("tailwindcss-animate"),
    require("@tailwindcss/typography"),
  ],
}
