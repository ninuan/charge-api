<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { createDiscreteApi } from "naive-ui";
import { Check, Copy, KeyRound, RefreshCw, Save, Settings2, Trash2 } from "@lucide/vue";
import { Button as UiButton } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import { useAdminStore } from "@/stores/admin";
import type { InviteCode, RegistrationSettings } from "@/types/dashboard";

const props = defineProps<{
  settings: RegistrationSettings | null;
  invites: InviteCode[];
}>();

const admin = useAdminStore();
const { message } = createDiscreteApi(["message"]);
const dirty = ref(false);
const initialized = ref(false);
const saving = ref(false);
const creatingInvite = ref(false);
const deleteTarget = ref<InviteCode | null>(null);
const deletingInvite = ref(false);
const draft = reactive<RegistrationSettings>({
  openRegistration: true,
  inviteRequired: true,
  defaultDeviceLimit: 10,
  defaultRefreshEnabled: true,
  statsRetentionDays: 90
});

const sortedInvites = computed(() => [...props.invites].sort((left, right) =>
  new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime()
));

watch(() => props.settings, (value) => {
  if (!value || (initialized.value && dirty.value)) return;
  Object.assign(draft, value);
  initialized.value = true;
}, { immediate: true, deep: true });

function markDirty() {
  dirty.value = true;
}

function isDirty() {
  return dirty.value;
}

async function saveSettings() {
  saving.value = true;
  try {
    await admin.saveSettings({ ...draft });
    dirty.value = false;
    message.success("系统策略已保存");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    saving.value = false;
  }
}

