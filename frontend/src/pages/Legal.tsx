import { useNavigate, useSearchParams } from "react-router-dom"
import PageHeader from "@/components/PageHeader"
import LegalContent from "@/components/LegalContent"

export default function Legal() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const type = params.get("type") === "terms" ? "terms" : "privacy"
  return <div className="min-h-dvh bg-bg pb-10">
    <PageHeader title={type === "terms" ? "用户服务协议" : "隐私政策"} onBack={() => navigate(-1)} />
    <main className="mx-auto max-w-[640px] px-5 py-6"><LegalContent type={type} /></main>
  </div>
}
