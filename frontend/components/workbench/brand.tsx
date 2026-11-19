export function Brand({ small = false }: { small?: boolean }) {
  return (
    <span className={`wb-brand ${small ? "wb-brand-small" : ""}`}>
      <span className="wb-brand-mark" aria-hidden="true">
        <svg viewBox="0 0 24 24" width="15" height="15" fill="none">
          <path
            d="M13.6 2.5L5.4 13.3h5.5l-.7 8.2 8.4-10.8h-5.5l.5-8.2Z"
            fill="currentColor"
          />
        </svg>
      </span>
      <span className="wb-brand-name">Charge</span>
    </span>
  )
}
