<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { createDiscreteApi } from "naive-ui";
import { Badge as UiBadge } from "@/components/ui/badge";
import { Button as UiButton } from "@/components/ui/button";
import {
  Card as UiCard,
  CardContent as UiCardContent,
  CardDescription as UiCardDescription,
  CardHeader as UiCardHeader,
  CardTitle as UiCardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input as UiInput } from "@/components/ui/input";
import {
  Table as UiTable,
  TableBody as UiTableBody,
  TableCell as UiTableCell,
  TableHead as UiTableHead,
  TableHeader as UiTableHeader,
  TableRow as UiTableRow,
} from "@/components/ui/table";
import PileCard from "./components/PileCard.vue";
import TurnstileWidget from "./components/TurnstileWidget.vue";
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
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null);
const captchaToken = ref("");
const turnstileEnabled = ref(false);
const turnstileSiteKey = ref("");
const authConfigReady = ref(false);

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
const nextRetryAt = computed(() => formatTime(store.refresh.nextRetryAt));
const refreshMessageType = computed(() => (store.refresh.cached || store.refresh.partial ? "warning" : "success"));
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
  if (turnstileEnabled.value && !captchaToken.value) {
    message.error("请先完成人机验证");
    return;
  }

  loggingIn.value = true;
  try {
    await auth.login(loginForm.username.trim(), loginForm.password, captchaToken.value);
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
    resetCaptcha();
  }
}

async function register() {
  if (!loginForm.username.trim() || !loginForm.password.trim()) {
    message.error("请输入用户名和密码");
    return;
  }
  if (loginForm.password.length < 8) {
    message.error("密码至少需要 8 个字符");
    return;
  }
  if (turnstileEnabled.value && !captchaToken.value) {
    message.error("请先完成人机验证");
    return;
  }

  registering.value = true;
  try {
    await auth.register(loginForm.username.trim(), loginForm.password, captchaToken.value);
    loginForm.password = "";
    await store.fetchSnapshot();
    message.success("注册成功");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    registering.value = false;
    resetCaptcha();
  }
}

function resetCaptcha() {
  captchaToken.value = "";
  turnstileRef.value?.reset();
}

