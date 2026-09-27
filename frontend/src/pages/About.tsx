import { Link, useNavigate } from "react-router-dom"
import { ArrowUpRight, BookOpen, ShoppingBasket, Sparkles } from "lucide-react"
import PageHeader from "@/components/PageHeader"
import { aboutContent as content } from "@/lib/about-content"

const featureIcons = { "book-open": BookOpen, sparkles: Sparkles, "shopping-basket": ShoppingBasket }

export default function About() {
  const navigate = useNavigate()
  return <div className="app-scroll h-dvh overflow-y-auto bg-bg text-text">
    <PageHeader title="关于我们" subtitle="认识你的日常饮食小帮手" onBack={() => window.history.state?.idx > 0 ? navigate(-1) : navigate("/me")} />
    <main className="mx-auto max-w-[640px] px-5 pb-10">
      <section className="border-b border-border py-8">
        <img src="/chef-mark.svg" alt="" className="mb-5 h-16 w-16" />
        <p className="text-sm font-semibold text-primary">{content.name}</p>
        <h2 className="mt-3 max-w-[18ch] text-[28px] font-extrabold leading-snug tracking-tight">{content.tagline.split("，").map((line, index) => <span key={line} className="block">{line}{index === 0 ? "，" : ""}</span>)}</h2>
        <p className="mt-4 text-sm leading-7 text-text2">{content.intro}</p>
      </section>
      <section className="py-7" aria-labelledby="about-features">
        <h2 id="about-features" className="mb-5 text-lg font-bold">在这里，你可以</h2>
        <div className="space-y-6">{content.features.map((feature) => {
          const Icon = featureIcons[feature.icon]
          return <div key={feature.title} className="flex items-start gap-4">
            <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-primary-light text-primary"><Icon size={22} /></span>
            <div><h3 className="text-sm font-bold">{feature.title}</h3><p className="mt-1 text-sm leading-6 text-text2">{feature.text}</p></div>
          </div>
        })}</div>
      </section>
      <section className="rounded-2xl border border-border bg-card p-4" aria-labelledby="about-start">
        <h2 id="about-start" className="mb-2 text-lg font-bold">从这四步开始</h2>
        <ol className="divide-y divide-border">{content.steps.map((step) => <li key={step.number}>
          <Link to={step.web} className="flex min-h-11 items-center gap-3 py-4">
            <span className="self-start pt-0.5 text-sm font-semibold tabular-nums text-primary">{step.number}</span>
            <span className="min-w-0 flex-1"><span className="block text-sm font-semibold">{step.title}</span><span className="mt-1 block text-xs leading-5 text-text2">{step.text}</span></span>
            <ArrowUpRight size={18} className="shrink-0 text-text3" />
          </Link>
        </li>)}</ol>
      </section>
      <section className="pt-8" aria-labelledby="about-help">
        <h2 id="about-help" className="mb-5 text-lg font-bold">用之前，你可能想知道</h2>
        <dl className="space-y-5">{content.questions.map((question) => <div key={question.title}><dt className="text-sm font-semibold">{question.title}</dt><dd className="mt-1.5 text-sm leading-6 text-text2">{question.text}</dd></div>)}</dl>
      </section>
      <footer className="mt-8 border-t border-border pt-5">
        <p className="text-xs leading-6 text-text3">愿每一餐，都更合你的心意。</p>
        <div className="flex flex-wrap gap-x-5"><Link to="/legal?type=terms" className="inline-flex min-h-11 items-center text-xs text-primary underline underline-offset-4">用户服务协议</Link><Link to="/legal?type=privacy" className="inline-flex min-h-11 items-center text-xs text-primary underline underline-offset-4">隐私政策</Link></div>
      </footer>
    </main>
  </div>
}
