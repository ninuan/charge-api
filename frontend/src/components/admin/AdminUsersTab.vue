<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from "vue";
import { ChevronLeft, ChevronRight, LoaderCircle, Search, SlidersHorizontal, UserPlus, Users } from "@lucide/vue";
import { Badge as UiBadge } from "@/components/ui/badge";
import { Button as UiButton } from "@/components/ui/button";
import { Input as UiInput } from "@/components/ui/input";
import {
  Table as UiTable,
  TableBody as UiTableBody,
  TableCell as UiTableCell,
  TableHead as UiTableHead,
  TableHeader as UiTableHeader,
  TableRow as UiTableRow
} from "@/components/ui/table";
import { hasAdminRisk } from "@/utils/adminRisk";
import type { AdminUserListQuery, AdminUserPage, AdminUserSummary, CredentialState } from "@/types/dashboard";

const props = defineProps<{
  page: AdminUserPage;
  query: AdminUserListQuery;
  loading?: boolean;
  prefetching?: boolean;
  error?: string;
}>();
const emit = defineEmits<{
  "select-user": [user: AdminUserSummary];
  "create-user": [];
  "query-change": [query: Partial<AdminUserListQuery>];
  "page-change": [page: number];
}>();

const search = ref("");
const mobileFiltersOpen = ref(false);
let searchTimer: ReturnType<typeof setTimeout> | undefined;

watch(() => props.query.search, (value) => {
  search.value = value;
}, { immediate: true });

onBeforeUnmount(() => {
  if (searchTimer) clearTimeout(searchTimer);
});

const credentialLabels: Record<CredentialState, string> = {
  unbound: "未绑定",
  waiting_device: "等待设备",
  healthy: "凭据正常",
  sync_failed: "同步失败",
  expired: "凭据失效"
};

function formatDateTime(value?: string) {
  return value ? new Date(value).toLocaleString("zh-CN") : "暂无访问";
}

function healthLabel(summary: AdminUserSummary) {
  if (!summary.user.enabled) return "账户已禁用";
  if (hasAdminRisk(summary)) return "需要关注";
  return "运行正常";
}

function badgeVariant(summary: AdminUserSummary) {
  return hasAdminRisk(summary) ? "destructive" : "secondary";
}

function scheduleSearch(value: string | number) {
  const keyword = String(value);
  search.value = keyword;
  if (searchTimer) clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    emit("query-change", { search: keyword, page: 1 });
  }, 250);
}

function updateFilter(key: "account" | "credential" | "health", event: Event) {
  emit("query-change", { [key]: (event.target as HTMLSelectElement).value, page: 1 });
}
</script>

