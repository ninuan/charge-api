import { cleanup, render, screen, waitFor } from "@testing-library/react"
import { ActivityIcon } from "lucide-react"
import { afterEach, describe, expect, it } from "vitest"

import { MetricCard } from "@/components/metric-card"

describe("MetricCard", () => {
  afterEach(() => cleanup())

  it("renders a compact summary item without an outer card", () => {
    render(<MetricCard compact label="全部端口" value={10} detail="所有设备端口总和" icon={ActivityIcon} />)

    expect(screen.getByLabelText("全部端口 指标")).toHaveClass("p-3")
    expect(screen.queryByTestId("metric-card")).not.toBeInTheDocument()
  })

  it("counts smoothly to a changed value", async () => {
    const { rerender } = render(
      <MetricCard
        compact
        label="全部端口"
        value={10}
        detail="所有设备端口总和"
        icon={ActivityIcon}
      />
    )

    rerender(
      <MetricCard
        compact
        label="全部端口"
        value={15}
        detail="所有设备端口总和"
        icon={ActivityIcon}
      />
    )

    await waitFor(() =>
      expect(screen.getByLabelText("全部端口 指标")).toHaveTextContent("15")
    )
  })
})
