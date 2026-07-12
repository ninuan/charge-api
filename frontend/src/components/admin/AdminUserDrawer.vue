<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { createDiscreteApi } from "naive-ui";
import {
  AlertTriangle,
  Ban,
  CheckCircle2,
  KeyRound,
  Power,
  Save,
  Trash2,
  UserRound
} from "@lucide/vue";
import { Badge as UiBadge } from "@/components/ui/badge";
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
import { hasAdminRisk, hasActiveAuthFailure } from "@/utils/adminRisk";
import type { AdminUserSummary, CredentialState } from "@/types/dashboard";

const props = defineProps<{
  user: AdminUserSummary | null;
  currentUserId?: string;
}>();
const emit = defineEmits<{
  close: [];
  updated: [];
  deleted: [];
}>();

const admin = useAdminStore();
const { message } = createDiscreteApi(["message"]);
const quota = ref(1);
const busy = ref(false);
const resetOpen = ref(false);
const deleteOpen = ref(false);
const password = ref("");

const credentialLabels: Record<CredentialState, string> = {
  unbound: "未绑定扫码登录",
  waiting_device: "已绑定，等待添加设备",
  healthy: "扫码凭据正常",
  sync_failed: "凭据同步失败",
  expired: "扫码凭据已失效"
};

watch(() => props.user?.user.deviceLimit, (value) => {
  quota.value = value ?? 1;
}, { immediate: true });

const isSelf = computed(() => props.user?.user.id === props.currentUserId);
const issues = computed(() => {
  if (!props.user) return [];
  const result: string[] = [];
  if (!props.user.user.enabled) result.push("账户当前已禁用");
  if (props.user.credential.state === "unbound" && props.user.deviceIds.length) result.push("尚未绑定扫码登录");
  if (props.user.credential.state === "expired") result.push("扫码凭据已经失效");
  if (props.user.credential.state === "sync_failed") result.push("登录凭据同步失败");
  if (hasActiveAuthFailure(props.user.stats)) result.push("最近存在远端鉴权失败");
  if (props.user.lastRefresh.failedDevices > 0) result.push(`${props.user.lastRefresh.failedDevices} 台设备刷新失败`);
  if (props.user.dashboard.offlinePorts > 0) result.push(`${props.user.dashboard.offlinePorts} 个充电口离线`);
  return result;
});

function formatDateTime(value?: string) {
  return value ? new Date(value).toLocaleString("zh-CN") : "暂无记录";
}

async function run(action: () => Promise<void>, success: string) {
  busy.value = true;
  try {
    await action();
    emit("updated");
    message.success(success);
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    busy.value = false;
  }
}

async function toggleEnabled() {
  if (!props.user) return;
  const enabled = !props.user.user.enabled;
  await run(() => admin.setEnabled(props.user!.user, enabled), enabled ? "用户已启用" : "用户已禁用");
}

async function saveQuota() {
  if (!props.user || quota.value < 1 || quota.value > 100) {
    message.error("设备额度需要在 1 到 100 之间");
    return;
  }
  await run(() => admin.updateUser(props.user!.user, { deviceLimit: quota.value }), "设备额度已更新");
}

async function toggleRefresh() {
  if (!props.user) return;
  const enabled = !props.user.user.refreshEnabled;
  await run(
    () => admin.updateUser(props.user!.user, { refreshEnabled: enabled }),
    enabled ? "已恢复远端刷新" : "已暂停远端刷新"
  );
}

async function resetPassword() {
  if (!props.user || password.value.length < 8) {
    message.error("新密码至少需要 8 个字符");
    return;
  }
  await run(() => admin.updateUser(props.user!.user, { password: password.value }), "密码已重置，旧会话已撤销");
  password.value = "";
  resetOpen.value = false;
}

