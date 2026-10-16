import js from '@eslint/js';
import tseslint from 'typescript-eslint';

const nodeGlobals = { process: 'readonly', console: 'readonly', fetch: 'readonly', Buffer: 'readonly', URL: 'readonly' };

export default tseslint.config(
  { ignores: ['dist', 'coverage', 'node_modules', '.claude'] },
  js.configs.recommended,
  ...tseslint.configs.strict,
  { files: ['**/*.{js,mjs}'], languageOptions: { globals: nodeGlobals } },
  {
    rules: {
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/no-non-null-assertion': 'warn',
    },
  },
);
