import { ref, reactive, computed } from 'vue'

export interface ValidationRule {
  test: (value: any) => boolean
  message: string
}

export interface FieldConfig {
  value: any
  rules?: ValidationRule[]
}

export function useFormValidation(fields: Record<string, FieldConfig>) {
  const errors = reactive<Record<string, string>>({})
  const touched = reactive<Record<string, boolean>>({})

  function validateField(name: string): boolean {
    const field = fields[name]
    if (!field?.rules) return true
    for (const rule of field.rules) {
      if (!rule.test(field.value)) {
        errors[name] = rule.message
        return false
      }
    }
    delete errors[name]
    return true
  }

  function validateAll(): boolean {
    let valid = true
    for (const name of Object.keys(fields)) {
      touched[name] = true
      if (!validateField(name)) valid = false
    }
    return valid
  }

  function touch(name: string) {
    touched[name] = true
    validateField(name)
  }

  function reset() {
    for (const name of Object.keys(errors)) delete errors[name]
    for (const name of Object.keys(touched)) delete touched[name]
  }

  const isValid = computed(() => {
    for (const name of Object.keys(fields)) {
      if (fields[name].rules) {
        for (const rule of fields[name].rules!) {
          if (!rule.test(fields[name].value)) return false
        }
      }
    }
    return true
  })

  function getError(name: string): string | undefined {
    return touched[name] ? errors[name] : undefined
  }

  return { errors, touched, validateField, validateAll, touch, reset, isValid, getError }
}

// Common validation rules
export const required = (msg = '此项必填'): ValidationRule => ({
  test: (v) => v !== null && v !== undefined && String(v).trim() !== '',
  message: msg,
})

export const minLength = (n: number, msg?: string): ValidationRule => ({
  test: (v) => !v || String(v).length >= n,
  message: msg || `至少 ${n} 个字符`,
})

export const maxLength = (n: number, msg?: string): ValidationRule => ({
  test: (v) => !v || String(v).length <= n,
  message: msg || `最多 ${n} 个字符`,
})

export const url = (msg = '请输入有效的 URL'): ValidationRule => ({
  test: (v) => {
    if (!v) return true
    try { new URL(v); return true } catch { return false }
  },
  message: msg,
})

export const pattern = (re: RegExp, msg = '格式不正确'): ValidationRule => ({
  test: (v) => !v || re.test(String(v)),
  message: msg,
})
