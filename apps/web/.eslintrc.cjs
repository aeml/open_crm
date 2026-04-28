module.exports = {
  root: true,
  env: {
    browser: true,
    es2022: true,
    node: true
  },
  extends: ['eslint:recommended'],
  parserOptions: {
    ecmaVersion: 'latest',
    ecmaFeatures: {
      jsx: true
    },
    sourceType: 'module'
  },
  rules: {
    'no-unused-vars': 'off'
  },
  overrides: [
    {
      files: ['**/*.test.js', '**/*.test.jsx', 'src/test/**/*.js'],
      env: {
        es2022: true
      }
    }
  ]
}
