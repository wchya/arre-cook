import { useAppInfoStore } from "@/store/useAppInfoStore"

export default function Splash() {
  const appName = useAppInfoStore((s) => s.appName)
  return (
    <div className="flex h-dvh flex-col items-center justify-center gap-4 bg-bg">
      <img src="/180.png" alt={appName} className="h-16 w-16 animate-pulse rounded-[20px] shadow-[0_12px_32px_rgba(232,115,74,.25)]" />
      <div className="text-[13px] font-semibold tracking-wide text-text3">{appName}</div>
    </div>
  )
}
