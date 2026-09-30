import { api } from '../../api/client'

// The settings endpoint returns status only. Neither the shared token nor its hash is retrievable.
export interface AuthSettings {
  admin_auth_enabled: boolean
  export_auth_enabled: boolean
  token_configured: boolean
  admin_mode: 'open' | 'protected'
  export_mode: 'open' | 'protected'
}

export function fetchAuthSettings(): Promise<AuthSettings> {
  return api.get<AuthSettings>('/api/v1/settings/auth')
}
