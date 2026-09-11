import { cva, type VariantProps } from 'class-variance-authority'

export const badgeVariants = cva(
  'inline-flex items-center rounded-md border px-2.5 py-0.5 text-xs font-medium transition-colors focus:outline-hidden focus:ring-2 focus:ring-accent focus:ring-offset-2',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-accent text-white hover:bg-accent-hover',
        primary: 'border-transparent bg-accent text-white hover:bg-accent-hover',
        secondary: 'border-border-subtle bg-surface-hover text-text-main',
        destructive: 'border-transparent bg-status-danger text-white',
        danger: 'border-transparent bg-status-danger text-white',
        outline: 'border-border-subtle text-text-main',
        success: 'border-status-success/30 bg-status-success/15 text-status-success',
        warning: 'border-status-warning/30 bg-status-warning/15 text-status-warning',
        info: 'border-status-info/30 bg-status-info/15 text-status-info'
      }
    },
    defaultVariants: {
      variant: 'default'
    }
  }
)

export type BadgeVariants = VariantProps<typeof badgeVariants>

export { default as Badge } from './Badge.vue'
