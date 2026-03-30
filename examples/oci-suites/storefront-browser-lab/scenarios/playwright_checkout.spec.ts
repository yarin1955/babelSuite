import { test, expect } from "@playwright/test"

test("playwright checkout.spec", async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText("Storefront Browser Lab")).toBeVisible()
})
