import { createRouter, createWebHashHistory } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { resolveProtectedRouteAfterAuth } from "./guards";

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: "/",
      redirect: "/login"
    },
    {
      path: "/login",
      component: () => import("@/pages/AuthPage.vue"),
      meta: { role: "guest" }
    },
    {
      path: "/register",
      component: () => import("@/pages/AuthPage.vue"),
      meta: { role: "guest" }
    },
    {
      path: "/dashboard",
      component: () => import("@/pages/UserDashboardPage.vue"),
      meta: { role: "user" }
    },
    {
      path: "/admin",
      component: () => import("@/pages/AdminDashboardPage.vue"),
      meta: { role: "admin" }
    },
    {
      path: "/:pathMatch(.*)*",
      redirect: "/"
    }
  ]
});

router.beforeEach(async (to) => {
  const auth = useAuthStore();
  const redirect = await resolveProtectedRouteAfterAuth({
    initialized: auth.initialized,
    role: auth.currentUser?.role ?? null,
    requiredRole: (to.meta.role as "admin" | "user" | "guest" | undefined) ?? "guest",
    fetchMe: auth.fetchMe
  });
  return redirect ?? true;
});

export default router;
