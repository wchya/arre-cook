import { useAppInfoStore } from "@/store/useAppInfoStore"

export default function Splash() {
  const appName = useAppInfoStore((s) => s.appName)
  return (
    <div className="flex h-dvh flex-col items-center justify-center gap-4 bg-bg" role="status" aria-live="polite">
      <div className="chef-loading-mark">
        <img src="/chef-mark.svg" alt="" className="h-full w-full" />
      </div>
      <div className="text-[13px] font-semibold tracking-wide text-text3">{appName} 正在准备厨房</div>
    </div>
  )
}
