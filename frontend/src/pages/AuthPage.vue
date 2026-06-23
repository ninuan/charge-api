<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { createDiscreteApi } from "naive-ui";
import { Activity, ArrowRight, CheckCircle2, Eye, EyeOff, Gauge, LockKeyhole, RefreshCw, ShieldCheck, UserRound } from "@lucide/vue";
import { Button as UiButton } from "@/components/ui/button";
import { Input as UiInput } from "@/components/ui/input";
import TurnstileWidget from "@/components/TurnstileWidget.vue";
import { useAuthStore } from "@/stores/auth";
import { resolveHomeRoute } from "@/router/guards";

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const { message } = createDiscreteApi(["message"]);
const mode = computed(() => route.path === "/register" ? "register" : "login");
const submitting = ref(false);
const showPassword = ref(false);
const authConfigReady = ref(false);
const turnstileEnabled = ref(false);
const turnstileSiteKey = ref("");
const captchaToken = ref("");
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null);
const registerCaptchaEnabled = ref(true);
const registrationOpen = ref(true);
const inviteRequired = ref(true);
const authConfigVersion = ref(0);
const captchaID = ref("");
const captchaImage = ref("");
const captchaAnswer = ref("");
const captchaLoading = ref(false);
const form = reactive({ username: "", password: "", inviteCode: "" });
const registrationAvailable = computed(() => registrationOpen.value || inviteRequired.value);
const registerDescription = computed(() => {
  if (inviteRequired.value && !registrationOpen.value) return "输入管理员提供的邀请码，创建你的独立账户。";
  if (inviteRequired.value && registrationOpen.value) return "可以直接注册，也可以填写管理员提供的邀请码。";
  if (registrationOpen.value) return "创建账户后，即可配置个人 Cookie 和充电桩。";
  return "当前未开放自助注册，请联系管理员开通账户。";
});

async function loadConfig() {
  try {
    const res = await fetch("/api/auth/config", { cache: "no-store" });
    if (!res.ok) throw new Error("安全配置加载失败");
    const config = await res.json() as {
      turnstileEnabled: boolean;
      turnstileSiteKey: string;
      authConfigVersion?: number;
      registerCaptchaEnabled?: boolean;
      registrationOpen?: boolean;
      inviteRequired?: boolean;
    };
    turnstileEnabled.value = config.turnstileEnabled;
    authConfigVersion.value = config.authConfigVersion ?? 0;
    turnstileSiteKey.value = config.turnstileSiteKey;
    registerCaptchaEnabled.value = config.registerCaptchaEnabled ?? true;
    registrationOpen.value = config.registrationOpen ?? true;
    inviteRequired.value = config.inviteRequired ?? false;
    authConfigReady.value = true;
    if (mode.value === "register") await loadCaptcha();
  } catch (error) {
    message.error((error as Error).message);
  }
}

async function loadCaptcha() {
  if (!registerCaptchaEnabled.value) return;
  captchaLoading.value = true;
  captchaAnswer.value = "";
  try {
    const res = await fetch("/api/auth/register-captcha", { credentials: "include", cache: "no-store" });
    if (!res.ok) throw new Error((await res.json()).error ?? "验证码加载失败");
    const challenge = await res.json() as { id: string; image: string };
    captchaID.value = challenge.id;
    captchaImage.value = challenge.image;
  } catch (error) {
    captchaID.value = "";
    captchaImage.value = "";
    message.error((error as Error).message);
  } finally {
    captchaLoading.value = false;
  }
}

function resetTurnstile() {
  captchaToken.value = "";
  turnstileRef.value?.reset();
}

async function submit() {
  const username = form.username.trim();
  if (username.length < 3 || !form.password) {
    message.error("请输入至少 3 位用户名和密码");
    return;
  }
  if (mode.value === "register" && form.password.length < 8) {
    message.error("注册密码至少需要 8 个字符");
    return;
  }
  if (mode.value === "register" && !registrationOpen.value && inviteRequired.value && !form.inviteCode.trim()) {
    message.error("请输入邀请码");
    return;
  }
  if (mode.value === "register" && !registrationOpen.value && inviteRequired.value && authConfigVersion.value < 2) {
    message.error("后端服务仍是旧版本，请重启后端服务后再使用邀请码注册");
    return;
  }
  if (turnstileEnabled.value && !captchaToken.value) {
    message.error("请先完成人机验证");
    return;
  }
  if (mode.value === "register" && registerCaptchaEnabled.value && !captchaAnswer.value.trim()) {
    message.error("请输入图片验证码");
    return;
  }

  submitting.value = true;
  try {
    if (mode.value === "login") {
      await auth.login(username, form.password, captchaToken.value);
    } else {
      await auth.register(username, form.password, captchaToken.value, captchaID.value, captchaAnswer.value.trim(), form.inviteCode.trim());
    }
    form.password = "";
    await router.replace(resolveHomeRoute(auth.currentUser?.role ?? null));
    message.success(mode.value === "login" ? "登录成功" : "注册成功");
  } catch (error) {
    const errorMessage = (error as Error).message;
    if (
      mode.value === "register" &&
      form.inviteCode.trim() &&
      errorMessage.includes("未开放注册")
    ) {
      message.error("邀请码已填写，但后端仍在运行旧版本。请重启后端服务后重试");
    } else {
      message.error(errorMessage);
    }
    if (mode.value === "register") await loadCaptcha();
  } finally {
    submitting.value = false;
    resetTurnstile();
  }
}

watch(mode, async (nextMode) => {
  resetTurnstile();
  captchaAnswer.value = "";
  if (nextMode === "register" && authConfigReady.value) await loadCaptcha();
});

onMounted(loadConfig);
</script>

