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
});
