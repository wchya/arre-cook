import type { ReactNode } from "react"

// 统一的卡片阴影：三页此前重复手写这段长类名，集中到此处便于统一调整
export const cardShadow = "app-card"

interface CardProps {
  children: ReactNode
  className?: string
  onClick?: () => void
}

export default function Card({ children, className = "", onClick }: CardProps) {
  return (
    <div
      onClick={onClick}
      className={`bg-card rounded-2xl border border-border ${cardShadow} ${onClick ? "cursor-pointer transition-transform duration-200 ease-[var(--ease-spring)] active:scale-[.98]" : ""} ${className}`}
    >
      {children}
    </div>
  )
}
