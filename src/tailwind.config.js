/** @type {import('tailwindcss').Config} */
module.exports = {
    content: [
      './frontend/templates/**/*.html',
      './handlers/**/*.go'
    ],
    theme: {
      extend: {},
    },
    plugins: [], // leave empty, since you're using @plugin in CSS
  }