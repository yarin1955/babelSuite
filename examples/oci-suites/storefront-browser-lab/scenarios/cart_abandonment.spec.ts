import { test, expect } from "@playwright/test"

test("cart abandonment leaves item in cart drawer", async ({ page }) => {
  await page.goto("/")

  await page.getByRole("button", { name: /add launch headset/i }).click()
  await page.getByRole("button", { name: /open cart/i }).click()

  await expect(page.getByText("Launch Headset")).toBeVisible()
  await expect(page.getByText("Cart total")).toBeVisible()
})
