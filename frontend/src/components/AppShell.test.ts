import { mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AppShell from "./AppShell.vue";
import { useAuthStore } from "@/stores/auth";
import type { CurrentUser } from "@/types/dashboard";

vi.mock("vue-router", () => ({
  useRouter: () => ({ replace: vi.fn() })
}));

const user: CurrentUser = {
  id: "user-1",
  username: "wang",
  role: "user",
  enabled: true,
  createdAt: "2026-06-24T00:00:00Z",
  deviceLimit: 5,
  refreshEnabled: true
};

describe("AppShell", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("renders the identity role as part of the shell instead of a generic badge", () => {
    const auth = useAuthStore();
    auth.currentUser = user;

    const wrapper = mount(AppShell, {
      props: {
        title: "充电桩运营看板",
        description: "查看端口状态"
      },
      global: {
        stubs: {
          SecurityDialog: true
        }
      }
    });

    const identity = wrapper.get(".identity-pill");

    expect(identity.find(".identity-role").exists()).toBe(true);
    expect(identity.find(".identity-role").text()).toContain("普通用户");
    expect(identity.find('[data-slot="badge"]').exists()).toBe(false);
  });
});
