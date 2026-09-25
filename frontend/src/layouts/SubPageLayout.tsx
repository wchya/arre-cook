import { Outlet } from "react-router-dom"

export default function SubPageLayout() {
  return (
    <div className="app-scroll h-dvh bg-bg overflow-y-auto overscroll-y-contain" style={{ touchAction: "pan-y" }}>
      <Outlet />
    </div>
  )
}
