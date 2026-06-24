import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { mount } from "@vue/test-utils";
import ConnectionBadge from "./ConnectionBadge.vue";

describe("ConnectionBadge styles", () => {
  it("uses the same sizing rhythm as shell action buttons", () => {
    const wrapper = mount(ConnectionBadge, {
      props: { state: "connected" }
    });

    expect(wrapper.get(".connection-badge").classes()).toContain("dashboard-action");
  });

  it("keeps action sizing more specific than generated button utilities", () => {
    const css = readFileSync(resolve(__dirname, "../assets/index.css"), "utf8");

    expect(css).toContain(".shell-action.shell-action");
    expect(css).toContain(".dashboard-action.dashboard-action");
  });
});
