<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { BookOpenCheck, CheckCircle2, ExternalLink, MousePointer2, ShieldCheck } from "@lucide/vue";
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
import { useAuthStore } from "@/stores/auth";
import installReqableImage from "@/assets/usage_guide/image-20260624185059728.webp";
import qrcodeImage from "@/assets/usage_guide/image-20260624190314141.webp";
import enableProxyImage from "@/assets/usage_guide/image-20260624190718228.webp";
import capturedRequestsImage from "@/assets/usage_guide/image-20260624191018135.webp";
import requestListImage from "@/assets/usage_guide/image-20260624191018220.webp";
import copyCookieImage from "@/assets/usage_guide/image-20260624191252221.webp";

const auth = useAuthStore();
const { message } = createDiscreteApi(["message"]);
const open = ref(false);
const requiredMode = ref(false);
const hasReachedEnd = ref(false);
const saving = ref(false);
const promptedUserID = ref("");
const canClose = computed(() => !requiredMode.value || hasReachedEnd.value);
const guideSteps = [
  {
    title: "安装 Reqable",
    detail: "准备好抓包工具",
    type: "install",
    image: installReqableImage,
    imageAlt: "Reqable 官网和客户端界面截图",
    imageCaption: "选择自己设备对应的 Reqable 版本，安装后打开客户端。"
  },
  {
    title: "准备二维码和微信",
    detail: "把充电桩二维码发到微信",
    type: "list",
    image: enableProxyImage,
    imageAlt: "Reqable 开启代理和调试按钮截图",
    imageCaption: "准备好二维码后，下一步需要让 Reqable 保持代理和调试开启。",
    items: ["准备任意一张充电桩二维码照片。", "把二维码照片发送到微信文件传输助手。", "在当前设备上保持微信可用，后面需要识别二维码并登录充电页面。"]
  },
  {
    title: "打开 Reqable 抓包",
    detail: "开启代理和调试",
    type: "list",
    image: qrcodeImage,
    imageAlt: "充电桩二维码示例截图",
    imageCaption: "Reqable 开启后，在微信里识别充电桩二维码并进入充电页面。",
    items: ["打开系统代理。", "打开调试。", "保持 Reqable 运行，不要提前关闭。"]
  },
  {
    title: "在微信里打开充电页面",
    detail: "完成手机号验证码登录",
    type: "list",
    image: capturedRequestsImage,
    imageAlt: "Reqable 捕获微信充电页面请求列表截图",
    imageCaption: "微信完成登录后，Reqable 里会出现 ele.mocele.com 相关请求。",
    items: ["打开微信文件传输助手里的二维码图片。", "右键或长按识别图中二维码。", "按页面要求完成手机号和验证码登录，进入正常充电界面。"]
  },
  {
    title: "回到 Reqable 复制 Cookie",
    detail: "找到 cnum 请求并复制 Cookies",
    type: "cookie",
    image: copyCookieImage,
    imageAlt: "Reqable Cookies 复制按钮截图",
    imageCaption: "选中 cnum 请求，切到 Cookies 面板，复制完整 Cookie。"
  },
  {
    title: "粘贴到系统里",
    detail: "更新 Cookie 并验证",
    type: "paste",
    imageCaption: "回到系统点击“更新 Cookie”，粘贴刚才复制的完整 Cookie 并保存验证。"
  },
  {
    title: "收尾",
    detail: "关闭代理和调试",
    type: "final",
    image: requestListImage,
    imageAlt: "Reqable 调试请求列表截图",
    imageCaption: "Cookie 保存成功后，关闭代理和调试，避免后续网络一直走抓包代理。"
  }
];

function resetReadingState() {
  hasReachedEnd.value = false;
  void nextTick(checkScrollEnd);
}

function checkScrollEnd() {
  const scroller = document.querySelector<HTMLElement>("[data-testid='usage-guide-scroll']");
  if (!scroller || scroller.scrollHeight === 0) return;
  hasReachedEnd.value = scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 8;
}

function handleScroll(event: Event) {
  const target = event.currentTarget as HTMLElement;
  hasReachedEnd.value = target.scrollTop + target.clientHeight >= target.scrollHeight - 8;
}

function openReferenceGuide() {
  requiredMode.value = false;
  hasReachedEnd.value = true;
  open.value = true;
}

function handleOpenChange(nextOpen: boolean) {
  if (!nextOpen && requiredMode.value) return;
  open.value = nextOpen;
}

async function closeGuide() {
  if (requiredMode.value && !hasReachedEnd.value) return;
  if (!requiredMode.value) {
    open.value = false;
    return;
  }
  saving.value = true;
  try {
    await auth.acknowledgeUsageGuide();
    requiredMode.value = false;
    open.value = false;
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    saving.value = false;
  }
}

watch(
  () => auth.currentUser,
  (user) => {
    if (!user || user.role !== "user" || user.usageGuideAckAt || promptedUserID.value === user.id) return;
    promptedUserID.value = user.id;
    requiredMode.value = true;
    open.value = true;
    resetReadingState();
  },
  { immediate: true }
);
</script>

