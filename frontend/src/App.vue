<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import {
  NAlert,
  NButton,
  NCard,
  NConfigProvider,
  NDivider,
  NForm,
  NFormItem,
  NGrid,
  NGridItem,
  NInput,
  NInputNumber,
  NLayout,
  NLayoutContent,
  NModal,
  NSelect,
  NSpace,
  NStatistic,
  NTag,
  createDiscreteApi,
  darkTheme,
} from "naive-ui";
import PileCard from "./components/PileCard.vue";
import { useAuthStore } from "./stores/auth";
import { useDashboardStore } from "./stores/dashboard";
import type { AdminUserSummary, CurrentUser, UserRole } from "./types/dashboard";

const auth = useAuthStore();
const store = useDashboardStore();
const { message } = createDiscreteApi(["message"]);

const adding = ref(false);
const refreshing = ref(false);
const updatingCookie = ref(false);
const cookieModalVisible = ref(false);
const cookieText = ref("");
const loggingIn = ref(false);
const registering = ref(false);
const authMode = ref<"login" | "register">("login");
const adminLoading = ref(false);
const creatingUser = ref(false);
const adminUsers = ref<AdminUserSummary[]>([]);

const loginForm = reactive({
  username: "",
  password: ""
});

const userForm = reactive({
  username: "",
  password: "",
  role: "user" as UserRole
});

const form = reactive({
  id: "",
  name: "",
  number: "",
  openNum: 10,
  status: "在线",
  address: ""
});

const roleOptions = [
  { label: "普通用户", value: "user" },
  { label: "管理员", value: "admin" }
];

const lastRemoteAt = computed(() => formatTime(store.refresh.lastRemoteAt));
const nextRemoteAt = computed(() => formatTime(store.refresh.nextRemoteAt));
const refreshMessageType = computed(() => (store.refresh.cached ? "warning" : "success"));
const adminTotals = computed(() => {
  return adminUsers.value.reduce(
    (totals, summary) => {
      totals.users += 1;
      totals.enabledUsers += summary.user.enabled ? 1 : 0;
      totals.totalRequests += summary.stats.totalRequests;
      totals.remoteFetches += summary.stats.remoteFetches;
      totals.cachedRefreshes += summary.stats.cachedRefreshes;
      totals.failedRequests += summary.stats.failedRequests;
      totals.authFailures += summary.stats.authFailures;
      totals.devices += summary.deviceIds.length;
      return totals;
    },
    {
      users: 0,
      enabledUsers: 0,
      totalRequests: 0,
      remoteFetches: 0,
      cachedRefreshes: 0,
      failedRequests: 0,
      authFailures: 0,
      devices: 0
    }
  );
});

async function login() {
  if (!loginForm.username.trim() || !loginForm.password.trim()) {
    message.error("请输入用户名和密码");
    return;
  }

  loggingIn.value = true;
  try {
    await auth.login(loginForm.username.trim(), loginForm.password);
    loginForm.password = "";
    if (auth.isAdmin) {
      await loadAdminUsers();
    } else {
      await store.fetchSnapshot();
    }
    message.success("登录成功");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    loggingIn.value = false;
  }
}

async function register() {
  if (!loginForm.username.trim() || !loginForm.password.trim()) {
    message.error("请输入用户名和密码");
    return;
  }

  registering.value = true;
  try {
    await auth.register(loginForm.username.trim(), loginForm.password);
    loginForm.password = "";
    await store.fetchSnapshot();
    message.success("注册成功");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    registering.value = false;
  }
}

async function logout() {
  await auth.logout();
  store.reset();
  adminUsers.value = [];
  message.success("已退出登录");
}

async function addPile() {
  if (!form.id.trim()) {
    message.error("请输入设备长ID");
    return;
  }

  adding.value = true;
  try {
    await store.addPile({
      id: form.id.trim(),
      name: form.name.trim() || `新增桩 ${form.id.trim()}`,
      number: form.number.trim(),
      openNum: form.openNum,
      status: form.status.trim() || "在线",
      address: form.address.trim()
    });

    form.id = "";
    form.name = "";
    form.number = "";
    form.openNum = 10;
    form.status = "在线";
    form.address = "";
    message.success("桩已添加");
    if (auth.isAdmin) await loadAdminUsers();
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    adding.value = false;
  }
}

