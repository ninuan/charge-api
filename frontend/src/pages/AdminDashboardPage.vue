<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { createDiscreteApi } from "naive-ui";
import {
  Activity,
  AlertTriangle,
  Ban,
  Check,
  Copy,
  DatabaseZap,
  KeyRound,
  Plus,
  Power,
  RefreshCw,
  Save,
  Search,
  Settings2,
  ShieldAlert,
  Trash2,
  UserCheck,
  UsersRound
} from "@lucide/vue";
import AppShell from "@/components/AppShell.vue";
import AdminTrafficChart from "@/components/AdminTrafficChart.vue";
import MetricCard from "@/components/MetricCard.vue";
import { Badge as UiBadge } from "@/components/ui/badge";
import { Button as UiButton } from "@/components/ui/button";
import { Input as UiInput } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from "@/components/ui/dialog";
import {
  Table as UiTable,
  TableBody as UiTableBody,
  TableCell as UiTableCell,
  TableHead as UiTableHead,
  TableHeader as UiTableHeader,
  TableRow as UiTableRow
} from "@/components/ui/table";
import { useAdminStore } from "@/stores/admin";
import { useAuthStore } from "@/stores/auth";
import type { AdminUserSummary, UserRole } from "@/types/dashboard";

const admin = useAdminStore();
const auth = useAuthStore();
const { message } = createDiscreteApi(["message"]);
const search = ref("");
const filter = ref<"all" | "risk" | "disabled">("all");
const createOpen = ref(false);
const creating = ref(false);
const deleteTarget = ref<AdminUserSummary | null>(null);
const resetTarget = ref<AdminUserSummary | null>(null);
const resetPassword = ref("");
const form = reactive({ username: "", password: "", role: "user" as UserRole });
const savingSettings = ref(false);
const creatingInvite = ref(false);
const settingsDraft = reactive({
  openRegistration: true,
  inviteRequired: true,
  defaultDeviceLimit: 10,
  defaultRefreshEnabled: true,
  statsRetentionDays: 90
});

watch(() => admin.settings, (value) => {
  if (value) Object.assign(settingsDraft, value);
}, { deep: true });

const filteredUsers = computed(() => admin.users.filter((summary) => {
  const matchesSearch = summary.user.username.toLowerCase().includes(search.value.trim().toLowerCase());
  const risky = !summary.hasCookie || summary.stats.authFailures > 0 || summary.stats.failedRequests > 0;
  if (filter.value === "risk") return matchesSearch && risky;
  if (filter.value === "disabled") return matchesSearch && !summary.user.enabled;
  return matchesSearch;
}));

const riskUsers = computed(() => admin.users
  .filter((summary) => !summary.hasCookie || summary.stats.authFailures > 0)
  .sort((a, b) => b.stats.authFailures - a.stats.authFailures)
  .slice(0, 5));
const hourlyTrend = computed(() => admin.hourly.slice(-12));
const trendMax = computed(() => Math.max(1, ...hourlyTrend.value.map((point) => point.requests)));
const cacheRate = computed(() => admin.totals.refreshRequests
  ? Math.round(admin.totals.cachedRefreshes / admin.totals.refreshRequests * 100) : 0);
const remoteSuccessRate = computed(() => {
  const total = admin.hourly.reduce((sum, point) => sum + point.remote, 0);
  const ok = admin.hourly.reduce((sum, point) => sum + point.remoteOk, 0);
  return total ? Math.round(ok / total * 100) : 0;
});

function formatDateTime(value?: string) {
  return value ? new Date(value).toLocaleString("zh-CN") : "暂无访问";
}

async function load() {
  try {
    await admin.load();
  } catch (error) {
    message.error((error as Error).message);
  }
}

async function createUser() {
  if (form.username.trim().length < 3 || form.password.length < 8) {
    message.error("用户名至少 3 位，密码至少 8 位");
    return;
  }
  creating.value = true;
  try {
    await admin.create({ username: form.username.trim(), password: form.password, role: form.role });
    Object.assign(form, { username: "", password: "", role: "user" });
    createOpen.value = false;
    message.success("用户已创建");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    creating.value = false;
  }
}