async function createInvite() {
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

async function copyInvite(invite: InviteCode) {
  try {
    await navigator.clipboard.writeText(invite.code);
    message.success("邀请码已复制");
  } catch {
    message.error("复制失败，请手动选择邀请码");
  }
}

async function removeInvite() {
  if (!deleteTarget.value) return;
  deletingInvite.value = true;
  try {
    await admin.removeInvite(deleteTarget.value.id);
    deleteTarget.value = null;
    message.success("邀请码已删除");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    deletingInvite.value = false;
  }
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString("zh-CN");
}

defineExpose({ isDirty });
</script>

<template>
  <section class="admin-settings-grid">
    <article class="surface-panel overflow-hidden">
      <div class="p-5 sm:p-6">
        <p class="section-kicker">访问策略</p>
        <h2 class="mt-2 flex items-center gap-2 text-xl font-bold"><Settings2 class="size-5" />注册与资源策略</h2>
        <p class="mt-2 text-sm leading-6 text-muted-foreground">控制注册入口，以及新账户默认拥有的设备额度和刷新权限。</p>

        <div class="mt-6 space-y-6">
          <fieldset>
            <legend class="setting-group-label">注册入口</legend>
            <div class="mt-2 grid items-start gap-3 sm:grid-cols-2">
              <label class="setting-option">
                <span><strong>允许公开注册</strong><small>无需邀请码即可创建账户</small></span>
                <input
                  v-model="draft.openRegistration"
                  class="setting-checkbox"
                  type="checkbox"
                  aria-label="允许公开注册"
                  @change="markDirty"
                >
              </label>
              <label class="setting-option">
                <span><strong>允许邀请码注册</strong><small>关闭公开注册后仍可凭邀请码加入</small></span>
                <input
                  v-model="draft.inviteRequired"
                  class="setting-checkbox"
                  type="checkbox"
                  aria-label="允许邀请码注册"
                  @change="markDirty"
                >
              </label>
            </div>
          </fieldset>

          <fieldset>
            <legend class="setting-group-label">新账户默认权限</legend>
            <label class="setting-option setting-option--compact mt-2">
              <span><strong>允许远端刷新</strong><small>仅影响之后创建的新账户，已有账户可单独调整</small></span>
              <input
                v-model="draft.defaultRefreshEnabled"
                class="setting-checkbox"
                type="checkbox"
                aria-label="默认允许远端刷新"
                @change="markDirty"
              >
            </label>
          </fieldset>

          <fieldset>
            <legend class="setting-group-label">资源与数据</legend>
            <div class="mt-2 grid items-start gap-3 sm:grid-cols-2">
              <label class="form-field setting-field">
                <span>新用户默认设备额度</span>
                <input
                  v-model.number="draft.defaultDeviceLimit"
                  class="native-input"
                  min="1"
                  max="100"
                  type="number"
                  aria-label="新用户默认设备额度"
                  @input="markDirty"
                >
                <small>可在用户详情中单独覆盖。</small>
              </label>
              <label class="form-field setting-field">
                <span>统计数据保留时间</span>
                <input
                  v-model.number="draft.statsRetentionDays"
                  class="native-input"
                  min="1"
                  max="365"
                  type="number"
                  aria-label="统计数据保留时间"
                  @input="markDirty"
                >
                <small>单位：天，保存后自动清理更早数据。</small>
              </label>
            </div>
          </fieldset>
        </div>
      </div>

      <div class="panel-action-bar">
        <p>
          <Check aria-hidden="true" />
          {{ dirty ? "存在尚未保存的修改" : "当前策略已保存" }}
        </p>
        <UiButton data-save-settings :disabled="saving || !dirty" @click="saveSettings">
          <Save />{{ saving ? "保存中…" : "保存策略" }}
        </UiButton>
      </div>
    </article>

    <article class="surface-panel p-5 sm:p-6">
      <div class="flex flex-col justify-between gap-4 sm:flex-row sm:items-start">
        <div>
          <p class="section-kicker">邀请管理</p>
          <h2 class="mt-2 flex items-center gap-2 text-xl font-bold"><KeyRound class="size-5" />长期邀请码</h2>
          <p class="mt-2 text-sm leading-6 text-muted-foreground">邀请码由服务端安全随机生成，长期有效，可按需创建和撤销。</p>
        </div>
        <UiButton data-create-invite class="shrink-0" :disabled="creatingInvite" @click="createInvite">
          <RefreshCw :class="{ 'animate-spin': creatingInvite }" />
          {{ creatingInvite ? "生成中…" : "生成随机邀请码" }}
        </UiButton>
      </div>

      <div class="admin-invite-list">
        <article v-for="invite in sortedInvites" :key="invite.id" class="admin-invite-card">
          <div class="min-w-0">
            <p class="break-all font-mono text-sm font-bold">{{ invite.code }}</p>
            <p class="mt-2 text-xs leading-5 text-muted-foreground">
              创建于 {{ formatDateTime(invite.createdAt) }} · 已使用 {{ invite.usedCount }} 次
            </p>
          </div>
          <div class="flex shrink-0 gap-1">
            <UiButton
              variant="ghost"
              size="icon-sm"
              :aria-label="`复制邀请码 ${invite.code}`"
              :title="`复制邀请码 ${invite.code}`"
              @click="copyInvite(invite)"
            ><Copy /></UiButton>
            <UiButton
              variant="ghost"
              size="icon-sm"
              :aria-label="`删除邀请码 ${invite.code}`"
              :title="`删除邀请码 ${invite.code}`"
              @click="deleteTarget = invite"
            ><Trash2 /></UiButton>
          </div>
        </article>
        <div v-if="!sortedInvites.length" class="empty-compact rounded-xl border border-dashed border-border">
          尚未生成邀请码。
        </div>
      </div>
    </article>
  </section>

  <Dialog :open="Boolean(deleteTarget)" @update:open="!$event && (deleteTarget = null)">
    <DialogContent class="max-w-md">
      <DialogHeader>
        <DialogTitle>删除邀请码？</DialogTitle>
        <DialogDescription>
          删除后，“{{ deleteTarget?.code }}”将无法继续用于注册，已经创建的账户不受影响。
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <UiButton variant="ghost" @click="deleteTarget = null">取消</UiButton>
        <UiButton variant="destructive" :disabled="deletingInvite" @click="removeInvite">
          <Trash2 />{{ deletingInvite ? "删除中…" : "确认删除" }}
        </UiButton>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
