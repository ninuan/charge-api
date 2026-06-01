<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import {
  NAlert,
  NButton,
  NCard,
  NConfigProvider,
  NDivider,
  NForm,
  NFormItem,
  NGrid,
  NGridItem,
  NInput,
  NInputNumber,
  NLayout,
  NLayoutContent,
  NModal,
  NSpace,
  NStatistic,
  NTag,
  createDiscreteApi,
  darkTheme,
} from "naive-ui";
import PileCard from "./components/PileCard.vue";
import { useDashboardStore } from "./stores/dashboard";

const store = useDashboardStore();
const { message } = createDiscreteApi(["message"]);
const adding = ref(false);
const refreshing = ref(false);
const updatingCookie = ref(false);
const cookieModalVisible = ref(false);
const cookieText = ref("");

const form = reactive({
  id: "",
  name: "",
  number: "",
  openNum: 10,
  status: "在线",
  address: ""
});

const lastRemoteAt = computed(() => formatTime(store.refresh.lastRemoteAt));
const nextRemoteAt = computed(() => formatTime(store.refresh.nextRemoteAt));
const refreshMessageType = computed(() => (store.refresh.cached ? "warning" : "success"));

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
  } catch (error) {
    message.error((error as Error).message);
  }
}

async function refreshStatus() {
  refreshing.value = true;
  try {
    await store.refreshFromCapture();
    message.success(store.refresh.message || "状态已刷新");
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
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    updatingCookie.value = false;
  }
}

function formatTime(value?: string) {
  if (!value) return "--";
  return new Date(value).toLocaleTimeString();
}

onMounted(async () => {
  await store.fetchSnapshot();
});
</script>

<template>
  <n-config-provider :theme="darkTheme">
    <n-layout class="page-shell">
      <n-layout-content content-style="padding: 24px">
          <header class="topbar">
            <div>
              <h1>充电桩运营看板</h1>
              <p>远端接口状态监控</p>
            </div>
            <div class="topbar-actions">
              <n-button type="primary" :loading="refreshing" @click="refreshStatus">
                刷新状态
              </n-button>
              <n-button secondary @click="cookieModalVisible = true">
                更新 Cookie
              </n-button>
            </div>
          </header>

          <n-alert
            v-if="store.refresh.message"
            class="refresh-alert"
            :type="refreshMessageType"
            :show-icon="false"
          >
            {{ store.refresh.message }} · 上次远端请求 {{ lastRemoteAt }} · 下次可请求 {{ nextRemoteAt }}
          </n-alert>

          <n-grid :cols="4" :x-gap="12" :y-gap="12" class="stats-grid">
            <n-grid-item>
              <n-card>
                <n-statistic label="充电桩总数" :value="store.stats.pileCount" />
              </n-card>
            </n-grid-item>
            <n-grid-item>
              <n-card>
                <n-statistic label="充电口总数" :value="store.stats.portCount" />
              </n-card>
            </n-grid-item>
            <n-grid-item>
              <n-card>
                <n-statistic label="使用中" :value="store.stats.inUsePortCount" />
              </n-card>
            </n-grid-item>
            <n-grid-item>
              <n-card>
                <n-statistic label="最后更新时间" :value="new Date(store.snapshot.updatedAt).toLocaleTimeString()" />
              </n-card>
            </n-grid-item>
          </n-grid>

          <n-card class="create-card" title="动态新增充电桩">
            <n-form inline>
              <n-form-item label="设备长ID">
                <n-input v-model:value="form.id" placeholder="例如 2601201412385560001" />
              </n-form-item>
              <n-form-item label="名称">
                <n-input v-model:value="form.name" placeholder="如：松园3号楼北侧" />
              </n-form-item>
              <n-form-item label="桩号">
                <n-input v-model:value="form.number" placeholder="可选" />
              </n-form-item>
              <n-form-item label="口数量">
                <n-input-number v-model:value="form.openNum" :min="1" :max="20" />
              </n-form-item>
              <n-form-item label="状态">
                <n-input v-model:value="form.status" placeholder="在线/离线" />
              </n-form-item>
              <n-form-item label="地址">
                <n-input v-model:value="form.address" placeholder="可选" />
              </n-form-item>
              <n-form-item>
                <n-button type="primary" :loading="adding" @click="addPile">添加桩</n-button>
              </n-form-item>
            </n-form>
          </n-card>

          <section class="piles-wrap">
            <n-space vertical :size="14" style="width: 100%">
              <pile-card
                v-for="pile in store.piles"
                :key="pile.id"
                :pile="pile"
                @remove-pile="removePile"
              />
            </n-space>
          </section>

          <n-modal
            v-model:show="cookieModalVisible"
            preset="card"
            title="更新远端 Cookie"
            class="cookie-modal"
          >
            <n-alert type="info" :show-icon="false">
              从浏览器重新复制 `Cookie` 请求头后粘贴到这里，后端会立即用新 Cookie 试刷远端接口。
            </n-alert>
            <n-divider />
            <n-input
              v-model:value="cookieText"
              type="textarea"
              placeholder="deviceid=...; org=1; wxopenid=...; info=...; verifycode=...; sid=..."
              :autosize="{ minRows: 5, maxRows: 8 }"
            />
            <template #footer>
              <div class="modal-actions">
                <n-button @click="cookieModalVisible = false">取消</n-button>
                <n-button type="primary" :loading="updatingCookie" @click="updateCookie">
                  保存并验证
                </n-button>
              </div>
            </template>
          </n-modal>
      </n-layout-content>
    </n-layout>
  </n-config-provider>
</template>

<style scoped>
.page-shell {
  min-height: 100vh;
  background:
    linear-gradient(90deg, rgb(255 255 255 / 3%) 1px, transparent 1px),
    linear-gradient(rgb(255 255 255 / 3%) 1px, transparent 1px),
    linear-gradient(180deg, #121416, #181b1e);
  background-size: 28px 28px, 28px 28px, auto;
}

.topbar {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  align-items: center;
  margin-bottom: 14px;
}

.topbar h1 {
  margin: 0;
  font-size: 28px;
  letter-spacing: 0;
}

.topbar p {
  margin: 6px 0 0;
  color: #9fa7a1;
}

.topbar-actions,
.modal-actions {
  display: flex;
  gap: 10px;
  justify-content: flex-end;
}

.refresh-alert {
  margin-bottom: 12px;
}

.stats-grid {
  margin-bottom: 16px;
}

.create-card {
  margin-bottom: 16px;
}

.piles-wrap {
  padding-bottom: 24px;
}

.cookie-modal {
  max-width: 720px;
}

@media (max-width: 900px) {
  .topbar {
    align-items: stretch;
    flex-direction: column;
  }

  .topbar h1 {
    font-size: 24px;
  }

  .topbar-actions {
    justify-content: flex-start;
  }
}
</style>
