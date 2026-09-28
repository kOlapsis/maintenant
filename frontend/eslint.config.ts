import { globalIgnores } from 'eslint/config'
import { defineConfigWithVueTs, vueTsConfigs } from '@vue/eslint-config-typescript'
import pluginVue from 'eslint-plugin-vue'
import pluginVitest from '@vitest/eslint-plugin'
import pluginOxlint from 'eslint-plugin-oxlint'
import skipFormatting from 'eslint-config-prettier/flat'

// To allow more languages other than `ts` in `.vue` files, uncomment the following lines:
// import { configureVueProject } from '@vue/eslint-config-typescript'
// configureVueProject({ scriptLangs: ['ts', 'tsx'] })
// More info at https://github.com/vuejs/eslint-config-typescript/#advanced-setup

export default defineConfigWithVueTs(
  {
    name: 'app/files-to-lint',
    files: ['**/*.{vue,ts,mts,tsx}'],
  },

  globalIgnores(['**/dist/**', '**/dist-ssr/**', '**/coverage/**']),

  ...pluginVue.configs['flat/essential'],
  vueTsConfigs.recommended,

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
        'warn',
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
