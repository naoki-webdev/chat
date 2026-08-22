import { expect, test } from '@playwright/test'
import { login } from './support/auth'

test('logs in and sends a realtime chat message', async ({ page }) => {
  const body = `playwright-${Date.now()}`
  await page.goto('/')

  await expect(page.getByRole('heading', { name: 'チームにログイン' })).toBeVisible()
  await page.getByLabel('メールアドレス').fill('demo@example.com')
  await page.getByLabel('パスワード').fill('demo-password')
  await page.getByRole('button', { name: 'ログイン' }).click()

  await expect(page.getByText('Lumen Labs')).toBeVisible()
  await expect(page.getByRole('button', { name: 'ワークスペースを追加' })).toHaveCount(0)
  await expect(page.locator('.connection-pill')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'チャンネルを編集' })).toBeVisible()

  const composer = page.getByPlaceholder('#design-systemにメッセージを送信')
  await composer.fill(body)
  await composer.press('Enter')
  await expect(page.getByText(body)).toBeVisible()
})

test('can switch channels and mark the selected channel as read', async ({ page }) => {
  await login(page)

  await page.getByRole('button', { name: /frontend/ }).click()
  await expect(page.getByRole('main').getByRole('heading', { name: 'frontend', exact: true })).toBeVisible()
  await expect(page.getByText('APIレスポンスの型定義、shared/typesに置いておくと使いやすそうです。')).toBeVisible()
})

test('can open a thread and add a persistent reaction', async ({ page }) => {
  await login(page)

  await expect(page.locator('.message-row').first()).toBeVisible()
  await page.waitForTimeout(250)
  const firstMessage = page.locator('.message-row').first()
  await firstMessage.hover()
  await firstMessage.locator('.message-actions button[aria-label="いいね"]').click()
  await expect(page.locator('.reaction-active').first()).toBeVisible()

  await page.getByRole('button', { name: /返信/ }).first().click()
  await expect(page.getByText('返信', { exact: true })).toBeVisible()
  const reply = page.getByPlaceholder('スレッドに返信')
  await reply.fill(`thread-${Date.now()}`)
  await reply.press('Enter')
  await expect(page.locator('.thread-replies')).toContainText('thread-')
})
