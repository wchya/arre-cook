import { legalContent } from "@/lib/legal-content"

export default function LegalContent({ type }: { type: "terms" | "privacy" }) {
  return <div className="space-y-6 text-sm leading-7 text-text2">
    {legalContent[type].sections.map((section) => <section key={section.heading}>
      <h2 className="mb-2 text-base font-bold text-text">{section.heading}</h2>
      {section.paragraphs.map((paragraph) => <p key={paragraph} className="mb-2">{paragraph}</p>)}
    </section>)}
  </div>
}