<template>
  <Dialog :open="open" @update:open="handleOpenChange">
    <DialogTrigger as-child>
      <UiButton data-testid="usage-guide-trigger" class="dashboard-action" variant="outline" @click="openReferenceGuide">
        <BookOpenCheck />
        使用说明
      </UiButton>
    </DialogTrigger>

    <DialogContent
      v-if="open"
      data-testid="usage-guide-dialog"
      :show-close-button="false"
      class="usage-guide-dialog usage-guide-dialog--wide h-[min(820px,calc(100dvh-4.5rem))] max-w-[calc(100vw-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden rounded-3xl p-0 sm:max-w-[min(1180px,calc(100vw-2rem))]"
    >
      <DialogHeader class="usage-guide-header">
        <span class="usage-guide-icon" aria-hidden="true"><BookOpenCheck /></span>
        <div class="min-w-0 flex-1">
          <p class="section-kicker">First run guide</p>
          <DialogTitle class="mt-2 text-2xl font-bold tracking-[-0.03em]">充电桩 Cookie 获取使用说明</DialogTitle>
          <DialogDescription v-if="requiredMode" class="mt-2 leading-6">
            首次进入前请完整看完说明，滚动到底部后才能关闭。
          </DialogDescription>
        </div>
      </DialogHeader>

      <div data-testid="usage-guide-scroll" class="usage-guide-scroll min-h-0" @scroll="handleScroll">
        <div data-testid="usage-guide-layout" class="usage-guide-layout">
          <aside data-testid="usage-guide-sidebar" class="usage-guide-sidebar" aria-label="使用说明步骤">
            <p class="usage-guide-sidebar-title">操作路径</p>
            <a
              v-for="(step, index) in guideSteps"
              :key="step.title"
              data-testid="usage-guide-step-link"
              class="usage-guide-step-link"
              :href="`#usage-guide-step-${index + 1}`"
            >
              <span>{{ index + 1 }}</span>
              <strong>{{ step.title }}</strong>
              <small>{{ step.detail }}</small>
            </a>
          </aside>

          <div class="usage-guide-main">
            <section
              v-for="(step, index) in guideSteps"
              :id="`usage-guide-step-${index + 1}`"
              :key="step.title"
              class="usage-guide-card"
              :class="{ 'usage-guide-card--final': step.type === 'final' }"
            >
              <div class="usage-guide-card-heading">
                <span class="usage-guide-step-index">{{ index + 1 }}</span>
                <div>
                  <h3>{{ step.title }}</h3>
                  <p>{{ step.detail }}</p>
                </div>
              </div>

              <template v-if="step.type === 'install'">
                <p>
                  先安装抓包工具 Reqable：
                  <a href="https://reqable.com/zh-CN/" target="_blank" rel="noreferrer">打开官网 <ExternalLink class="inline size-3.5" /></a>
                </p>
                <p class="usage-guide-note">Windows、Mac、安卓和 iPhone 都可以安装，选择自己设备对应的版本即可。</p>
              </template>

              <ol v-else-if="step.type === 'list'">
                <li v-for="item in step.items" :key="item">{{ item }}</li>
              </ol>

              <ol v-else-if="step.type === 'cookie'">
                <li>找到类似 <code>URL=http://ele.mocele.com/i/cnum?n=61034278</code> 的请求。</li>
                <li>双击打开该请求，切到 Cookies 选项。</li>
                <li>复制 Cookies 里的完整值，格式通常类似 <code>deviceid=...; org=...; wxopenid=...; info=...</code>。</li>
              </ol>

              <p v-else-if="step.type === 'paste'">回到本系统，点击“更新 Cookie”，把刚才复制到的完整 Cookie 粘贴进去并保存验证。</p>

              <div v-else class="usage-guide-final-copy">
                <p>保存成功后，关闭系统代理，停止调试，再关闭 Reqable。之后添加充电桩并点击刷新即可查看状态。</p>
                <CheckCircle2 aria-hidden="true" />
              </div>

              <figure v-if="step.image" class="usage-guide-figure">
                <img
                  data-testid="usage-guide-image"
                  :src="step.image"
                  :alt="step.imageAlt"
                  loading="lazy"
                  decoding="async"
                >
                <figcaption>{{ step.imageCaption }}</figcaption>
              </figure>
            </section>
          </div>
        </div>
      </div>

      <DialogFooter data-testid="usage-guide-footer" class="usage-guide-footer pb-[calc(2rem+env(safe-area-inset-bottom))]">
        <div data-testid="usage-guide-footer-inner" class="usage-guide-footer-inner">
          <div class="usage-guide-progress" aria-live="polite">
            <ShieldCheck v-if="canClose" />
            <MousePointer2 v-else />
            <span>{{ canClose ? "已读到说明底部，可以关闭。" : "请继续向下滚动，看完整个说明。" }}</span>
          </div>
          <UiButton
            data-testid="usage-guide-primary"
            :disabled="!canClose || saving"
            @click="closeGuide"
          >
            <CheckCircle2 />
            {{ saving ? "正在确认…" : requiredMode ? "我已看完并关闭" : "关闭" }}
          </UiButton>
        </div>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
