/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        primary: {
          DEFAULT: '#1a73e8',
          dark: '#1557b0',
          light: '#4a90d9',
        },
        secondary: '#34a853',
        danger: '#ea4335',
        warning: '#fbbc04',
      },
      fontFamily: {
        sans: ['Tajawal', 'system-ui', 'sans-serif'],
      },
    },
  },
  plugins: [],
};