async function removePile(id: string) {
  try {
    await store.deletePile(id);
    message.success("桩已删除");
    if (auth.isAdmin) await loadAdminUsers();
  } catch (error) {
    message.error((error as Error).message);
  }
}

async function refreshStatus() {
  refreshing.value = true;
  try {
    await store.refreshFromCapture();
    message.success(store.refresh.message || "状态已刷新");
    if (auth.isAdmin) await loadAdminUsers();
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    refreshing.value = false;
  }
}

async function updateCookie() {
  if (!cookieText.value.trim()) {
    message.error("请粘贴新的 Cookie");
    return;
  }

  updatingCookie.value = true;
  try {
    await store.updateCookie(cookieText.value.trim());
    cookieText.value = "";
    cookieModalVisible.value = false;
    message.success("Cookie 已更新并验证通过");
    if (auth.isAdmin) await loadAdminUsers();
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    updatingCookie.value = false;
  }
}

async function loadAdminUsers() {
  if (!auth.isAdmin) return;
  adminLoading.value = true;
  try {
    const res = await fetch("/api/admin/users", { credentials: "include" });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error ?? "load users failed");
    }
    adminUsers.value = (await res.json()) as AdminUserSummary[];
  } finally {
    adminLoading.value = false;
  }
}

async function createUser() {
  if (!userForm.username.trim() || !userForm.password.trim()) {
    message.error("请输入用户名和密码");
    return;
  }

  creatingUser.value = true;
  try {
    const res = await fetch("/api/admin/users", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        username: userForm.username.trim(),
        password: userForm.password,
        role: userForm.role
      })
    });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error ?? "create user failed");
    }
    userForm.username = "";
    userForm.password = "";
    userForm.role = "user";
    await loadAdminUsers();
    message.success("用户已创建");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    creatingUser.value = false;
  }
}

async function updateUserEnabled(user: CurrentUser, enabled: boolean) {
  const res = await fetch(`/api/admin/users/${user.id}`, {
    method: "PATCH",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled })
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "update user failed");
  }
}

async function toggleUser(summary: AdminUserSummary) {
  try {
    await updateUserEnabled(summary.user, !summary.user.enabled);
    await loadAdminUsers();
    message.success(summary.user.enabled ? "用户已禁用" : "用户已启用");
  } catch (error) {
    message.error((error as Error).message);
  }
}

async function deleteUser(summary: AdminUserSummary) {
  if (summary.user.id === auth.currentUser?.id) {
    message.error("不能删除当前登录用户");
    return;
  }

  try {
    const res = await fetch(`/api/admin/users/${summary.user.id}`, {
      method: "DELETE",
      credentials: "include"
    });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error ?? "delete user failed");
    }
    await loadAdminUsers();
    message.success("用户已删除");
  } catch (error) {
    message.error((error as Error).message);
  }
}

function formatTime(value?: string) {
  if (!value) return "--";
  return new Date(value).toLocaleTimeString();
}

function formatDateTime(value?: string) {
  if (!value) return "--";
  return new Date(value).toLocaleString();
}

function roleLabel(role: UserRole) {
  return role === "admin" ? "管理员" : "普通用户";
}

onMounted(async () => {
  const user = await auth.fetchMe();
  if (!user) return;
  if (auth.isAdmin) {
    await loadAdminUsers();
  } else {
    await store.fetchSnapshot();
  }
});
</script>

