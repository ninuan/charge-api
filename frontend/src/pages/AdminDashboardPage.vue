<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { createDiscreteApi } from "naive-ui";
import { Activity } from "@lucide/vue";
import AdminConsoleShell from "@/components/admin/AdminConsoleShell.vue";
import AdminHealthStrip from "@/components/admin/AdminHealthStrip.vue";
import AdminOverviewTab from "@/components/admin/AdminOverviewTab.vue";
import AdminSettingsTab from "@/components/admin/AdminSettingsTab.vue";
import AdminUserDrawer from "@/components/admin/AdminUserDrawer.vue";
import AdminUsersTab from "@/components/admin/AdminUsersTab.vue";
import { Button as UiButton } from "@/components/ui/button";
import { Input as UiInput } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import { useAdminStore } from "@/stores/admin";
import { useAuthStore } from "@/stores/auth";
import { useAdminAutoRefresh } from "@/composables/useAdminAutoRefresh";
import { adminTabQuery, resolveAdminTab, type AdminTab } from "@/lib/adminTabs";
import type { AdminUserSummary, UserRole } from "@/types/dashboard";

const admin = useAdminStore();
const auth = useAuthStore();
const route = useRoute();
const router = useRouter();
const { message } = createDiscreteApi(["message"]);
const createOpen = ref(false);
const creating = ref(false);
const form = reactive({ username: "", password: "", role: "user" as UserRole });
const settingsLoaded = ref(false);
const settingsTab = ref<InstanceType<typeof AdminSettingsTab> | null>(null);
const activeTab = computed(() => resolveAdminTab(route.query.tab));
const pageMeta = computed(() => ({
  overview: {
    eyebrow: "Operations overview",
    title: "运营总览",
    description: "最近 24 小时系统运行、设备请求和当前待处理问题。"
  },
  users: {
    eyebrow: "Account directory",
    title: "用户管理",
    description: "查看账户、扫码凭据、设备额度和当前健康状态。"
  },
  settings: {
    eyebrow: "System policies",
    title: "系统设置",
    description: "管理注册入口、新账户默认权限和长期邀请码。"
  }
}[activeTab.value]));
const selectedUserId = computed(() => typeof route.query.user === "string" ? route.query.user : "");
const selectedUser = computed(() =>
  admin.userPage.items.find((summary) => summary.user.id === selectedUserId.value)
  ?? admin.users.find((summary) => summary.user.id === selectedUserId.value)
  ?? null
);
const autoRefresh = useAdminAutoRefresh(admin.loadOverview);

async function load() {
  switch (activeTab.value) {
  case "settings":
    await Promise.allSettled([admin.loadSettings(), admin.loadInvites()]);
    settingsLoaded.value = true;
    break;
  case "users":
    await admin.loadUserPage();
    break;
  default:
    await admin.loadOverview();
  }
}

async function refreshCurrent() {
  if (activeTab.value === "settings" && settingsTab.value?.isDirty()) {
    message.warning("存在尚未保存的策略修改，请先保存后再刷新");
    return;
  }
  await load();
}

async function selectTab(tab: AdminTab) {
  await router.replace({ query: adminTabQuery(tab) });
}

async function openOverviewUser(userId: string) {
  await router.replace({ query: { tab: "users", user: userId } });
}

async function openUser(summary: AdminUserSummary) {
  await openOverviewUser(summary.user.id);
}

async function closeUser() {
  await router.replace({ query: { tab: "users" } });
}

async function createUser() {
  if (form.username.trim().length < 3 || form.password.length < 8) {
    message.error("用户名至少 3 位，密码至少 8 位");
    return;
  }
  creating.value = true;
  try {
    await admin.create({ username: form.username.trim(), password: form.password, role: form.role });
    await admin.loadUserPage({ page: 1 }, { force: true });
    Object.assign(form, { username: "", password: "", role: "user" });
    createOpen.value = false;
    message.success("用户已创建");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    creating.value = false;
  }
}

