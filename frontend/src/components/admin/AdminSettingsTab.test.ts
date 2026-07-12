import { createPinia } from "pinia";
import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import AdminSettingsTab from "./AdminSettingsTab.vue";
import { useAdminStore } from "@/stores/admin";
import type { InviteCode, RegistrationSettings } from "@/types/dashboard";

const settings: RegistrationSettings = {
  openRegistration: true,
  inviteRequired: true,
  defaultDeviceLimit: 10,
  defaultRefreshEnabled: true,
  statsRetentionDays: 90
};

const invites: InviteCode[] = [{
  id: "invite-1",
  code: "CHG-SERVER-CODE",
  enabled: true,
  createdAt: "2026-07-09T08:00:00Z",
  usedCount: 2
}];

describe("AdminSettingsTab", () => {
  it("protects a dirty draft until the administrator saves it", async () => {
    const pinia = createPinia();
    const wrapper = mount(AdminSettingsTab, {
      props: { settings, invites },
      global: { plugins: [pinia] }
    });
    const admin = useAdminStore();
    admin.saveSettings = vi.fn().mockResolvedValue(undefined);

    const limit = wrapper.get('[aria-label="新用户默认设备额度"]');
    expect((limit.element as HTMLInputElement).value).toBe("10");
    expect(wrapper.get('[aria-label="统计数据保留时间"]').attributes("type")).toBe("number");

    await limit.setValue("20");
    expect(admin.saveSettings).not.toHaveBeenCalled();

    await wrapper.setProps({
      settings: { ...settings, defaultDeviceLimit: 30 }
    });
    expect((limit.element as HTMLInputElement).value).toBe("20");

    await wrapper.get('[data-save-settings]').trigger("click");
    expect(admin.saveSettings).toHaveBeenCalledWith(expect.objectContaining({
      defaultDeviceLimit: 20
    }));
  });

  it("uses backend-generated invites and accessible invite actions", async () => {
    const pinia = createPinia();
    const wrapper = mount(AdminSettingsTab, {
      props: { settings, invites },
      global: { plugins: [pinia] }
    });
    const admin = useAdminStore();
    admin.createInvite = vi.fn().mockResolvedValue(undefined);

    await wrapper.get('[data-create-invite]').trigger("click");

    expect(admin.createInvite).toHaveBeenCalledWith();
    expect(wrapper.get('[aria-label="复制邀请码 CHG-SERVER-CODE"]').attributes("title")).toContain("复制邀请码");
    expect(wrapper.get('[aria-label="删除邀请码 CHG-SERVER-CODE"]').attributes("title")).toContain("删除邀请码");
    expect(wrapper.text()).not.toContain("远端成功率");
    expect(wrapper.text()).not.toContain("待处理异常");
  });
});