<template>
  <n-config-provider :theme="darkTheme">
    <n-layout class="page-shell">
      <n-layout-content content-style="padding: 24px">
        <section v-if="auth.loading" class="login-shell">
          <n-card class="login-card">正在恢复登录状态...</n-card>
        </section>

        <section v-else-if="!auth.isLoggedIn" class="login-shell">
          <n-card class="login-card" :title="authMode === 'login' ? '登录充电桩看板' : '注册普通用户'">
            <p class="login-hint">
              {{ authMode === 'login' ? '管理员进入流量监控，普通用户进入自己的充电桩看板。' : '注册后会自动登录，使用你自己的 Cookie 和充电桩列表。' }}
            </p>
            <n-form>
              <n-form-item label="用户名">
                <n-input
                  v-model:value="loginForm.username"
                  :placeholder="authMode === 'login' ? 'admin 或你的用户名' : '设置用户名'"
                  @keyup.enter="authMode === 'login' ? login() : register()"
                />
              </n-form-item>
              <n-form-item label="密码">
                <n-input
                  v-model:value="loginForm.password"
                  type="password"
                  show-password-on="mousedown"
                  placeholder="请输入密码"
                  @keyup.enter="authMode === 'login' ? login() : register()"
                />
              </n-form-item>
              <n-button
                v-if="authMode === 'login'"
                type="primary"
                block
                :loading="loggingIn"
                @click="login"
              >
                登录
              </n-button>
              <n-button
                v-else
                type="primary"
                block
                :loading="registering"
                @click="register"
              >
                注册并进入
              </n-button>
              <n-button
                class="mode-switch"
                text
                block
                @click="authMode = authMode === 'login' ? 'register' : 'login'"
              >
                {{ authMode === 'login' ? '没有账号？注册普通用户' : '已有账号？返回登录' }}
              </n-button>
            </n-form>
          </n-card>
        </section>

        <template v-else>
          <header class="topbar">
            <div>
              <h1>{{ auth.isAdmin ? '流量监控大屏' : '充电桩运营看板' }}</h1>
              <p>{{ auth.isAdmin ? '用户访问、远端请求与异常统计' : '远端接口状态监控' }}</p>
            </div>
            <div class="topbar-actions">
              <div class="user-chip">
                <strong>{{ auth.currentUser?.username }}</strong>
                <n-tag size="small" :type="auth.isAdmin ? 'warning' : 'success'">
                  {{ roleLabel(auth.currentUser?.role || 'user') }}
                </n-tag>
              </div>
              <n-button v-if="!auth.isAdmin" type="primary" :loading="refreshing" @click="refreshStatus">
                刷新状态
              </n-button>
              <n-button v-if="!auth.isAdmin" secondary @click="cookieModalVisible = true">
                更新我的 Cookie
              </n-button>
              <n-button tertiary @click="logout">退出</n-button>
            </div>
          </header>

          <template v-if="auth.isAdmin">
          <n-grid :cols="4" :x-gap="12" :y-gap="12" class="stats-grid monitor-grid">
            <n-grid-item>
              <n-card class="monitor-card">
                <n-statistic label="用户总数" :value="adminTotals.users" />
              </n-card>
            </n-grid-item>
            <n-grid-item>
              <n-card class="monitor-card">
                <n-statistic label="远端请求" :value="adminTotals.remoteFetches" />
              </n-card>
            </n-grid-item>
            <n-grid-item>
              <n-card class="monitor-card">
                <n-statistic label="缓存命中" :value="adminTotals.cachedRefreshes" />
              </n-card>
            </n-grid-item>
            <n-grid-item>
              <n-card class="monitor-card danger">
                <n-statistic label="失败 / 鉴权失败" :value="`${adminTotals.failedRequests} / ${adminTotals.authFailures}`" />
              </n-card>
            </n-grid-item>
          </n-grid>

          <n-card class="admin-card" title="用户管理与流量统计">
            <n-form inline>
              <n-form-item label="用户名">
                <n-input v-model:value="userForm.username" placeholder="新用户" />
              </n-form-item>
              <n-form-item label="密码">
                <n-input v-model:value="userForm.password" type="password" placeholder="初始密码" />
              </n-form-item>
              <n-form-item label="角色">
                <n-select v-model:value="userForm.role" :options="roleOptions" style="width: 120px" />
              </n-form-item>
              <n-form-item>
                <n-button type="primary" :loading="creatingUser" @click="createUser">添加用户</n-button>
              </n-form-item>
              <n-form-item>
                <n-button secondary :loading="adminLoading" @click="loadAdminUsers">刷新统计</n-button>
              </n-form-item>
            </n-form>

            <div class="admin-list">
              <div v-for="summary in adminUsers" :key="summary.user.id" class="admin-user-row">
                <div class="admin-user-head">
                  <div>
                    <strong>{{ summary.user.username }}</strong>
                    <span>{{ roleLabel(summary.user.role) }}</span>
                  </div>
                  <div class="admin-user-actions">
                    <n-tag size="small" :type="summary.user.enabled ? 'success' : 'error'">
                      {{ summary.user.enabled ? '启用' : '禁用' }}
                    </n-tag>
                    <n-tag size="small" :type="summary.hasCookie ? 'success' : 'warning'">
                      {{ summary.hasCookie ? '已配置 Cookie' : '未配置 Cookie' }}
                    </n-tag>
                    <n-button size="small" secondary @click="toggleUser(summary)">
                      {{ summary.user.enabled ? '禁用' : '启用' }}
                    </n-button>
                    <n-button
                      size="small"
                      type="error"
                      secondary
                      :disabled="summary.user.id === auth.currentUser?.id"
                      @click="deleteUser(summary)"
                    >
                      删除
                    </n-button>
                  </div>
                </div>

                <div class="traffic-grid">
                  <span>总请求 {{ summary.stats.totalRequests }}</span>
                  <span>刷新 {{ summary.stats.refreshRequests }}</span>
                  <span>远端请求 {{ summary.stats.remoteFetches }}</span>
                  <span>缓存命中 {{ summary.stats.cachedRefreshes }}</span>
                  <span>失败 {{ summary.stats.failedRequests }}</span>
                  <span>鉴权失败 {{ summary.stats.authFailures }}</span>
                  <span>设备 {{ summary.deviceIds.length }}</span>
                  <span>最近访问 {{ formatDateTime(summary.stats.lastRequestAt) }}</span>
                </div>
              </div>
            </div>
          </n-card>
          </template>

          <template v-else>
          <n-alert
            v-if="store.refresh.message"
            class="refresh-alert"
            :type="refreshMessageType"
            :show-icon="false"
          >
            {{ store.refresh.message }} · 上次远端请求 {{ lastRemoteAt }} · 下次可请求 {{ nextRemoteAt }}
          </n-alert>

          <n-grid :cols="4" :x-gap="12" :y-gap="12" class="stats-grid">
            <n-grid-item>
              <n-card>
                <n-statistic label="充电桩总数" :value="store.stats.pileCount" />
              </n-card>
            </n-grid-item>
            <n-grid-item>
              <n-card>
                <n-statistic label="充电口总数" :value="store.stats.portCount" />
              </n-card>
            </n-grid-item>
            <n-grid-item>
              <n-card>
                <n-statistic label="使用中" :value="store.stats.inUsePortCount" />
              </n-card>
            </n-grid-item>
            <n-grid-item>
              <n-card>
                <n-statistic label="最后更新时间" :value="new Date(store.snapshot.updatedAt).toLocaleTimeString()" />
              </n-card>
            </n-grid-item>
          </n-grid>

          <n-card class="create-card" title="动态新增充电桩">
            <n-form inline>
              <n-form-item label="设备长ID">
                <n-input v-model:value="form.id" placeholder="例如 2601201412385560001" />
              </n-form-item>
              <n-form-item label="名称">
                <n-input v-model:value="form.name" placeholder="如：松园3号楼北侧" />
              </n-form-item>
              <n-form-item label="桩号">
                <n-input v-model:value="form.number" placeholder="可选" />
              </n-form-item>
              <n-form-item label="口数量">
                <n-input-number v-model:value="form.openNum" :min="1" :max="20" />
              </n-form-item>
              <n-form-item label="状态">
                <n-input v-model:value="form.status" placeholder="在线/离线" />
              </n-form-item>
              <n-form-item label="地址">
                <n-input v-model:value="form.address" placeholder="可选" />
              </n-form-item>
              <n-form-item>
                <n-button type="primary" :loading="adding" @click="addPile">添加桩</n-button>
              </n-form-item>
            </n-form>
          </n-card>

          <section class="piles-wrap">
            <n-space vertical :size="14" style="width: 100%">
              <pile-card
                v-for="pile in store.piles"
                :key="pile.id"
                :pile="pile"
                @remove-pile="removePile"
              />
            </n-space>
          </section>

          <n-modal
            v-model:show="cookieModalVisible"
            preset="card"
            title="更新我的远端 Cookie"
            class="cookie-modal"
          >
            <n-alert type="info" :show-icon="false">
              每个用户只会使用自己的 Cookie。保存后后端会立即尝试刷新你的充电桩数据。
            </n-alert>
            <n-divider />
            <n-input
              v-model:value="cookieText"
              type="textarea"
              placeholder="deviceid=...; org=1; wxopenid=...; info=...; verifycode=...; sid=..."
              :autosize="{ minRows: 5, maxRows: 8 }"
            />
            <template #footer>
              <div class="modal-actions">
                <n-button @click="cookieModalVisible = false">取消</n-button>
                <n-button type="primary" :loading="updatingCookie" @click="updateCookie">
                  保存并验证
                </n-button>
              </div>
            </template>
          </n-modal>
          </template>
        </template>
      </n-layout-content>
    </n-layout>
  </n-config-provider>
