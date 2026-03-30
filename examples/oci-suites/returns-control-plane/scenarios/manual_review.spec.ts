import { test, expect } from "@playwright/test"

test("manual review.spec", async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText("Returns Control Plane")).toBeVisible()
})
