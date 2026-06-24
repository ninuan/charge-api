<script setup lang="ts">
import { ref } from "vue";
import { Cookie, KeyRound } from "@lucide/vue";
import { createDiscreteApi } from "naive-ui";
import { Button as UiButton } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from "@/components/ui/dialog";
import { useDashboardStore } from "@/stores/dashboard";

const store = useDashboardStore();
const { message } = createDiscreteApi(["message"]);
const open = ref(false);
const cookie = ref("");
const saving = ref(false);

async function save() {
  if (!cookie.value.trim()) {
    message.error("请粘贴 Cookie");
    return;
  }
  saving.value = true;
  try {
    await store.updateCookie(cookie.value.trim());
    cookie.value = "";
    open.value = false;
    message.success("Cookie 已验证并加密保存");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child>
      <UiButton class="dashboard-action" variant="outline"><Cookie />更新 Cookie</UiButton>
    </DialogTrigger>
    <DialogContent class="max-w-2xl">
      <DialogHeader>
        <DialogTitle class="flex items-center gap-2"><KeyRound class="size-5 text-primary" />更新远端访问凭据</DialogTitle>
        <DialogDescription>凭据只属于当前账户，后端会加密保存。提交后会立即验证一次设备访问能力。</DialogDescription>
      </DialogHeader>
      <label class="form-field">
        <span>Cookie 内容</span>
        <textarea
          v-model="cookie"
          class="native-textarea font-mono"
          spellcheck="false"
          placeholder="deviceid=...; org=...; wxopenid=...; info=..."
        />
      </label>
      <DialogFooter>
        <UiButton variant="ghost" @click="open = false">取消</UiButton>
        <UiButton :disabled="saving" @click="save">{{ saving ? "正在验证…" : "保存并验证" }}</UiButton>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
