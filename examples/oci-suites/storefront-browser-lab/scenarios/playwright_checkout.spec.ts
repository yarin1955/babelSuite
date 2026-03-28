import { test, expect } from "@playwright/test"

test("checkout happy path", async ({ page }) => {
  await page.goto("/")

  await expect(page.getByRole("heading", { name: "Storefront Browser Lab" })).toBeVisible()
  await page.getByRole("button", { name: /add starter keyboard/i }).click()
  await page.getByRole("button", { name: /checkout/i }).click()

  await expect(page.getByText("Order accepted")).toBeVisible()
  await expect(page.getByText("ord_7001")).toBeVisible()
})