async function toggle(summary: AdminUserSummary) {
  try {
    await admin.setEnabled(summary.user, !summary.user.enabled);
    message.success(summary.user.enabled ? "用户已禁用" : "用户已启用");
  } catch (error) {
    message.error((error as Error).message);
  }
}

async function remove() {
  if (!deleteTarget.value) return;
  try {
    await admin.remove(deleteTarget.value.user);
    deleteTarget.value = null;
    message.success("用户已删除");
  } catch (error) {
    message.error((error as Error).message);
  }
}

async function saveSettings() {
  savingSettings.value = true;
  try {
    await admin.saveSettings({ ...settingsDraft });
    message.success("系统策略已保存");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    savingSettings.value = false;
  }
}

async function addInvite() {
  creatingInvite.value = true;
  try {
    await admin.createInvite();
    message.success("已生成新的随机邀请码");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    creatingInvite.value = false;
  }
}

async function copyInvite(code: string) {
  try {
    await navigator.clipboard.writeText(code);
    message.success("邀请码已复制");
  } catch {
    message.error("复制失败，请手动选择邀请码");
  }
}

async function updateQuota(summary: AdminUserSummary, event: Event) {
  const deviceLimit = Number((event.target as HTMLInputElement).value);
  try {
    await admin.updateUser(summary.user, { deviceLimit });
    message.success("设备额度已更新");
  } catch (error) {
    message.error((error as Error).message);
  }
}

async function toggleRefresh(summary: AdminUserSummary) {
  try {
    await admin.updateUser(summary.user, { refreshEnabled: !summary.user.refreshEnabled });
    message.success(summary.user.refreshEnabled ? "已暂停远端刷新" : "已恢复远端刷新");
  } catch (error) {
    message.error((error as Error).message);
  }
}

async function submitResetPassword() {
  if (!resetTarget.value || resetPassword.value.length < 8) {
    message.error("新密码至少需要 8 个字符");
    return;
  }
  try {
    await admin.updateUser(resetTarget.value.user, { password: resetPassword.value });
    resetTarget.value = null;
    resetPassword.value = "";
    message.success("密码已重置，旧会话已全部撤销");
  } catch (error) {
    message.error((error as Error).message);
  }
}

onMounted(load);
</script>

