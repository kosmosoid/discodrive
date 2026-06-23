// Promise-based app-style dialogs instead of browser confirm()/prompt().
// Single app-level state; rendered by the global <AppDialog> component (see app.vue).
export interface DialogState {
  kind: 'confirm' | 'prompt'
  title: string
  message?: string
  value: string
  confirmText: string
  danger: boolean
  resolve: (v: boolean | string | null) => void
}

export const useDialogState = () => useState<DialogState | null>('appDialog', () => null)

export function useDialog() {
  const state = useDialogState()

  function confirm(
    title: string,
    opts?: { message?: string; confirmText?: string; danger?: boolean },
  ): Promise<boolean> {
    return new Promise((resolve) => {
      state.value = {
        kind: 'confirm',
        title,
        message: opts?.message,
        value: '',
        confirmText: opts?.confirmText ?? 'OK',
        danger: opts?.danger ?? false,
        resolve: (v) => resolve(v === true),
      }
    })
  }

  function prompt(title: string, defaultValue = '', opts?: { confirmText?: string }): Promise<string | null> {
    return new Promise((resolve) => {
      state.value = {
        kind: 'prompt',
        title,
        value: defaultValue,
        confirmText: opts?.confirmText ?? 'OK',
        danger: false,
        resolve: (v) => resolve(typeof v === 'string' ? v : null),
      }
    })
  }

  return { confirm, prompt }
}
