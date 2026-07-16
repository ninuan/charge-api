import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { AuthPageShell } from "@/components/auth-page-shell"

describe("AuthPageShell", () => {
  it("renders only a centered authentication panel", () => {
    render(<AuthPageShell mode="login"><div>login fields</div></AuthPageShell>)

    expect(screen.queryByText("Infrastructure, clearly seen")).not.toBeInTheDocument()
    expect(screen.getByRole("main")).toHaveClass("min-h-dvh", "pt-[max(1rem,6dvh)]", "pb-4")
  })
})