<template>
  <AppShell title="流量监控中心" description="聚合用户访问、远端请求、缓存命中和鉴权异常，快速定位可能触发风控或凭据失效的账户。">
    <template #heading-actions>
      <div class="flex gap-2">
        <UiButton variant="outline" :disabled="admin.loading" @click="load">
          <Activity :class="{ 'animate-spin': admin.loading }" />刷新统计
        </UiButton>
        <Dialog v-model:open="createOpen">
          <DialogTrigger as-child><UiButton><Plus />添加用户</UiButton></DialogTrigger>
          <DialogContent class="max-w-lg">
            <DialogHeader>
              <DialogTitle>创建用户</DialogTitle>
              <DialogDescription>普通用户管理自己的 Cookie 和设备；管理员只能进入流量监控中心。</DialogDescription>
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

    <section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label="流量指标">
      <MetricCard label="用户总数" :value="admin.totals.users" :detail="`${admin.totals.enabledUsers} 个账户已启用`" :icon="UsersRound" />
      <MetricCard label="远端请求" :value="admin.totals.remoteFetches" :detail="`${remoteSuccessRate}% 远端成功率`" tone="green" :icon="DatabaseZap" />
      <MetricCard label="缓存命中" :value="admin.totals.cachedRefreshes" :detail="`${cacheRate}% 缓存命中率`" tone="amber" :icon="UserCheck" />
      <MetricCard label="鉴权失败" :value="admin.totals.authFailures" :detail="`${admin.totals.failedRequests} 次请求失败`" tone="red" :icon="ShieldAlert" />
    </section>

    <section class="mt-7 grid gap-4 xl:grid-cols-2">
      <article class="surface-panel p-5 sm:p-6">
        <div>
          <div>
            <p class="section-kicker">访问策略</p>
            <h2 class="mt-2 flex items-center gap-2 text-xl font-bold"><Settings2 class="size-5" />注册与资源策略</h2>
            <p class="mt-2 text-sm leading-6 text-muted-foreground">控制注册入口，以及新账户默认拥有的设备额度和刷新权限。</p>
          </div>
        </div>
        <div class="mt-5 space-y-4">
          <fieldset>
            <legend class="setting-group-label">注册入口</legend>
            <div class="mt-2 grid items-start gap-3 sm:grid-cols-2">
              <label class="setting-option">
                <span><strong>允许公开注册</strong><small>无需邀请码即可创建账户</small></span>
                <input v-model="settingsDraft.openRegistration" class="setting-checkbox" type="checkbox">
              </label>
              <label class="setting-option">
                <span><strong>允许邀请码注册</strong><small>关闭公开注册后仍可凭邀请码加入</small></span>
                <input v-model="settingsDraft.inviteRequired" class="setting-checkbox" type="checkbox">
              </label>
            </div>
          </fieldset>

          <fieldset>
            <legend class="setting-group-label">新账户默认权限</legend>
            <label class="setting-option setting-option--compact mt-2">
              <span><strong>允许远端刷新</strong><small>仅影响之后创建的新账户，已有账户可单独调整</small></span>
              <input v-model="settingsDraft.defaultRefreshEnabled" class="setting-checkbox" type="checkbox">
            </label>
          </fieldset>

          <fieldset>
            <legend class="setting-group-label">资源与数据</legend>
            <div class="mt-2 grid items-start gap-3 sm:grid-cols-2">
              <label class="form-field setting-field"><span>新用户默认设备额度</span><input v-model.number="settingsDraft.defaultDeviceLimit" class="native-input" min="1" max="100" type="number"><small>可在用户列表中单独覆盖。</small></label>
              <label class="form-field setting-field"><span>统计数据保留时间</span><input v-model.number="settingsDraft.statsRetentionDays" class="native-input" min="1" max="365" type="number"><small>单位：天，保存后自动清理更早数据。</small></label>
            </div>
          </fieldset>
        </div>
        <div class="panel-action-bar">
          <p><Check aria-hidden="true" />修改不会自动保存</p>
          <UiButton :disabled="savingSettings" @click="saveSettings">
            <Save />
            {{ savingSettings ? "保存中…" : "保存策略" }}
          </UiButton>
        </div>
      </article>

      <article class="surface-panel p-5 sm:p-6">
        <div>
          <p class="section-kicker">邀请管理</p>
          <h2 class="mt-2 flex items-center gap-2 text-xl font-bold"><KeyRound class="size-5" />长期邀请码</h2>
          <p class="mt-2 text-sm leading-6 text-muted-foreground">系统自动生成高强度随机邀请码。邀请码长期有效，可创建多个并查看使用次数。</p>
        </div>
        <div class="mt-5">
          <UiButton :disabled="creatingInvite" @click="addInvite">
            <RefreshCw :class="{ 'animate-spin': creatingInvite }" />
            {{ creatingInvite ? "生成中…" : "生成随机邀请码" }}
          </UiButton>
        </div>
        <div class="mt-4 max-h-56 space-y-2 overflow-auto">
          <div v-for="invite in admin.invites" :key="invite.id" class="invite-item">
            <div class="min-w-0"><p class="truncate font-mono text-sm font-bold">{{ invite.code }}</p><p class="mt-1 text-xs text-muted-foreground">长期有效 · 已使用 {{ invite.usedCount }} 次</p></div>
            <div class="flex shrink-0 gap-1">
              <UiButton variant="ghost" size="icon-sm" aria-label="复制邀请码" title="复制邀请码" @click="copyInvite(invite.code)"><Copy /></UiButton>
              <UiButton variant="ghost" size="icon-sm" aria-label="删除邀请码" title="删除邀请码" @click="admin.removeInvite(invite.id)"><Trash2 /></UiButton>
            </div>
          </div>
          <p v-if="!admin.invites.length" class="text-sm text-muted-foreground">尚未创建邀请码，当前用户无法完成邀请注册。</p>
        </div>
      </article>
    </section>

    <section class="mt-7 grid gap-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(320px,.7fr)]">
      <article class="surface-panel p-5 sm:p-6">
        <div class="flex items-start justify-between gap-4">
          <div>
            <p class="section-kicker">Traffic composition</p>
            <h2 class="mt-2 text-xl font-bold">请求结构</h2>
          </div>
          <UiBadge variant="outline">{{ admin.totals.totalRequests }} 次访问</UiBadge>
        </div>
        <AdminTrafficChart
          :remote="admin.totals.remoteFetches"
          :cached="admin.totals.cachedRefreshes"
          :failed="admin.totals.failedRequests"
        />
        <div class="mt-5 border-t border-border pt-5">
          <div class="flex items-center justify-between gap-3">
            <h3 class="text-sm font-bold">近 12 小时请求趋势</h3>
            <span class="text-xs text-muted-foreground">保留 {{ admin.settings?.statsRetentionDays ?? 90 }} 天</span>
          </div>
          <div v-if="hourlyTrend.length" class="mt-4 flex h-28 items-end gap-1.5" aria-label="近十二小时请求趋势">
            <div v-for="point in hourlyTrend" :key="point.time" class="group flex min-w-0 flex-1 flex-col items-center justify-end gap-1">
              <span class="text-xs tabular-nums text-muted-foreground">{{ point.requests }}</span>
              <div class="w-full rounded-t bg-primary/75" :style="{ height: `${Math.max(6, point.requests / trendMax * 76)}px` }" />
              <span class="text-[11px] text-muted-foreground">{{ new Date(point.time).getHours() }}时</span>
            </div>
          </div>
          <p v-else class="mt-5 text-sm text-muted-foreground">统计数据将在用户产生请求后显示。</p>
        </div>
      </article>

      <article class="surface-panel overflow-hidden">
        <header class="border-b border-border p-5">
          <p class="section-kicker text-destructive">风险概览</p>
          <h2 class="mt-2 flex items-center gap-2 text-xl font-bold"><AlertTriangle class="size-5 text-destructive" />需要关注</h2>
        </header>
        <div v-if="riskUsers.length" class="divide-y divide-border">
          <div v-for="summary in riskUsers" :key="summary.user.id" class="flex items-center justify-between gap-3 p-4">
            <div class="min-w-0">
              <p class="truncate font-semibold">{{ summary.user.username }}</p>
              <p class="mt-1 text-xs text-muted-foreground">
                {{ !summary.hasCookie ? "未配置 Cookie" : `${summary.stats.authFailures} 次鉴权失败` }}
              </p>
            </div>
            <UiBadge :variant="summary.hasCookie ? 'destructive' : 'outline'">{{ summary.hasCookie ? "异常" : "待配置" }}</UiBadge>
          </div>
        </div>
        <div v-else class="empty-compact">当前没有需要关注的账户。</div>
      </article>
    </section>

    <section class="surface-panel mt-4 overflow-hidden">
      <header class="border-b border-border p-5">
        <p class="section-kicker text-destructive">异常中心</p>
        <h2 class="mt-2 flex items-center gap-2 text-xl font-bold"><AlertTriangle class="size-5 text-destructive" />异常中心</h2>
        <p class="mt-1 text-sm text-muted-foreground">集中显示 Cookie、刷新和设备异常，当前仅提供站内提示。</p>
      </header>
      <div v-if="admin.exceptions.length" class="divide-y divide-border">
        <div v-for="item in admin.exceptions" :key="item.id" class="flex flex-col justify-between gap-2 p-4 sm:flex-row sm:items-center">
          <div><p class="font-semibold">{{ item.username }} · {{ item.message }}</p><p class="mt-1 text-xs text-muted-foreground">{{ new Date(item.time).toLocaleString("zh-CN") }}</p></div>
          <UiBadge :variant="item.level === 'critical' ? 'destructive' : 'outline'">{{ item.level === "critical" ? "紧急" : "注意" }}</UiBadge>
        </div>
      </div>
      <div v-else class="empty-compact">当前没有需要处理的异常。</div>
    </section>

    <section class="surface-panel mt-4 overflow-hidden">
      <header class="flex flex-col justify-between gap-4 border-b border-border p-5 lg:flex-row lg:items-center">
        <div>
          <h2 class="text-xl font-bold">用户管理</h2>
          <p class="mt-1 text-sm text-muted-foreground">搜索、筛选并管理账户状态。</p>
        </div>
        <div class="flex flex-col gap-2 sm:flex-row">
          <label class="relative">
            <Search class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <UiInput v-model="search" class="pl-10 sm:w-64" aria-label="搜索用户名" placeholder="搜索用户名" />
          </label>
          <select v-model="filter" class="native-input sm:w-36" aria-label="筛选用户">
            <option value="all">全部用户</option>
            <option value="risk">风险用户</option>
            <option value="disabled">已禁用</option>
          </select>
        </div>
      </header>

      <div class="hidden overflow-x-auto lg:block">
        <UiTable>
          <UiTableHeader>
            <UiTableRow>
              <UiTableHead>用户</UiTableHead><UiTableHead>状态</UiTableHead><UiTableHead>凭据</UiTableHead>
              <UiTableHead>访问总数</UiTableHead><UiTableHead>远端 / 缓存</UiTableHead><UiTableHead>失败 / 鉴权</UiTableHead>
              <UiTableHead>设备 / 额度</UiTableHead><UiTableHead>远端刷新</UiTableHead><UiTableHead>最近访问</UiTableHead><UiTableHead class="text-right">操作</UiTableHead>
            </UiTableRow>
          </UiTableHeader>
          <UiTableBody>
            <UiTableRow v-for="summary in filteredUsers" :key="summary.user.id">
              <UiTableCell><strong>{{ summary.user.username }}</strong><p class="text-xs text-muted-foreground">{{ summary.user.role === "admin" ? "管理员" : "普通用户" }}</p></UiTableCell>
              <UiTableCell><UiBadge :variant="summary.user.enabled ? 'secondary' : 'destructive'">{{ summary.user.enabled ? "启用" : "禁用" }}</UiBadge></UiTableCell>
              <UiTableCell>{{ summary.hasCookie ? "已配置" : "未配置" }}</UiTableCell>
              <UiTableCell class="font-mono tabular-nums">{{ summary.stats.totalRequests }}</UiTableCell>
              <UiTableCell class="font-mono tabular-nums">{{ summary.stats.remoteFetches }} / {{ summary.stats.cachedRefreshes }}</UiTableCell>
              <UiTableCell class="font-mono tabular-nums">{{ summary.stats.failedRequests }} / {{ summary.stats.authFailures }}</UiTableCell>
              <UiTableCell>
                <div class="flex items-center gap-2"><span>{{ summary.deviceIds.length }} /</span><input class="native-input h-9 w-16 px-2" type="number" min="1" max="100" :value="summary.user.deviceLimit" @change="updateQuota(summary, $event)"></div>
              </UiTableCell>
              <UiTableCell>
                <UiButton size="sm" :variant="summary.user.refreshEnabled ? 'outline' : 'destructive'" @click="toggleRefresh(summary)">
                  <Power />{{ summary.user.refreshEnabled ? "允许" : "已暂停" }}
                </UiButton>
              </UiTableCell>
              <UiTableCell>{{ formatDateTime(summary.stats.lastRequestAt) }}</UiTableCell>
              <UiTableCell>
                <div class="flex justify-end gap-1">
                  <UiButton variant="ghost" size="sm" @click="toggle(summary)"><Ban />{{ summary.user.enabled ? "禁用" : "启用" }}</UiButton>
                  <UiButton variant="ghost" size="sm" :disabled="summary.user.id === auth.currentUser?.id" @click="resetTarget = summary"><KeyRound />重置密码</UiButton>
                  <UiButton
                    variant="destructive"
                    size="sm"
                    :disabled="summary.user.id === auth.currentUser?.id"
                    @click="deleteTarget = summary"
                  ><Trash2 />删除</UiButton>
                </div>
              </UiTableCell>
            </UiTableRow>
          </UiTableBody>
        </UiTable>
      </div>

      <div class="grid gap-3 p-4 lg:hidden">
        <article v-for="summary in filteredUsers" :key="summary.user.id" class="rounded-xl border border-border bg-muted/30 p-4">
          <div class="flex items-start justify-between gap-3">
            <div><strong>{{ summary.user.username }}</strong><p class="text-xs text-muted-foreground">{{ summary.user.role === "admin" ? "管理员" : "普通用户" }}</p></div>
            <UiBadge :variant="summary.user.enabled ? 'secondary' : 'destructive'">{{ summary.user.enabled ? "启用" : "禁用" }}</UiBadge>
          </div>
          <dl class="mt-4 grid grid-cols-2 gap-3 text-xs">
            <div><dt class="text-muted-foreground">远端 / 缓存</dt><dd class="mt-1 font-mono">{{ summary.stats.remoteFetches }} / {{ summary.stats.cachedRefreshes }}</dd></div>
            <div><dt class="text-muted-foreground">失败 / 鉴权</dt><dd class="mt-1 font-mono">{{ summary.stats.failedRequests }} / {{ summary.stats.authFailures }}</dd></div>
            <div><dt class="text-muted-foreground">设备额度</dt><dd class="mt-1">{{ summary.deviceIds.length }} / {{ summary.user.deviceLimit }}</dd></div>
            <div><dt class="text-muted-foreground">远端刷新</dt><dd class="mt-1">{{ summary.user.refreshEnabled ? "允许" : "已暂停" }}</dd></div>
          </dl>
          <div class="mt-4 flex gap-2">
            <UiButton class="flex-1" variant="outline" @click="toggle(summary)">{{ summary.user.enabled ? "禁用" : "启用" }}</UiButton>
            <UiButton variant="outline" @click="toggleRefresh(summary)"><Power /></UiButton>
            <UiButton variant="destructive" :disabled="summary.user.id === auth.currentUser?.id" @click="deleteTarget = summary"><Trash2 />删除</UiButton>
          </div>
        </article>
      </div>
      <div v-if="filteredUsers.length === 0" class="empty-compact">没有符合条件的用户。</div>
    </section>
  </AppShell>

  <Dialog :open="Boolean(deleteTarget)" @update:open="!$event && (deleteTarget = null)">
    <DialogContent class="max-w-md">
      <DialogHeader>
        <DialogTitle>删除用户“{{ deleteTarget?.user.username }}”？</DialogTitle>
        <DialogDescription>该用户的设备、统计、加密 Cookie 和全部会话都会被删除，此操作不可撤销。</DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <UiButton variant="ghost" @click="deleteTarget = null">取消</UiButton>
        <UiButton variant="destructive" @click="remove"><Trash2 />确认删除</UiButton>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <Dialog :open="Boolean(resetTarget)" @update:open="!$event && (resetTarget = null)">
    <DialogContent class="max-w-md">
      <DialogHeader>
        <DialogTitle>重置“{{ resetTarget?.user.username }}”的密码</DialogTitle>
        <DialogDescription>保存后该用户在所有设备上的旧会话会立即失效。</DialogDescription>
      </DialogHeader>
      <label class="form-field"><span>新密码</span><UiInput v-model="resetPassword" type="password" autocomplete="new-password" /></label>
      <DialogFooter>
        <UiButton variant="ghost" @click="resetTarget = null">取消</UiButton>
        <UiButton @click="submitResetPassword">确认重置</UiButton>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
