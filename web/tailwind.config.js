/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        cream: 'var(--color-cream)',
        cobalt: 'var(--color-cobalt)',
        jet: 'var(--color-jet)',
        'deep-gray': 'var(--color-deep-gray)',
        muted: 'var(--color-muted)',
        'muted-strong': 'var(--color-muted-strong)',
        hairline: 'var(--color-hairline)',
        amber: 'var(--color-amber)',
        red: 'var(--color-red)',

        // Semantic aliases
        canvas: 'var(--color-cream)',
        surface: 'var(--color-cream)',
        'surface-subtle': 'rgba(255, 255, 255, 0.2)',
        'surface-elevated': 'var(--color-cream)',
        'text-primary': 'var(--color-jet)',
        'text-secondary': 'var(--color-deep-gray)',
        'text-muted': 'var(--color-muted)',
        'text-muted-strong': 'var(--color-muted-strong)',
        accent: 'var(--color-cobalt)',
        'border-hairline': 'var(--color-hairline)',
        'border-default': 'var(--color-hairline)',
      },
      borderRadius: {
        none: '0px',
        control: '0px',
        panel: '0px',
        pill: '0px',
        DEFAULT: '0px',
      },
      boxShadow: {
        none: 'none',
        DEFAULT: 'none',
      },
      fontFamily: {
        sans: ['General Sans', 'Aileron', 'Helvetica Neue', 'Arial', 'sans-serif'],
        display: ['General Sans', 'Aileron', 'Helvetica Neue', 'Arial', 'sans-serif'],
        mono: ['JetBrains Mono', 'IBM Plex Mono', 'monospace'],
      },
      letterSpacing: {
        'tightest': '-0.04em',
        'tighter': '-0.03em',
        'tight': '-0.02em',
        'widest-poster': '0.2em',
      },
      lineHeight: {
        'poster-hero': '0.85',
        'poster-headline': '0.9',
      },
    },
  },
  plugins: [],
};