<template>
  <main class="auth-page">
    <div class="auth-noise" aria-hidden="true" />
    <section class="auth-story">
      <div class="relative z-10">
        <div class="flex items-center gap-3">
          <div class="brand-mark brand-mark--large"><span /></div>
          <div>
            <p class="text-lg font-bold">Charge Console</p>
            <p class="text-xs text-white/55">充电设施运营中心</p>
          </div>
        </div>
        <div class="mt-16 max-w-xl lg:mt-24">
          <p class="section-kicker text-emerald-300">Infrastructure, clearly seen</p>
          <h1 class="mt-4 text-4xl font-bold leading-[1.06] tracking-[-0.055em] text-white sm:text-5xl lg:text-6xl">
            十个端口，<br>每个状态都清楚。
          </h1>
          <p class="mt-6 max-w-lg text-base leading-7 text-white/62">
            按需刷新远端数据，独立管理个人凭据，在一个安静、可靠的工作台里掌握设备占用和剩余时间。
          </p>
        </div>
      </div>
      <div class="relative z-10 grid gap-3 sm:grid-cols-3">
        <div class="auth-proof"><Activity /><strong>主动刷新</strong><span>避免无意义请求</span></div>
        <div class="auth-proof"><ShieldCheck /><strong>凭据隔离</strong><span>每个用户独立</span></div>
        <div class="auth-proof"><Gauge /><strong>退避保护</strong><span>请求节奏可控</span></div>
      </div>
    </section>

    <section class="auth-form-side">
      <div class="w-full max-w-md">
        <div class="mb-8 lg:hidden">
          <div class="flex items-center gap-3">
            <div class="brand-mark"><span /></div>
            <strong>Charge Console</strong>
          </div>
        </div>
        <p class="section-kicker">{{ mode === "login" ? "Welcome back" : "Create account" }}</p>
        <h2 class="mt-2 text-3xl font-bold tracking-[-0.04em]">
          {{ mode === "login" ? "登录运营工作台" : "注册普通用户" }}
        </h2>
        <p class="mt-3 text-sm leading-6 text-muted-foreground">
          {{ mode === "login" ? "管理员进入监控中心，普通用户进入个人设备看板。" : registerDescription }}
        </p>

        <div class="mt-7 grid grid-cols-2 rounded-xl bg-muted p-1" aria-label="登录注册切换">
          <RouterLink class="auth-tab" :class="{ active: mode === 'login' }" to="/login">登录</RouterLink>
          <RouterLink class="auth-tab" :class="{ active: mode === 'register' }" to="/register">注册</RouterLink>
        </div>

        <form class="mt-7 space-y-5" @submit.prevent="submit">
          <div v-if="mode === 'register' && !registrationAvailable" class="auth-notice" role="status">
            <ShieldCheck aria-hidden="true" />
            <div>
              <strong>自助注册暂未开放</strong>
              <p>请联系管理员为你创建账户。</p>
            </div>
          </div>
          <label class="form-field">
            <span><UserRound />用户名</span>
            <UiInput v-model="form.username" autocomplete="username" placeholder="请输入用户名" />
          </label>

          <label v-if="mode === 'register' && inviteRequired" class="form-field">
            <span>
              <ShieldCheck />
              邀请码
              <b v-if="!registrationOpen">*</b>
              <small v-else class="ml-auto">选填</small>
            </span>
            <UiInput v-model="form.inviteCode" autocomplete="off" placeholder="请输入管理员提供的邀请码" />
            <small v-if="!registrationOpen">当前仅开放邀请注册。</small>
          </label>
          <label class="form-field">
            <span><LockKeyhole />密码</span>
            <div class="relative">
              <UiInput
                v-model="form.password"
                :type="showPassword ? 'text' : 'password'"
                :autocomplete="mode === 'login' ? 'current-password' : 'new-password'"
                placeholder="请输入密码"
                class="pr-12"
              />
              <button
                class="password-toggle"
                type="button"
                :aria-label="showPassword ? '隐藏密码' : '显示密码'"
                @click="showPassword = !showPassword"
              >
                <EyeOff v-if="showPassword" />
                <Eye v-else />
              </button>
            </div>
            <small v-if="mode === 'register'">至少 8 个字符。</small>
          </label>

          <label v-if="mode === 'register' && registerCaptchaEnabled" class="form-field">
            <span><CheckCircle2 />图片验证码</span>
            <div class="grid grid-cols-[minmax(0,1fr)_148px] gap-3">
              <UiInput v-model="captchaAnswer" autocomplete="off" placeholder="输入验证码" />
              <button class="captcha-button" type="button" :disabled="captchaLoading" @click="loadCaptcha">
                <img v-if="captchaImage" :src="captchaImage" alt="注册验证码，点击可刷新">
                <span v-else><RefreshCw :class="{ 'animate-spin': captchaLoading }" />刷新</span>
              </button>
            </div>
          </label>

          <TurnstileWidget
            v-if="turnstileEnabled && turnstileSiteKey"
            ref="turnstileRef"
            :site-key="turnstileSiteKey"
            :action="mode"
            @verified="captchaToken = $event"
            @expired="captchaToken = ''"
            @error="captchaToken = ''"
          />

          <UiButton
            class="w-full"
            size="lg"
            type="submit"
            :disabled="!authConfigReady || submitting || captchaLoading || (mode === 'register' && !registrationAvailable) || (turnstileEnabled && !captchaToken)"
          >
            {{ submitting ? "正在处理…" : mode === "login" ? "登录" : "注册并进入" }}
            <ArrowRight />
          </UiButton>
        </form>
        <p class="mt-7 text-center text-xs leading-5 text-muted-foreground">
          登录状态最长保留 7 天。公共设备上使用后请及时退出。
        </p>
      </div>
    </section>
  </main>
</template>
