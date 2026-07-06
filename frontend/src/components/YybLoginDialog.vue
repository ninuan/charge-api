<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { CheckCircle2, Link2Off, Loader2, QrCode, RefreshCw, ShieldCheck } from "@lucide/vue";
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
import CookieDialog from "@/components/CookieDialog.vue";
import { useYYBStore } from "@/stores/yyb";

const yyb = useYYBStore();
const { message } = createDiscreteApi(["message"]);
const open = ref(false);
const advanced = ref(false);

const qrImage = computed(() => yyb.qr?.imageBase64 || "");
const pollStatusMessage = computed(() => {
  switch (yyb.poll?.status) {
    case "pending":
      return "等待扫码，请使用微信扫描左侧二维码。";
    case "scanned":
      return "已扫码，请在微信中确认登录。";
    case "authorized":
    case "confirmed":
      return "扫码已确认，可以点击确认绑定。";
    case "expired":
      return "二维码已过期，请重新生成。";
    case "cancelled":
      return "扫码已取消，请重新生成二维码。";
    case "unknown":
      return "扫码状态暂时无法识别，请稍后再检查。";
    default:
      return "";
  }
});
const statusText = computed(() => {
  if (yyb.binding.bound) {
    const name = yyb.binding.nickname || "微信账号";
    return `${name} 已绑定${yyb.binding.openidSuffix ? `（尾号 ${yyb.binding.openidSuffix}）` : ""}`;
  }
  if (yyb.poll?.message) return yyb.poll.message;
  if (pollStatusMessage.value) return pollStatusMessage.value;
  if (yyb.qr) return "请使用微信扫码，扫码完成后点击确认。";
  return "生成二维码后，扫码结果会通过 Charge 后端确认并保存到当前账户。";
});
const confirmDisabled = computed(() => !yyb.qr?.sessionId || yyb.confirming);

async function loadBinding() {
  try {
    await yyb.fetchBinding();
  } catch (error) {
    message.error((error as Error).message);
  }
}

async function createQR() {
  try {
    await yyb.createQR();
    message.success("二维码已生成，扫码后请点击检查扫码状态");
  } catch (error) {
    message.error((error as Error).message);
  }
}

async function pollOnce() {
  try {
    await yyb.pollQR();
    message.success(statusText.value || "扫码状态已更新");
  } catch (error) {
    message.error((error as Error).message);
  }
}

async function confirmQR() {
  try {
    const result = await yyb.confirmQR();
    message.success(result.message || (result.cookieSynced ? "扫码登录已生效" : "扫码登录已完成"));
  } catch (error) {
    message.error((error as Error).message);
  }
}

async function clearBinding() {
  try {
    await yyb.clearBinding();
    message.success("扫码绑定已解除");
  } catch (error) {
    message.error((error as Error).message);
  }
}

watch(open, (value) => {
  if (value) {
    void loadBinding();
  }
});
</script>

<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child>
      <UiButton class="dashboard-action" variant="outline"><QrCode />扫码登录</UiButton>
    </DialogTrigger>
    <DialogContent class="sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle class="flex items-center gap-2"><ShieldCheck class="size-5 text-primary" />扫码登录远端账号</DialogTitle>
        <DialogDescription>
          微信扫码后，Charge 会通过后端服务保存当前账户的登录绑定；以后添加充电桩时会自动维护访问凭据。
        </DialogDescription>
      </DialogHeader>

      <div class="yyb-login-grid">
        <section class="yyb-qr-panel" aria-label="扫码二维码">
          <div v-if="qrImage" class="yyb-qr-frame">
            <img :src="qrImage" alt="微信扫码登录二维码">
          </div>
          <div v-else class="yyb-qr-placeholder">
            <QrCode class="size-10" />
            <p>二维码尚未生成</p>
          </div>
          <UiButton class="mt-4 w-full" :disabled="yyb.loading" @click="createQR">
            <Loader2 v-if="yyb.loading" class="animate-spin" />
            <QrCode v-else />
            {{ yyb.loading ? "生成中…" : "生成扫码二维码" }}
          </UiButton>
        </section>

        <section class="yyb-status-panel" aria-live="polite">
          <div :class="['yyb-binding-card', yyb.binding.bound ? 'yyb-binding-card--active' : '']">
            <CheckCircle2 v-if="yyb.binding.bound" class="size-5 text-primary" />
            <ShieldCheck v-else class="size-5 text-muted-foreground" />
            <div>
              <strong>{{ yyb.binding.bound ? "已绑定扫码登录" : "尚未绑定" }}</strong>
              <p>{{ statusText }}</p>
            </div>
          </div>

          <div class="grid gap-2 sm:grid-cols-2">
            <UiButton variant="outline" :disabled="!yyb.qr || yyb.polling" @click="pollOnce">
              <Loader2 v-if="yyb.polling" class="animate-spin" />
              <RefreshCw v-else />
              检查扫码状态
            </UiButton>
            <UiButton :disabled="confirmDisabled" @click="confirmQR">
              <Loader2 v-if="yyb.confirming" class="animate-spin" />
              <CheckCircle2 v-else />
              确认绑定
            </UiButton>
          </div>

          <p class="rounded-xl bg-secondary/60 p-3 text-xs leading-5 text-muted-foreground">
            如果当前账户已经添加过充电桩，确认绑定后会立即尝试同步凭据；如果还没有设备，后续通过“添加充电桩”入口添加时会自动生效。
          </p>

          <button class="advanced-toggle" type="button" @click="advanced = !advanced">
            {{ advanced ? "收起高级设置" : "高级设置：手动 Cookie 与解绑" }}
          </button>
          <div v-if="advanced" class="grid gap-2 rounded-xl border border-border bg-background/60 p-3">
            <CookieDialog trigger-label="手动更新 Cookie" trigger-class="w-full justify-start" />
            <UiButton variant="destructive" class="justify-start" :disabled="!yyb.binding.bound" @click="clearBinding">
              <Link2Off />解除扫码绑定
            </UiButton>
          </div>
        </section>
      </div>

      <DialogFooter>
        <UiButton variant="ghost" @click="open = false">关闭</UiButton>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
