import { cva, type VariantProps } from 'class-variance-authority'

export const buttonVariants = cva(
  'inline-flex items-center justify-center font-medium select-none cursor-pointer transition-colors duration-150 active:scale-[0.98] focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-1 disabled:cursor-not-allowed disabled:opacity-60 disabled:active:scale-100 shrink-0',
  {
    variants: {
      variant: {
        primary: 'bg-accent text-white hover:bg-accent-hover active:bg-accent-hover border border-transparent shadow-xs',
        secondary: 'bg-surface-hover text-text-main hover:bg-surface-active border border-border-subtle',
        danger: 'bg-status-danger text-white hover:opacity-90 active:opacity-80 border border-transparent shadow-xs',
        destructive: 'bg-status-danger text-white hover:opacity-90 active:opacity-80 border border-transparent shadow-xs',
        ghost: 'bg-transparent text-text-muted hover:text-text-main hover:bg-surface-hover border border-transparent',
        outline: 'bg-transparent text-text-main hover:bg-surface-hover border border-border-subtle',
        link: 'text-accent underline-offset-4 hover:underline p-0 min-h-0 min-w-0 border-none'
      },
      size: {
        sm: 'px-2.5 py-1 text-xs min-h-[44px] min-w-[44px] sm:min-h-[40px] sm:min-w-[40px] gap-1.5 rounded-md',
        md: 'px-3.5 py-2 text-xs sm:text-sm min-h-[44px] min-w-[44px] sm:min-h-[40px] sm:min-w-[40px] gap-2 rounded-md',
        lg: 'px-4.5 py-2.5 text-sm sm:text-base min-h-[44px] min-w-[44px] sm:min-h-[44px] gap-2.5 rounded-lg',
        icon: 'h-11 w-11 sm:h-9 sm:w-9 min-h-[44px] min-w-[44px] sm:min-h-[36px] sm:min-w-[36px] rounded-md p-0'
      }
    },
    defaultVariants: {
      variant: 'secondary',
      size: 'md'
    }
  }
)

export type ButtonVariants = VariantProps<typeof buttonVariants>

export { default as Button } from './Button.vue'
