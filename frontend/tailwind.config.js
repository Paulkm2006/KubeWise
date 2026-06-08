/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        bg: '#18191c',
        surface: '#222327',
        elevated: '#2c2d32',
        hover: '#37393e',
        border: 'rgba(255,255,255,0.08)',
        text: '#e4e4e9',
        'text-secondary': '#b0b0bb',
        'text-muted': '#808088',
        accent: '#d4a030',
        'accent-dim': 'rgba(212,160,48,0.2)',
        green: '#5cb87a',
        'green-dim': 'rgba(92,184,122,0.2)',
        amber: '#d4a84a',
        'amber-dim': 'rgba(212,168,74,0.2)',
        red: '#e06060',
        'red-dim': 'rgba(224,96,96,0.2)',
      },
      fontFamily: {
        sans: ['"IBM Plex Sans"', 'system-ui', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'monospace'],
      },
      borderRadius: {
        DEFAULT: '6px',
        lg: '10px',
      },
    },
  },
  plugins: [],
};
