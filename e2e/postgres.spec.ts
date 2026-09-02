import { expect, test } from '@playwright/test'
import { login } from './support/auth'

test('persists messages in PostgreSQL and restores missed events after reconnect', async ({ page, browser }) => {
  await login(page)

  const persistedBody = `postgres-persist-${Date.now()}`
  const composer = page.getByPlaceholder('#design-systemにメッセージを送信')
  await composer.fill(persistedBody)
  await composer.press('Enter')
  await expect(page.getByText(persistedBody)).toBeVisible()

  await page.reload()
  await expect(page.getByText(persistedBody)).toBeVisible()

  await page.context().setOffline(true)
  await page.waitForTimeout(500)

  const senderContext = await browser.newContext()
  const senderPage = await senderContext.newPage()
  try {
    await login(senderPage, 'ken@example.com')
    const missedBody = `postgres-replay-${Date.now()}`
    const senderComposer = senderPage.getByPlaceholder('#design-systemにメッセージを送信')
    await senderComposer.fill(missedBody)
    await senderComposer.press('Enter')
    await expect(senderPage.getByText(missedBody)).toBeVisible()

    await page.context().setOffline(false)
    await expect(page.getByText(missedBody)).toBeVisible({ timeout: 15_000 })
  } finally {
    await senderContext.close()
  }
})
