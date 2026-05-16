/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: "#0b0d10",
        panel: "#14171c",
        panel2: "#1b1f25",
        accent: "#e6b450",
        accentMuted: "#8a6d2f",
        text: "#e6e1cf",
        muted: "#7a8086",
      },
    },
  },
  plugins: [],
};
