<script setup lang="ts">
import { reactive, ref } from "vue";
import { Plus, ServerCog } from "@lucide/vue";
import { createDiscreteApi } from "naive-ui";
import { Button as UiButton } from "@/components/ui/button";
import { Input as UiInput } from "@/components/ui/input";
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
import { useAuthStore } from "@/stores/auth";

const store = useDashboardStore();
const auth = useAuthStore();
const { message } = createDiscreteApi(["message"]);
const open = ref(false);
const advanced = ref(false);
const submitting = ref(false);
const form = reactive({
  id: "",
  name: "",
  number: "",
  openNum: 10,
  status: "在线",
  address: ""
});

async function submit() {
  if (!/^[0-9]{6,64}$/.test(form.id.trim())) {
    message.error("设备长 ID 需要为 6–64 位数字");
    return;
  }
  submitting.value = true;
  try {
    await store.addPile({
      id: form.id.trim(),
      name: form.name.trim() || `充电桩 ${form.id.trim().slice(-6)}`,
      number: form.number.trim(),
      openNum: Number(form.openNum),
      status: form.status.trim() || "在线",
      address: form.address.trim()
    });
    Object.assign(form, { id: "", name: "", number: "", openNum: 10, status: "在线", address: "" });
    open.value = false;
    message.success("充电桩已添加");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child>
      <UiButton><Plus />添加充电桩</UiButton>
    </DialogTrigger>
    <DialogContent class="max-w-xl">
      <DialogHeader>
        <DialogTitle class="flex items-center gap-2"><ServerCog class="size-5 text-primary" />添加充电桩</DialogTitle>
        <DialogDescription>当前账户最多添加 {{ auth.currentUser?.deviceLimit ?? 10 }} 台设备。添加时会验证一次远端数据，成功后不会重复请求。</DialogDescription>
      </DialogHeader>
      <form class="space-y-5" @submit.prevent="submit">
        <label class="form-field">
          <span>设备长 ID <b>*</b></span>
          <UiInput v-model="form.id" inputmode="numeric" autocomplete="off" placeholder="例如 2601201412385560001" />
          <small>从充电桩页面或请求参数中获得的纯数字设备 ID。</small>
        </label>
        <label class="form-field">
          <span>显示名称</span>
          <UiInput v-model="form.name" placeholder="例如：松园 3 号楼北侧" />
        </label>
        <button class="advanced-toggle" type="button" @click="advanced = !advanced">
          {{ advanced ? "收起高级字段" : "填写桩号、地址等高级字段" }}
        </button>
        <div v-if="advanced" class="grid gap-4 sm:grid-cols-2">
          <label class="form-field"><span>桩号</span><UiInput v-model="form.number" placeholder="可选" /></label>
          <label class="form-field">
            <span>充电口数量</span>
            <input v-model.number="form.openNum" class="native-input" min="1" max="20" type="number">
          </label>
          <label class="form-field"><span>设备状态</span><UiInput v-model="form.status" placeholder="在线" /></label>
          <label class="form-field"><span>安装地址</span><UiInput v-model="form.address" placeholder="可选" /></label>
        </div>
        <DialogFooter>
          <UiButton type="button" variant="ghost" @click="open = false">取消</UiButton>
          <UiButton type="submit" :disabled="submitting">{{ submitting ? "添加中…" : "确认添加" }}</UiButton>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>
