import { defineConfig, globalIgnores } from 'eslint/config'
import tseslint from 'typescript-eslint'
import vueParser from 'vue-eslint-parser'
import pluginVue from 'eslint-plugin-vue'
import pluginVitest from '@vitest/eslint-plugin'
import pluginOxlint from 'eslint-plugin-oxlint'
import skipFormatting from 'eslint-config-prettier/flat'

export default defineConfig(
  {
    name: 'app/files-to-lint',
    files: ['**/*.{vue,ts,mts,tsx}'],
  },

  globalIgnores(['**/dist/**', '**/dist-ssr/**', '**/coverage/**']),

  ...pluginVue.configs['flat/essential'],
  ...tseslint.configs.recommended.map((c) => ({ ...c, files: ['**/*.{ts,mts,tsx,vue}'] })),
  ...pluginVue.configs['flat/base'],

  {
    name: 'app/vue-typescript',
    files: ['**/*.vue'],
    languageOptions: {
      parser: vueParser,
      parserOptions: { parser: tseslint.parser, ecmaVersion: 2024, extraFileExtensions: ['.vue'] },
    },
    rules: { 'vue/block-lang': ['error', { script: { lang: 'ts' } }] },
  },

  {
    ...pluginVitest.configs.recommended,
    files: ['src/**/__tests__/*'],
  },

  {
    name: 'app/shared-ui-components',
    files: ['src/**/*.vue'],
    ignores: ['src/components/ui/**', 'src/**/__tests__/**'],
    rules: {
      'vue/no-restricted-html-elements': [
        'error',
        { element: 'input', message: 'Use the shared TextInput, CheckboxInput, or RadioGroup component instead of a raw <input>.' },
        { element: 'select', message: 'Use the shared SelectInput component instead of a raw <select>.' },
        { element: 'textarea', message: 'Use the shared TextareaInput component instead of a raw <textarea>.' },
        { element: 'button', message: 'Use the shared UiButton, ToggleSwitch, or TabNav component instead of a raw <button>.' },
      ],
    },
  },

  ...pluginOxlint.buildFromOxlintConfigFile('.oxlintrc.json'),

  skipFormatting,
)
