import { expect, test } from '@playwright/test'
import { login } from './support/auth'

test('Orbit AI streams a response in its DM', async ({ page }) => {
  await login(page)

  await page.getByRole('button', { name: 'Orbit AI' }).click()
  await expect(page.getByRole('main').getByRole('heading', { name: 'Orbit AI', exact: true })).toBeVisible()

  const composer = page.getByPlaceholder('@Orbit AIにメッセージを送信')
  await composer.fill('今日の会話をまとめて')
  await composer.press('Enter')
  await expect(page.getByText('Orbit AI（デモ）', { exact: false }).first()).toBeVisible({ timeout: 10000 })
})

test('AI Work Summary organizes the current conversation', async ({ page }) => {
  await login(page)

  const summary = page.getByRole('region', { name: '会話の要点' })
  await expect(summary).toContainText('会話の要点')
  await summary.getByRole('button', { name: '会話をまとめる' }).click()
  await expect(summary).toContainText('会話', { timeout: 10000 })
  await expect(summary.getByRole('button', { name: '要点を更新' })).toBeVisible()
})

test('does not apply a stale summary after switching channels', async ({ page }) => {
  await page.route('**/api/channels/design-system/summary', async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 700))
    try {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          channel_id: 'design-system',
          generated_at: new Date().toISOString(),
          scope: 'recent',
          message_count: 1,
          unread_count: 0,
          summary: 'design-systemの古い要約',
          decisions: [],
          action_items: [],
          unresolved: [],
          chatter_count: 0,
          source_message_ids: [],
        }),
      })
    } catch {
      // The request may have been aborted when the channel changed.
    }
  })

  await login(page)

  const summary = page.getByRole('region', { name: '会話の要点' })
  await Promise.all([
    page.waitForRequest('**/api/channels/design-system/summary'),
    summary.getByRole('button', { name: '会話をまとめる' }).click(),
  ])
  await page.getByRole('button', { name: /frontend/ }).click()
  await expect(page.getByRole('main').getByRole('heading', { name: 'frontend', exact: true })).toBeVisible()
  await page.waitForTimeout(850)

  await expect(page.getByRole('region', { name: '会話の要点' })).not.toContainText('design-systemの古い要約')
})
