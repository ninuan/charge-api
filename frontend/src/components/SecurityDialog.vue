<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { createDiscreteApi } from "naive-ui";
import { KeyRound, Laptop, LogOut, ShieldCheck } from "@lucide/vue";
import { Button as UiButton } from "@/components/ui/button";
import { Input as UiInput } from "@/components/ui/input";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { useAuthStore } from "@/stores/auth";
import type { SessionView } from "@/types/dashboard";

const auth = useAuthStore();
const { message } = createDiscreteApi(["message"]);
const sessions = ref<SessionView[]>([]);
const saving = ref(false);
const form = reactive({ currentPassword: "", newPassword: "" });

async function loadSessions() {
  const res = await fetch("/api/auth/sessions", { credentials: "include" });
  if (res.ok) sessions.value = await res.json();
}

async function changePassword() {
  if (form.newPassword.length < 8) {
    message.error("新密码至少需要 8 个字符");
    return;
  }
  saving.value = true;
  try {
    await auth.changePassword(form.currentPassword, form.newPassword);
    Object.assign(form, { currentPassword: "", newPassword: "" });
    await loadSessions();
    message.success("密码已修改，其他设备已退出");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    saving.value = false;
  }
}

async function logoutOthers() {
  const res = await fetch("/api/auth/sessions/others", { method: "DELETE", credentials: "include" });
  if (!res.ok && res.status !== 204) {
    message.error((await res.json()).error ?? "操作失败");
    return;
  }
  await loadSessions();
  message.success("其他设备已退出");
}

onMounted(loadSessions);
</script>

<template>
  <Dialog>
    <DialogTrigger as-child>
      <UiButton variant="outline"><KeyRound />安全中心</UiButton>
    </DialogTrigger>
    <DialogContent class="max-h-[calc(100dvh-2rem)] gap-0 overflow-y-auto rounded-2xl p-0 sm:max-w-4xl">
      <DialogHeader class="security-dialog-header">
        <span class="security-dialog-icon"><ShieldCheck /></span>
        <div>
          <DialogTitle class="text-xl font-bold">账户安全</DialogTitle>
          <DialogDescription class="mt-1 leading-6">管理密码和有效登录会话。</DialogDescription>
        </div>
      </DialogHeader>
      <div class="grid gap-6 p-5 sm:p-6 md:grid-cols-[minmax(0,1.05fr)_minmax(320px,.95fr)] lg:gap-8 lg:p-8">
        <form class="space-y-4" @submit.prevent="changePassword">
          <div class="section-heading"><KeyRound /><div><h3>修改密码</h3><p>保留当前会话，退出其他设备。</p></div></div>
          <label class="form-field"><span>当前密码</span><UiInput v-model="form.currentPassword" type="password" autocomplete="current-password" /></label>
          <label class="form-field"><span>新密码</span><UiInput v-model="form.newPassword" type="password" autocomplete="new-password" /></label>
          <UiButton class="w-full" type="submit" :disabled="saving">{{ saving ? "保存中…" : "更新密码" }}</UiButton>
        </form>
        <section class="border-t border-border pt-6 md:border-l md:border-t-0 md:pl-6 md:pt-0">
          <div class="section-heading"><Laptop /><div><h3>登录会话</h3><p>查看当前账户的有效登录。</p></div></div>
          <div class="mt-4 space-y-2">
            <div v-for="session in sessions" :key="session.id" class="session-item">
              <p class="flex items-center gap-2 font-semibold"><Laptop class="size-4" />{{ session.current ? "当前会话" : "其他会话" }}</p>
              <p class="mt-1 text-xs text-muted-foreground">登录于 {{ new Date(session.createdAt).toLocaleString("zh-CN") }}</p>
            </div>
            <p v-if="!sessions.length" class="text-sm text-muted-foreground">暂无有效会话。</p>
          </div>
          <UiButton class="mt-4 w-full" variant="outline" @click="logoutOthers"><LogOut />退出其他设备</UiButton>
        </section>
      </div>
      <div class="security-dialog-footer">登录会话最长保留 7 天。请勿在公共设备上保持登录。</div>
    </DialogContent>
  </Dialog>
</template>
