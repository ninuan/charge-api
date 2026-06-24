import { describe, expect, it } from "vitest";
import { resolveHomeRoute, resolveProtectedRoute, resolveProtectedRouteAfterAuth } from "./guards";

describe("route guards", () => {
  it("sends anonymous users to login", () => {
    expect(resolveProtectedRoute(null, "user")).toBe("/login");
  });

  it("keeps each role inside its own workspace", () => {
    expect(resolveProtectedRoute("admin", "user")).toBe("/admin");
    expect(resolveProtectedRoute("user", "admin")).toBe("/dashboard");
  });

  it("chooses the correct home route", () => {
    expect(resolveHomeRoute("admin")).toBe("/admin");
    expect(resolveHomeRoute("user")).toBe("/dashboard");
    expect(resolveHomeRoute(null)).toBe("/login");
  });

  it("falls back to the login route when auth initialization fails", async () => {
    const redirect = await resolveProtectedRouteAfterAuth({
      initialized: false,
      role: null,
      requiredRole: "user",
      fetchMe: async () => {
        throw new Error("network failed");
      }
    });

    expect(redirect).toBe("/login");
  });
});
