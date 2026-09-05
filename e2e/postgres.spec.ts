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

test('persists channel membership in PostgreSQL', async ({ page }) => {
  await login(page)

  const channelName = `postgres-channel-${Date.now()}`
  await page.getByRole('button', { name: '開発にチャンネルを追加' }).click()
  const dialog = page.getByRole('dialog', { name: 'チャンネルを作成' })
  await dialog.getByRole('checkbox', { name: /Ayaka Mori/ }).check()
  await dialog.getByLabel('チャンネル名').fill(channelName)
  await dialog.getByLabel('説明').fill('PostgreSQL E2Eで作成したチャンネル')
  await dialog.getByRole('button', { name: '作成', exact: true }).click()
  await expect(page.getByRole('main').getByRole('heading', { name: channelName, exact: true })).toBeVisible()

  await page.getByRole('button', { name: 'プロフィールを開く' }).click()
  await page.getByRole('button', { name: 'ログアウト' }).click()
  await page.getByLabel('メールアドレス').fill('ayaka@example.com')
  await page.getByLabel('パスワード').fill('demo-password')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByRole('button', { name: channelName, exact: true })).toBeVisible()
})