async function loadAuthConfig() {
  try {
    const res = await fetch("/api/auth/config");
    if (!res.ok) {
      throw new Error("load auth config failed");
    }
    const config = (await res.json()) as {
      turnstileEnabled: boolean;
      turnstileSiteKey: string;
    };
    turnstileEnabled.value = config.turnstileEnabled;
    turnstileSiteKey.value = config.turnstileSiteKey;
    authConfigReady.value = true;
  } catch {
    authConfigReady.value = false;
    message.error("安全验证配置加载失败，请刷新页面重试");
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
  await loadAuthConfig();
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
  <main class="app-shell dark">
    <section v-if="auth.loading" class="auth-shell">
      <UiCard class="auth-card">
        <UiCardContent class="auth-loading">正在恢复登录状态...</UiCardContent>
      </UiCard>
    </section>

    <section v-else-if="!auth.isLoggedIn" class="auth-shell">
      <div class="auth-orb one" />
      <div class="auth-orb two" />
      <UiCard class="auth-card">
        <UiCardHeader>
          <p class="eyebrow">Charge Console</p>
          <UiCardTitle>{{ authMode === 'login' ? '登录充电桩看板' : '注册普通用户' }}</UiCardTitle>
          <UiCardDescription>
            {{ authMode === 'login' ? '管理员进入流量监控，普通用户进入自己的充电桩看板。' : '注册后自动登录，使用你自己的 Cookie 和充电桩列表。' }}
          </UiCardDescription>
        </UiCardHeader>
        <UiCardContent>
          <form class="auth-form" @submit.prevent="authMode === 'login' ? login() : register()">
            <label>
              用户名
              <UiInput
                v-model="loginForm.username"
                :placeholder="authMode === 'login' ? '用户名' : '设置用户名'"
              />
            </label>
            <label>
              密码
              <UiInput v-model="loginForm.password" type="password" placeholder="请输入密码" />
            </label>
            <TurnstileWidget
              v-if="turnstileEnabled && turnstileSiteKey"
              ref="turnstileRef"
              :site-key="turnstileSiteKey"
              :action="authMode"
              @verified="captchaToken = $event"
              @expired="captchaToken = ''"
              @error="captchaToken = ''"
            />
            <p v-if="!authConfigReady" class="auth-security-loading">正在加载安全验证...</p>
            <UiButton
              type="submit"
              size="lg"
              :disabled="!authConfigReady || loggingIn || registering || (turnstileEnabled && !captchaToken)"
            >
              <span v-if="authMode === 'login'">{{ loggingIn ? '登录中...' : '登录' }}</span>
              <span v-else>{{ registering ? '注册中...' : '注册并进入' }}</span>
            </UiButton>
            <UiButton
              type="button"
              variant="ghost"
              @click="authMode = authMode === 'login' ? 'register' : 'login'; resetCaptcha()"
            >
              {{ authMode === 'login' ? '没有账号？注册普通用户' : '已有账号？返回登录' }}
            </UiButton>
          </form>
        </UiCardContent>
      </UiCard>
    </section>

    <template v-else>
      <header class="topbar">
        <div>
          <p class="eyebrow">{{ auth.isAdmin ? 'Operations Control' : 'User Dashboard' }}</p>
          <h1>{{ auth.isAdmin ? '流量监控大屏' : '充电桩运营看板' }}</h1>
          <p>{{ auth.isAdmin ? '用户访问、远端请求与异常统计' : '远端接口状态监控' }}</p>
        </div>
        <div class="topbar-actions">
          <div class="user-chip">
            <strong>{{ auth.currentUser?.username }}</strong>
            <UiBadge :variant="auth.isAdmin ? 'secondary' : 'default'">
              {{ roleLabel(auth.currentUser?.role || 'user') }}
            </UiBadge>
          </div>
          <UiButton v-if="!auth.isAdmin" :disabled="refreshing" @click="refreshStatus">
            {{ refreshing ? '刷新中...' : '刷新状态' }}
          </UiButton>
          <UiButton v-if="!auth.isAdmin" variant="secondary" @click="cookieModalVisible = true">
            更新我的 Cookie
          </UiButton>
          <UiButton variant="ghost" @click="logout">退出</UiButton>
        </div>
      </header>

      <section v-if="auth.isAdmin" class="admin-dashboard">
        <div class="admin-hero-panel">
          <div>
            <p class="eyebrow">Realtime Risk Radar</p>
            <h2>流量监控与用户态势</h2>
            <p>独立统计每个用户的访问、缓存命中、远端请求和 Cookie 鉴权异常。</p>
          </div>
          <UiButton variant="secondary" :disabled="adminLoading" @click="loadAdminUsers">
            {{ adminLoading ? '刷新中...' : '刷新统计' }}
          </UiButton>
        </div>

        <div class="admin-metrics-grid">
          <UiCard class="metric-card">
            <UiCardHeader>
              <UiCardDescription>用户总数</UiCardDescription>
              <UiCardTitle>{{ adminTotals.users }}</UiCardTitle>
            </UiCardHeader>
            <UiCardContent>{{ adminTotals.enabledUsers }} 个启用账户</UiCardContent>
          </UiCard>
          <UiCard class="metric-card green">
            <UiCardHeader>
              <UiCardDescription>远端请求</UiCardDescription>
              <UiCardTitle>{{ adminTotals.remoteFetches }}</UiCardTitle>
            </UiCardHeader>
            <UiCardContent>{{ adminTotals.devices }} 个设备被用户添加</UiCardContent>
          </UiCard>
          <UiCard class="metric-card amber">
            <UiCardHeader>
              <UiCardDescription>缓存命中</UiCardDescription>
              <UiCardTitle>{{ adminTotals.cachedRefreshes }}</UiCardTitle>
            </UiCardHeader>
            <UiCardContent>减少重复请求，降低风控风险</UiCardContent>
          </UiCard>
          <UiCard class="metric-card red">
            <UiCardHeader>
              <UiCardDescription>失败 / 鉴权失败</UiCardDescription>
              <UiCardTitle>{{ adminTotals.failedRequests }} / {{ adminTotals.authFailures }}</UiCardTitle>
            </UiCardHeader>
            <UiCardContent>重点关注 Cookie 过期用户</UiCardContent>
          </UiCard>
        </div>

        <UiCard class="console-card">
          <UiCardHeader>
            <UiCardTitle>用户管理</UiCardTitle>
            <UiCardDescription>管理员可以创建、禁用或删除用户；普通用户可自行注册。</UiCardDescription>
          </UiCardHeader>
          <UiCardContent>
            <form class="admin-create-form" @submit.prevent="createUser">
              <label>
                用户名
                <UiInput v-model="userForm.username" placeholder="新用户" />
              </label>
              <label>
                初始密码
                <UiInput v-model="userForm.password" type="password" placeholder="初始密码" />
              </label>
              <label>
                角色
                <select v-model="userForm.role" class="field-select">
                  <option v-for="option in roleOptions" :key="option.value" :value="option.value">
                    {{ option.label }}
                  </option>
                </select>
              </label>
              <UiButton type="submit" :disabled="creatingUser">
                {{ creatingUser ? '创建中...' : '添加用户' }}
              </UiButton>
            </form>

            <div class="table-wrap">
              <UiTable>
                <UiTableHeader>
                  <UiTableRow>
                    <UiTableHead>用户</UiTableHead>
                    <UiTableHead>状态</UiTableHead>
                    <UiTableHead>Cookie</UiTableHead>
                    <UiTableHead>请求</UiTableHead>
                    <UiTableHead>远端 / 缓存</UiTableHead>
                    <UiTableHead>失败</UiTableHead>
                    <UiTableHead>设备</UiTableHead>
                    <UiTableHead>最近访问</UiTableHead>
                    <UiTableHead class="text-right">操作</UiTableHead>
                  </UiTableRow>
                </UiTableHeader>
                <UiTableBody>
                  <UiTableRow v-for="summary in adminUsers" :key="summary.user.id">
                    <UiTableCell>
                      <div class="user-cell">
                        <strong>{{ summary.user.username }}</strong>
                        <span>{{ roleLabel(summary.user.role) }}</span>
                      </div>
                    </UiTableCell>
                    <UiTableCell>
                      <UiBadge :variant="summary.user.enabled ? 'default' : 'destructive'">
                        {{ summary.user.enabled ? '启用' : '禁用' }}
                      </UiBadge>
                    </UiTableCell>
                    <UiTableCell>
                      <UiBadge :variant="summary.hasCookie ? 'secondary' : 'outline'">
                        {{ summary.hasCookie ? '已配置' : '未配置' }}
                      </UiBadge>
                    </UiTableCell>
                    <UiTableCell>{{ summary.stats.totalRequests }}</UiTableCell>
                    <UiTableCell>{{ summary.stats.remoteFetches }} / {{ summary.stats.cachedRefreshes }}</UiTableCell>
                    <UiTableCell>{{ summary.stats.failedRequests }} / {{ summary.stats.authFailures }}</UiTableCell>
                    <UiTableCell>{{ summary.deviceIds.length }}</UiTableCell>
                    <UiTableCell>{{ formatDateTime(summary.stats.lastRequestAt) }}</UiTableCell>
                    <UiTableCell>
                      <div class="row-actions">
                        <UiButton variant="secondary" size="sm" @click="toggleUser(summary)">
                          {{ summary.user.enabled ? '禁用' : '启用' }}
                        </UiButton>
                        <UiButton
                          variant="destructive"
                          size="sm"
                          :disabled="summary.user.id === auth.currentUser?.id"
                          @click="deleteUser(summary)"
                        >
                          删除
                        </UiButton>
                      </div>
                    </UiTableCell>
                  </UiTableRow>
                </UiTableBody>
              </UiTable>
            </div>
          </UiCardContent>
        </UiCard>
      </section>

      <section v-else class="user-dashboard">
        <div v-if="store.refresh.message" :class="['refresh-alert', refreshMessageType]">
          {{ store.refresh.message }} · 上次远端请求 {{ lastRemoteAt }} · 下次可请求 {{ nextRemoteAt }}
          <span v-if="store.refresh.nextRetryAt"> · 最早退避重试 {{ nextRetryAt }}</span>
        </div>

        <div class="user-metrics-grid">
          <UiCard class="metric-card">
            <UiCardHeader>
              <UiCardDescription>充电桩总数</UiCardDescription>
              <UiCardTitle>{{ store.stats.pileCount }}</UiCardTitle>
            </UiCardHeader>
          </UiCard>
          <UiCard class="metric-card green">
            <UiCardHeader>
              <UiCardDescription>充电口总数</UiCardDescription>
              <UiCardTitle>{{ store.stats.portCount }}</UiCardTitle>
            </UiCardHeader>
          </UiCard>
          <UiCard class="metric-card amber">
            <UiCardHeader>
              <UiCardDescription>使用中</UiCardDescription>
              <UiCardTitle>{{ store.stats.inUsePortCount }}</UiCardTitle>
            </UiCardHeader>
          </UiCard>
          <UiCard class="metric-card">
            <UiCardHeader>
              <UiCardDescription>最后更新时间</UiCardDescription>
              <UiCardTitle class="time-title">{{ new Date(store.snapshot.updatedAt).toLocaleTimeString() }}</UiCardTitle>
            </UiCardHeader>
          </UiCard>
        </div>

        <UiCard class="create-card-modern">
          <UiCardHeader>
            <UiCardTitle>动态新增充电桩</UiCardTitle>
            <UiCardDescription>每个用户最多添加 10 台；刷新采用有限并发，失败设备会保留上次数据并自动退避。</UiCardDescription>
          </UiCardHeader>
          <UiCardContent>
            <form class="pile-form" @submit.prevent="addPile">
              <label>
                设备长ID
                <UiInput v-model="form.id" placeholder="例如 2601201412385560001" />
              </label>
              <label>
                名称
                <UiInput v-model="form.name" placeholder="如：松园3号楼北侧" />
              </label>
              <label>
                桩号
                <UiInput v-model="form.number" placeholder="可选" />
              </label>
              <label>
                口数量
                <input v-model.number="form.openNum" class="field-input" min="1" max="20" type="number">
              </label>
              <label>
                状态
                <UiInput v-model="form.status" placeholder="在线/离线" />
              </label>
              <label>
                地址
                <UiInput v-model="form.address" placeholder="可选" />
              </label>
              <UiButton type="submit" :disabled="adding">
                {{ adding ? '添加中...' : '添加桩' }}
              </UiButton>
            </form>
          </UiCardContent>
        </UiCard>

        <div class="piles-wrap">
          <PileCard
            v-for="pile in store.piles"
            :key="pile.id"
            :pile="pile"
            @remove-pile="removePile"
          />
          <UiCard v-if="store.piles.length === 0" class="empty-card">
            <UiCardHeader>
              <UiCardTitle>还没有充电桩</UiCardTitle>
              <UiCardDescription>先更新你的 Cookie，再添加一个设备长 ID 开始监控。</UiCardDescription>
            </UiCardHeader>
          </UiCard>
        </div>

        <Dialog v-model:open="cookieModalVisible">
          <DialogContent class="cookie-dialog">
            <DialogHeader>
              <DialogTitle>更新我的远端 Cookie</DialogTitle>
              <DialogDescription>
                每个用户只会使用自己的 Cookie。保存后后端会立即尝试刷新你的充电桩数据。
              </DialogDescription>
            </DialogHeader>
            <textarea
              v-model="cookieText"
              class="cookie-textarea"
              placeholder="deviceid=...; org=1; wxopenid=...; info=...; verifycode=...; sid=..."
            />
            <DialogFooter>
              <UiButton variant="ghost" @click="cookieModalVisible = false">取消</UiButton>
              <UiButton :disabled="updatingCookie" @click="updateCookie">
                {{ updatingCookie ? '验证中...' : '保存并验证' }}
              </UiButton>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </section>
    </template>
  </main>
</template>

<style scoped>
.app-shell {
  min-height: 100vh;
  padding: 24px;
  color: hsl(var(--foreground));
  background:
    radial-gradient(circle at 8% 10%, rgb(91 140 110 / 22%), transparent 28%),
    radial-gradient(circle at 90% 0%, rgb(220 148 78 / 14%), transparent 30%),
    linear-gradient(90deg, rgb(255 255 255 / 3%) 1px, transparent 1px),
    linear-gradient(rgb(255 255 255 / 3%) 1px, transparent 1px),
    linear-gradient(180deg, #121416, #181b1e);
  background-size: auto, auto, 28px 28px, 28px 28px, auto;
}

.auth-shell {
  position: relative;
  min-height: calc(100vh - 48px);
  display: grid;
  place-items: center;
  overflow: hidden;
}

.auth-orb {
  position: absolute;
  width: 260px;
  height: 260px;
  border-radius: 999px;
  filter: blur(12px);
  opacity: 0.35;
}

.auth-orb.one {
  left: 12%;
  top: 12%;
  background: #4eb27d;
}

.auth-orb.two {
  right: 16%;
  bottom: 18%;
  background: #c58b4a;
}

.auth-card {
  position: relative;
  z-index: 1;
  width: min(460px, 100%);
  border-color: rgb(255 255 255 / 12%);
  background:
    linear-gradient(180deg, rgb(255 255 255 / 8%), transparent),
    rgb(18 22 21 / 92%);
  box-shadow: 0 24px 90px rgb(0 0 0 / 34%);
}

.auth-loading {
  color: #b7c1bb;
}

.auth-form,
.admin-create-form,
.pile-form {
  display: grid;
  gap: 12px;
}

.auth-form label,
.admin-create-form label,
.pile-form label {
  display: grid;
  gap: 7px;
  color: #bdc7c0;
  font-size: 12px;
  font-weight: 600;
}

.auth-security-loading {
  margin: 0;
  color: #9fa7a1;
  font-size: 12px;
  text-align: center;
}

.eyebrow {
  margin: 0 0 8px;
  color: #9dd6b2;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.topbar {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  align-items: center;
  max-width: 1440px;
  margin: 0 auto 18px;
}

.topbar h1 {
  margin: 0;
  font-size: clamp(28px, 4vw, 46px);
  line-height: 1;
  letter-spacing: -0.045em;
}

.topbar p {
  margin: 8px 0 0;
  color: #9fa7a1;
}

.topbar-actions {
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
  border: 1px solid rgb(255 255 255 / 10%);
  border-radius: 999px;
  background: rgb(255 255 255 / 7%);
}

.admin-dashboard,
.user-dashboard {
  max-width: 1440px;
  margin: 0 auto;
  display: grid;
  gap: 18px;
}

.admin-hero-panel {
  position: relative;
  overflow: hidden;
  display: flex;
  justify-content: space-between;
  gap: 18px;
  align-items: flex-end;
  padding: 26px;
  border: 1px solid rgb(255 255 255 / 10%);
  border-radius: 24px;
  background:
    radial-gradient(circle at top right, rgb(85 130 101 / 42%), transparent 34%),
    linear-gradient(135deg, rgb(37 43 39 / 96%), rgb(15 18 18 / 96%));
  box-shadow: 0 18px 70px rgb(0 0 0 / 28%);
}

.admin-hero-panel h2 {
  margin: 4px 0 8px;
  font-size: clamp(30px, 5vw, 54px);
  line-height: 1;
  letter-spacing: -0.055em;
}

.admin-hero-panel p {
  margin: 0;
  color: #afbab3;
}

.admin-metrics-grid,
.user-metrics-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.metric-card {
  border-color: rgb(255 255 255 / 10%);
  background:
    linear-gradient(180deg, rgb(255 255 255 / 8%), transparent),
    rgb(23 26 25 / 92%);
}

.metric-card.green {
  background:
    radial-gradient(circle at top right, rgb(58 171 116 / 24%), transparent 46%),
    rgb(23 26 25 / 92%);
}

.metric-card.amber {
  background:
    radial-gradient(circle at top right, rgb(218 161 70 / 24%), transparent 46%),
    rgb(23 26 25 / 92%);
}

.metric-card.red {
  background:
    radial-gradient(circle at top right, rgb(223 92 79 / 24%), transparent 46%),
    rgb(23 26 25 / 92%);
}

.metric-card :deep([data-slot="card-title"]) {
  font-size: 34px;
  letter-spacing: -0.04em;
}

.metric-card :deep([data-slot="card-content"]) {
  color: #aeb8b1;
  font-size: 13px;
}

.time-title {
  font-size: 24px !important;
}

.console-card,
.create-card-modern,
.empty-card {
  border-color: rgb(255 255 255 / 10%);
  background: rgb(17 20 20 / 94%);
}

.admin-create-form {
  grid-template-columns: minmax(180px, 1fr) minmax(180px, 1fr) minmax(140px, 180px) auto;
  align-items: end;
  margin-bottom: 18px;
}

.pile-form {
  grid-template-columns: repeat(6, minmax(130px, 1fr)) auto;
  align-items: end;
}

.field-select,
.field-input,
.cookie-textarea {
  width: 100%;
  border: 1px solid hsl(var(--input));
  border-radius: 8px;
  background: rgb(255 255 255 / 4%);
  color: hsl(var(--foreground));
  outline: none;
}

.field-select,
.field-input {
  height: 32px;
  padding: 0 10px;
}

.table-wrap {
  overflow-x: auto;
  border: 1px solid rgb(255 255 255 / 8%);
  border-radius: 14px;
}

.user-cell {
  display: grid;
  gap: 2px;
}

.user-cell strong {
  font-size: 14px;
}

.user-cell span {
  color: #9fa7a1;
  font-size: 12px;
}

.row-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.refresh-alert {
  padding: 12px 14px;
  border-radius: 14px;
  color: #e8f2ec;
  border: 1px solid rgb(255 255 255 / 10%);
  background: rgb(255 255 255 / 6%);
}

.refresh-alert.warning {
  border-color: rgb(226 173 83 / 28%);
  background: rgb(226 173 83 / 10%);
}

.refresh-alert.success {
  border-color: rgb(83 197 129 / 26%);
  background: rgb(83 197 129 / 10%);
}

.piles-wrap {
  display: grid;
  gap: 14px;
  padding-bottom: 24px;
}

.cookie-dialog {
  max-width: 720px !important;
}

.cookie-textarea {
  min-height: 160px;
  resize: vertical;
  padding: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  line-height: 1.5;
}

@media (max-width: 1100px) {
  .admin-metrics-grid,
  .user-metrics-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .pile-form,
  .admin-create-form {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 760px) {
  .app-shell {
    padding: 16px;
  }

  .topbar,
  .admin-hero-panel {
    align-items: stretch;
    flex-direction: column;
  }

  .topbar-actions,
  .row-actions {
    justify-content: flex-start;
  }

  .admin-metrics-grid,
  .user-metrics-grid,
  .pile-form,
  .admin-create-form {
    grid-template-columns: 1fr;
  }
}
</style>
