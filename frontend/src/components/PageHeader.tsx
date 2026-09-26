import type { ButtonHTMLAttributes, ReactNode } from "react"
import { ArrowLeft, type LucideIcon } from "lucide-react"

interface PageHeaderProps {
  title: ReactNode
  subtitle?: ReactNode
  icon?: LucideIcon
  meta?: ReactNode
  actions?: ReactNode
  onBack?: () => void
  centerTitle?: boolean
  className?: string
}

export function HeaderIconButton({ className = "", children, ...props }: ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      className={`inline-flex h-9 w-9 items-center justify-center rounded-full border border-border glass-strong text-text2 shadow-[0_1px_3px_rgba(26,26,46,.05),0_6px_18px_rgba(26,26,46,.06)] transition-all duration-200 ease-[var(--ease-spring)] hover:border-primary/20 hover:bg-primary-light hover:text-primary active:scale-90 ${className}`}
      {...props}
    >
      {children}
    </button>
  )
}

export default function PageHeader({
  title,
  subtitle,
  icon: Icon,
  meta,
  actions,
  onBack,
  centerTitle = false,
  className = "",
}: PageHeaderProps) {
  return (
    <header className={`sticky top-0 z-[100] border-b border-glass-border glass shadow-[0_10px_28px_rgba(26,26,46,.045)] ${className}`}>
      <div className="mx-auto grid min-h-[64px] max-w-[640px] grid-cols-[1fr_auto] items-center gap-3 px-5 py-2">
        <div className="flex min-w-0 items-center gap-3">
          {onBack && (
            <HeaderIconButton onClick={onBack} aria-label="返回" className="shrink-0">
              <ArrowLeft size={18} strokeWidth={2.4} />
            </HeaderIconButton>
          )}
          {Icon && (
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-[14px] border border-primary/10 bg-primary-light text-primary shadow-[inset_0_1px_0_var(--color-highlight),0_7px_18px_rgba(232,115,74,.11)]">
              <Icon size={23} strokeWidth={2.45} />
            </div>
          )}
          <div className={`min-w-0 ${centerTitle ? "text-center" : ""}`}>
            <div className="truncate text-[18px] font-extrabold leading-tight tracking-tight text-text">{title}</div>
            {subtitle && <div className="mt-1 truncate text-[12px] font-medium leading-tight text-text3">{subtitle}</div>}
          </div>
        </div>
        <div className="ml-auto flex min-w-0 shrink-0 items-center justify-end gap-2">
          {meta && <div className="max-w-[150px] truncate text-right text-[11px] font-medium text-text3">{meta}</div>}
          {actions}
        </div>
      </div>
    </header>
  )
}
