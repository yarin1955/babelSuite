import { test, expect } from "@playwright/test"

test("cart abandonment.spec", async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText("Storefront Browser Lab")).toBeVisible()
})
