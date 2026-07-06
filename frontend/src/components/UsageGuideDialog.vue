<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { BookOpenCheck, CheckCircle2, MousePointer2, ShieldCheck } from "@lucide/vue";
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
    title: "准备微信",
    detail: "确保可以扫码登录",
    type: "list",
    items: ["准备一台可以正常使用微信的手机。", "确认微信可以扫码并完成授权。", "本系统不会要求输入微信密码。"]
  },
  {
    title: "打开扫码登录",
    detail: "在系统里生成二维码",
    type: "list",
    items: ["回到用户看板页面。", "点击右上角的“扫码登录”。", "在弹窗中点击“生成二维码”。", "等待二维码显示出来，不要关闭弹窗。"]
  },
  {
    title: "使用微信扫码",
    detail: "完成授权登录",
    type: "list",
    items: ["使用微信扫描页面里的二维码。", "按微信页面提示完成确认。", "扫码后回到系统页面，点击“检查扫码状态”。"]
  },
  {
    title: "确认绑定状态",
    detail: "让系统保存登录凭据",
    type: "list",
    items: ["扫码完成后，点击“确认绑定”。", "如果提示“扫码登录已生效”，说明当前账号已经绑定成功。", "如果当前账号已经添加过充电桩，系统会尝试自动更新登录凭据。", "如果还没有添加充电桩，也没有关系，后续添加充电桩时会自动生效。"]
  },
  {
    title: "添加充电桩",
    detail: "输入桩号或设备长 ID",
    type: "list",
    items: ["回到用户看板页面。", "点击“添加充电桩”。", "输入桩号或设备长 ID。", "点击添加后，系统会自动查询并保存该充电桩。", "一般情况下输入短桩号即可；如果短桩号无法识别，再尝试输入设备长 ID。"]
  },
  {
    title: "刷新查看状态",
    detail: "查看充电口占用情况",
    type: "final",
    items: ["添加成功后，充电桩会出现在看板中。", "点击“刷新状态”获取最新充电口占用情况。", "系统会显示每个充电口是空闲、使用中、离线还是异常。", "为避免请求过于频繁，短时间重复刷新会优先返回缓存。"]
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
          <DialogTitle class="mt-2 text-2xl font-bold tracking-[-0.03em]">扫码登录与充电桩添加说明</DialogTitle>
          <DialogDescription v-if="requiredMode" class="mt-2 leading-6">
            首次进入前请完整看完说明。完成扫码登录后，就可以添加充电桩并查看充电口状态。
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

              <ol v-if="step.type === 'list'">
                <li v-for="item in step.items" :key="item">{{ item }}</li>
              </ol>

              <div v-else class="usage-guide-final-copy">
                <div>
                  <ol>
                    <li v-for="item in step.items" :key="item">{{ item }}</li>
                  </ol>
                  <p class="usage-guide-note mt-4">如果后台提示登录凭据失效，可以再次点击“扫码登录”重新绑定。</p>
                  <p class="usage-guide-note mt-2">高级备用方式：如果扫码登录暂时不可用，仍可以在高级设置中手动更新 Cookie。</p>
                </div>
                <CheckCircle2 aria-hidden="true" />
              </div>
            </section>
          </div>
        </div>
      </div>

      <DialogFooter data-testid="usage-guide-footer" class="usage-guide-footer pb-[calc(2rem+env(safe-area-inset-bottom))]">
        <div data-testid="usage-guide-footer-inner" class="usage-guide-footer-inner">
          <div class="usage-guide-progress" aria-live="polite">
            <ShieldCheck v-if="canClose" />
            <MousePointer2 v-else />
            <span>{{ canClose ? "已读到说明底部，可以开始使用。" : "请继续向下滚动，看完整个说明。" }}</span>
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