async function deleteUser() {
  if (!props.user) return;
  busy.value = true;
  try {
    await admin.remove(props.user.user);
    deleteOpen.value = false;
    emit("deleted");
    message.success("用户已删除");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <Dialog :open="Boolean(user)" @update:open="!$event && emit('close')">
    <DialogContent
      class="admin-user-drawer left-auto right-0 top-0 h-dvh max-h-dvh w-full max-w-none translate-x-0 translate-y-0 overflow-hidden rounded-none p-0 sm:max-w-xl"
    >
      <template v-if="user">
        <DialogHeader class="admin-user-drawer-header">
          <div class="flex items-start gap-3 pr-10">
            <span class="grid size-11 shrink-0 place-items-center rounded-xl bg-secondary text-primary"><UserRound /></span>
            <div class="min-w-0">
              <DialogTitle class="truncate text-xl">{{ user.user.username }}</DialogTitle>
              <DialogDescription class="mt-1">
                {{ user.user.role === "admin" ? "管理员" : "普通用户" }} · 创建于 {{ formatDateTime(user.user.createdAt) }}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <div class="admin-user-drawer-body">
          <section class="admin-drawer-section">
            <div class="flex items-center justify-between gap-3">
              <div>
                <p class="section-kicker">账户概况</p>
                <h3 class="mt-2 font-bold">状态与扫码凭据</h3>
              </div>
              <UiBadge :variant="hasAdminRisk(user) ? 'destructive' : 'secondary'">
                {{ hasAdminRisk(user) ? "需要关注" : "运行正常" }}
              </UiBadge>
            </div>
            <dl class="admin-detail-grid">
              <div><dt>账户状态</dt><dd>{{ user.user.enabled ? "已启用" : "已禁用" }}</dd></div>
              <div><dt>扫码凭据</dt><dd>{{ credentialLabels[user.credential.state] }}</dd></div>
              <div><dt>设备</dt><dd>{{ user.deviceIds.length }} 台</dd></div>
              <div><dt>充电口</dt><dd>{{ user.dashboard.portCount }} 个</dd></div>
            </dl>
          </section>

          <section class="admin-drawer-section">
            <p class="section-kicker">使用情况</p>
            <h3 class="mt-2 font-bold">请求与更新时间</h3>
            <dl class="admin-detail-grid mt-4">
              <div><dt>最近请求</dt><dd>{{ formatDateTime(user.stats.lastRequestAt) }}</dd></div>
              <div><dt>最近远端请求</dt><dd>{{ formatDateTime(user.stats.lastRemoteFetchAt) }}</dd></div>
              <div><dt>请求总数</dt><dd>{{ user.stats.totalRequests }}</dd></div>
              <div><dt>远端 / 缓存</dt><dd>{{ user.stats.remoteFetches }} / {{ user.stats.cachedRefreshes }}</dd></div>
            </dl>
          </section>

          <section class="admin-drawer-section">
            <p class="section-kicker">当前问题</p>
            <div v-if="issues.length" class="mt-3 space-y-2">
              <p v-for="issue in issues" :key="issue" class="admin-user-issue">
                <AlertTriangle />{{ issue }}
              </p>
            </div>
            <p v-else class="mt-3 flex items-center gap-2 text-sm text-primary">
              <CheckCircle2 class="size-4" />当前没有需要处理的问题
            </p>
          </section>

          <section class="admin-drawer-section">
            <p class="section-kicker">账户配置</p>
            <div class="mt-4 grid gap-4">
              <label class="form-field">
                <span>设备额度</span>
                <div class="flex gap-2">
                  <UiInput v-model.number="quota" type="number" min="1" max="100" />
                  <UiButton variant="outline" :disabled="busy" @click="saveQuota"><Save />保存</UiButton>
                </div>
              </label>
              <div class="admin-drawer-setting">
                <div>
                  <strong>远端刷新权限</strong>
                  <p>{{ user.user.refreshEnabled ? "当前允许用户请求远端设备数据" : "当前只能查看已有缓存" }}</p>
                </div>
                <UiButton :variant="user.user.refreshEnabled ? 'outline' : 'default'" :disabled="busy" @click="toggleRefresh">
                  <Power />{{ user.user.refreshEnabled ? "暂停刷新" : "恢复刷新" }}
                </UiButton>
              </div>
            </div>
          </section>

          <section class="admin-drawer-section admin-danger-zone">
            <p class="section-kicker">敏感操作</p>
            <div class="mt-4 grid gap-2 sm:grid-cols-2">
              <UiButton variant="outline" :disabled="busy || isSelf" @click="toggleEnabled">
                <Ban />{{ user.user.enabled ? "禁用账户" : "启用账户" }}
              </UiButton>
              <UiButton variant="outline" :disabled="busy || isSelf" @click="resetOpen = true">
                <KeyRound />重置密码
              </UiButton>
              <UiButton class="sm:col-span-2" variant="destructive" :disabled="busy || isSelf" @click="deleteOpen = true">
                <Trash2 />删除用户
              </UiButton>
            </div>
            <p v-if="isSelf" class="mt-3 text-xs text-muted-foreground">不能在这里修改或删除当前登录的管理员账户。</p>
          </section>
        </div>

        <DialogFooter class="admin-user-drawer-footer">
          <UiButton class="w-full" variant="outline" @click="emit('close')">关闭详情</UiButton>
        </DialogFooter>
      </template>
    </DialogContent>
  </Dialog>

  <Dialog v-model:open="resetOpen">
    <DialogContent class="max-w-md">
      <DialogHeader>
        <DialogTitle>重置“{{ user?.user.username }}”的密码</DialogTitle>
        <DialogDescription>新密码至少 8 个字符。确认后，该用户的旧会话会立即撤销。</DialogDescription>
      </DialogHeader>
      <label class="form-field"><span>新密码</span><UiInput v-model="password" type="password" autocomplete="new-password" /></label>
      <DialogFooter>
        <UiButton variant="ghost" @click="resetOpen = false">取消</UiButton>
        <UiButton :disabled="busy || password.length < 8" @click="resetPassword">确认重置</UiButton>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <Dialog v-model:open="deleteOpen">
    <DialogContent class="max-w-md">
      <DialogHeader>
        <DialogTitle>删除用户“{{ user?.user.username }}”？</DialogTitle>
        <DialogDescription>设备、统计、加密凭据和全部会话都会被删除，此操作不可撤销。</DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <UiButton variant="ghost" @click="deleteOpen = false">取消</UiButton>
        <UiButton variant="destructive" :disabled="busy" @click="deleteUser"><Trash2 />确认删除</UiButton>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