</template>

<style scoped>
.page-shell {
  min-height: 100vh;
  background:
    linear-gradient(90deg, rgb(255 255 255 / 3%) 1px, transparent 1px),
    linear-gradient(rgb(255 255 255 / 3%) 1px, transparent 1px),
    linear-gradient(180deg, #121416, #181b1e);
  background-size: 28px 28px, 28px 28px, auto;
}

.login-shell {
  min-height: calc(100vh - 48px);
  display: grid;
  place-items: center;
}

.login-card {
  width: min(440px, 100%);
  border-radius: 14px;
  background: linear-gradient(160deg, #1f2724, #151719);
  border: 1px solid rgb(255 255 255 / 10%);
}

.login-hint {
  margin: 0 0 18px;
  color: #9fa7a1;
  line-height: 1.6;
}

.mode-switch {
  margin-top: 12px;
}

.topbar {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  align-items: center;
  margin-bottom: 14px;
}

.topbar h1 {
  margin: 0;
  font-size: 28px;
  letter-spacing: 0;
}

.topbar p {
  margin: 6px 0 0;
  color: #9fa7a1;
}

.topbar-actions,
.modal-actions,
.admin-user-actions {
  display: flex;
  gap: 10px;
  justify-content: flex-end;
  align-items: center;
  flex-wrap: wrap;
}

.user-chip {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgb(255 255 255 / 7%);
}

.admin-card,
.refresh-alert,
.stats-grid,
.create-card {
  margin-bottom: 16px;
}

.monitor-grid :deep(.n-statistic-value) {
  font-size: 30px;
}

.monitor-card {
  border: 1px solid rgb(62 205 145 / 22%);
  background: linear-gradient(160deg, rgb(42 80 64 / 65%), rgb(24 28 29 / 90%));
}

.monitor-card.danger {
  border-color: rgb(255 111 97 / 24%);
  background: linear-gradient(160deg, rgb(88 48 43 / 65%), rgb(24 28 29 / 90%));
}

.admin-list {
  display: grid;
  gap: 10px;
}

.admin-user-row {
  padding: 14px;
  border-radius: 10px;
  background: rgb(255 255 255 / 5%);
  border: 1px solid rgb(255 255 255 / 8%);
}

.admin-user-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: center;
}

.admin-user-head strong {
  display: block;
  font-size: 16px;
}

.admin-user-head span {
  color: #aeb8b1;
  font-size: 13px;
}

.traffic-grid {
  margin-top: 12px;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 8px;
  color: #c7d0cb;
  font-size: 12px;
}

.piles-wrap {
  padding-bottom: 24px;
}

.cookie-modal {
  max-width: 720px;
}

@media (max-width: 900px) {
  .topbar,
  .admin-user-head {
    align-items: stretch;
    flex-direction: column;
  }

  .topbar h1 {
    font-size: 24px;
  }

  .topbar-actions,
  .admin-user-actions {
    justify-content: flex-start;
  }
}
</style>
