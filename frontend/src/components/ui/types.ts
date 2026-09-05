export type ButtonVariant = 'primary' | 'secondary' | 'danger' | 'ghost'
export type ButtonSize = 'sm' | 'md' | 'lg'

export type ModalSize = 'sm' | 'md' | 'lg'
export type DrawerPlacement = 'left' | 'right'

export interface SelectOption {
  value: string | number
  label: string
  disabled?: boolean
}

export interface ComboboxOption {
  value: string | number
  label: string
  disabled?: boolean
  description?: string
}

export interface MetricCardProps {
  label: string
  value: string | number
  unit?: string
  subtext?: string
  description?: string
  status?: 'success' | 'warning' | 'danger' | 'info' | 'neutral'
  active?: boolean
  clickable?: boolean
  wide?: boolean
}
