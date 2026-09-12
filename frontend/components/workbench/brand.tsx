import { ZapIcon } from "lucide-react"

export function Brand({ small = false }: { small?: boolean }) {
  return (
    <span className={`wb-brand ${small ? "wb-brand-small" : ""}`}>
      <span className="wb-brand-mark">
        <ZapIcon
          size={20}
          strokeWidth={2.7}
          fill="currentColor"
          aria-hidden="true"
        />
      </span>
      <span>
        charge<span className="wb-brand-period">.</span>
      </span>
    </span>
  )
}