<template>
  <section class="surface-panel overflow-hidden">
    <header class="admin-users-toolbar">
      <div>
        <h2 class="text-lg font-bold">账户目录</h2>
        <p class="mt-1 text-sm leading-6 text-muted-foreground">
          共 {{ page.total }} 个账户 · 第 {{ page.page }} / {{ page.totalPages }} 页，敏感操作集中在详情中完成。
        </p>
      </div>
      <UiButton class="shrink-0" @click="emit('create-user')"><UserPlus />添加用户</UiButton>
    </header>

    <div class="admin-user-filters">
      <div class="admin-user-search-row">
        <label class="relative min-w-0 flex-1">
          <Search class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <UiInput
            :model-value="search"
            class="pl-10"
            aria-label="搜索用户名"
            placeholder="搜索用户名"
            @update:model-value="scheduleSearch"
          />
        </label>
        <UiButton
          data-mobile-filter-toggle
          class="admin-mobile-filter-toggle"
          variant="outline"
          :aria-expanded="mobileFiltersOpen"
          aria-controls="admin-user-filter-options"
          @click="mobileFiltersOpen = !mobileFiltersOpen"
        ><SlidersHorizontal />筛选</UiButton>
      </div>
      <div id="admin-user-filter-options" class="admin-user-filter-options" :class="{ 'is-open': mobileFiltersOpen }">
        <select :value="query.account" class="native-input" aria-label="筛选账户状态" @change="updateFilter('account', $event)">
          <option value="all">全部账户状态</option>
          <option value="enabled">已启用</option>
          <option value="disabled">已禁用</option>
        </select>
        <select :value="query.credential" class="native-input" aria-label="筛选凭据状态" @change="updateFilter('credential', $event)">
          <option value="all">全部凭据状态</option>
          <option value="healthy">凭据正常</option>
          <option value="unbound">未绑定</option>
          <option value="waiting_device">等待设备</option>
          <option value="sync_failed">同步失败</option>
          <option value="expired">凭据失效</option>
        </select>
        <select :value="query.health" class="native-input" aria-label="筛选健康状态" @change="updateFilter('health', $event)">
          <option value="all">全部健康状态</option>
          <option value="healthy">运行正常</option>
          <option value="risk">需要关注</option>
        </select>
      </div>
    </div>

    <p v-if="error" class="admin-user-page-error" role="alert">{{ error }}</p>

    <div v-if="page.items.length" class="hidden overflow-x-auto lg:block" :aria-busy="loading">
      <UiTable class="admin-users-table">
        <UiTableHeader>
          <UiTableRow>
            <UiTableHead>用户</UiTableHead>
            <UiTableHead>账户状态</UiTableHead>
            <UiTableHead>扫码凭据</UiTableHead>
            <UiTableHead>设备</UiTableHead>
            <UiTableHead>当前健康</UiTableHead>
            <UiTableHead>最近访问</UiTableHead>
            <UiTableHead class="text-right">操作</UiTableHead>
          </UiTableRow>
        </UiTableHeader>
        <UiTableBody>
          <UiTableRow v-for="summary in page.items" :key="summary.user.id">
            <UiTableCell>
              <strong>{{ summary.user.username }}</strong>
              <p class="text-xs text-muted-foreground">{{ summary.user.role === "admin" ? "管理员" : "普通用户" }}</p>
            </UiTableCell>
            <UiTableCell>
              <UiBadge :variant="summary.user.enabled ? 'secondary' : 'destructive'">
                {{ summary.user.enabled ? "已启用" : "已禁用" }}
              </UiBadge>
            </UiTableCell>
            <UiTableCell>{{ credentialLabels[summary.credential.state] }}</UiTableCell>
            <UiTableCell class="font-mono tabular-nums">{{ summary.deviceIds.length }} / {{ summary.user.deviceLimit }}</UiTableCell>
            <UiTableCell>
              <UiBadge :variant="badgeVariant(summary)">{{ healthLabel(summary) }}</UiBadge>
            </UiTableCell>
            <UiTableCell>{{ formatDateTime(summary.stats.lastRequestAt) }}</UiTableCell>
            <UiTableCell class="text-right">
              <UiButton variant="ghost" size="sm" :data-user-detail="summary.user.id" @click="emit('select-user', summary)">
                查看详情<ChevronRight />
              </UiButton>
            </UiTableCell>
          </UiTableRow>
        </UiTableBody>
      </UiTable>
    </div>

    <div v-if="page.items.length" class="grid gap-3 p-4 lg:hidden" :aria-busy="loading">
      <article v-for="summary in page.items" :key="summary.user.id" class="admin-user-card">
        <div class="flex items-start justify-between gap-3">
          <div>
            <strong>{{ summary.user.username }}</strong>
            <p class="mt-1 text-xs text-muted-foreground">{{ summary.user.role === "admin" ? "管理员" : "普通用户" }}</p>
          </div>
          <UiBadge :variant="badgeVariant(summary)">{{ healthLabel(summary) }}</UiBadge>
        </div>
        <dl class="admin-user-card-details">
          <div><dt>账户状态</dt><dd>{{ summary.user.enabled ? "已启用" : "已禁用" }}</dd></div>
          <div><dt>扫码凭据</dt><dd>{{ credentialLabels[summary.credential.state] }}</dd></div>
          <div><dt>设备</dt><dd>{{ summary.deviceIds.length }} / {{ summary.user.deviceLimit }}</dd></div>
          <div><dt>最近访问</dt><dd>{{ formatDateTime(summary.stats.lastRequestAt) }}</dd></div>
        </dl>
        <UiButton class="mt-4 w-full" variant="outline" :data-user-detail="summary.user.id" @click="emit('select-user', summary)">
          查看详情<ChevronRight />
        </UiButton>
      </article>
    </div>

    <div v-if="!page.items.length && !loading" class="empty-compact">
      <Users class="mx-auto mb-3 size-7 text-muted-foreground" />
      <p>没有符合当前条件的用户。</p>
    </div>
    <div v-else-if="!page.items.length" class="empty-compact text-muted-foreground">
      <LoaderCircle class="mx-auto mb-3 size-7 animate-spin" />
      <p>正在加载账户目录…</p>
    </div>

    <footer v-if="page.total > 0" class="admin-user-pagination" aria-label="用户列表分页">
      <p>第 {{ page.page }} / {{ page.totalPages }} 页 · 共 {{ page.total }} 个账户</p>
      <div class="flex items-center gap-2">
        <span v-if="prefetching" class="hidden text-xs text-muted-foreground sm:inline">正在预加载下一页</span>
        <UiButton data-user-page-prev variant="outline" size="sm" :disabled="loading || page.page <= 1" @click="emit('page-change', page.page - 1)">
          <ChevronLeft />上一页
        </UiButton>
        <UiButton data-user-page-next variant="outline" size="sm" :disabled="loading || page.page >= page.totalPages" @click="emit('page-change', page.page + 1)">
          下一页<ChevronRight />
        </UiButton>
      </div>
    </footer>
  </section>
</template>