watch(activeTab, async (tab) => {
  if (tab === "settings" && settingsLoaded.value) return;
  await load();
});

onMounted(async () => {
  await load();
  autoRefresh.start();
});
onBeforeUnmount(autoRefresh.stop);
</script>

<template>
  <AdminConsoleShell :eyebrow="pageMeta.eyebrow" :title="pageMeta.title" :description="pageMeta.description">
    <template #navigation>
      <nav class="admin-subnav" aria-label="管理员中心导航">
        <button
          v-for="tab in ([
            { value: 'overview', label: '运营总览' },
            { value: 'users', label: '用户管理' },
            { value: 'settings', label: '系统设置' }
          ] as const)"
          :key="tab.value"
          type="button"
          class="admin-subnav-item"
          :data-admin-tab="tab.value"
          :aria-current="activeTab === tab.value ? 'page' : undefined"
          @click="selectTab(tab.value)"
        >
          {{ tab.label }}
        </button>
      </nav>
    </template>

    <template #status>
      <AdminHealthStrip
        v-if="activeTab === 'overview'"
        :health="admin.health"
        :loading="admin.healthLoading"
        :error="admin.healthError"
        @retry="admin.loadHealth"
      />
    </template>

    <template #actions>
      <div class="flex gap-2">
        <UiButton class="admin-refresh-action" variant="outline" aria-label="刷新当前页面数据" :disabled="admin.loading" @click="refreshCurrent">
          <Activity :class="{ 'animate-spin': admin.loading }" /><span>刷新数据</span>
        </UiButton>
        <Dialog v-model:open="createOpen">
          <DialogContent class="max-w-lg">
            <DialogHeader>
              <DialogTitle>创建用户</DialogTitle>
              <DialogDescription>普通用户管理自己的扫码登录和充电桩；管理员进入运营中心维护账户与系统策略。</DialogDescription>
            </DialogHeader>
            <form class="space-y-4" @submit.prevent="createUser">
              <label class="form-field"><span>用户名</span><UiInput v-model="form.username" autocomplete="off" /></label>
              <label class="form-field"><span>初始密码</span><UiInput v-model="form.password" type="password" autocomplete="new-password" /></label>
              <label class="form-field">
                <span>角色</span>
                <select v-model="form.role" class="native-input">
                  <option value="user">普通用户</option>
                  <option value="admin">管理员</option>
                </select>
              </label>
              <DialogFooter>
                <UiButton type="button" variant="ghost" @click="createOpen = false">取消</UiButton>
                <UiButton type="submit" :disabled="creating">{{ creating ? "创建中…" : "确认创建" }}</UiButton>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>
    </template>

    <AdminOverviewTab
      v-if="activeTab === 'overview'"
      :overview="admin.overview"
      :hourly="admin.hourly"
      :daily="admin.daily"
      :issues="admin.exceptions"
      :users="admin.users"
      :stats-loading="admin.statsLoading"
      :stats-error="admin.statsError"
      :last-updated-at="admin.lastUpdatedAt"
      @open-user="openOverviewUser"
      @open-users="selectTab('users')"
      @retry-stats="admin.loadStats"
    />

    <AdminSettingsTab
      v-if="activeTab === 'settings'"
      ref="settingsTab"
      :settings="admin.settings"
      :invites="admin.invites"
    />

    <AdminUsersTab
      v-if="activeTab === 'users'"
      :page="admin.userPage"
      :query="admin.userQuery"
      :loading="admin.usersLoading"
      :prefetching="admin.usersPrefetching"
      :error="admin.usersError"
      @create-user="createOpen = true"
      @select-user="openUser"
      @query-change="admin.loadUserPage($event)"
      @page-change="admin.loadUserPage({ page: $event })"
    />
  </AdminConsoleShell>

  <AdminUserDrawer
    :user="selectedUser"
    :current-user-id="auth.currentUser?.id"
    @close="closeUser"
    @updated="admin.loadUserPage({}, { force: true })"
    @deleted="admin.loadUserPage({}, { force: true }); closeUser()"
  />
</template>
