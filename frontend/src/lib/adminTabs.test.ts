import { describe, expect, it } from "vitest";
import { adminTabQuery, resolveAdminTab } from "./adminTabs";

describe("admin tab helpers", () => {
  it("accepts known tabs", () => {
    expect(resolveAdminTab("overview")).toBe("overview");
    expect(resolveAdminTab("users")).toBe("users");
    expect(resolveAdminTab("settings")).toBe("settings");
  });

  it("falls back to overview for invalid query values", () => {
    expect(resolveAdminTab(undefined)).toBe("overview");
    expect(resolveAdminTab("unknown")).toBe("overview");
    expect(resolveAdminTab(["users"])).toBe("overview");
  });

  it("serializes a tab as a route query", () => {
    expect(adminTabQuery("users")).toEqual({ tab: "users" });
  });
});
