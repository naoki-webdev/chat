import { expect, test } from '@playwright/test'
import { login } from './support/auth'

test('quick links and conversation search open real workspace views', async ({ page }) => {
  await login(page)

  await page.getByRole('button', { name: /^受信トレイ/ }).click()
  const inbox = page.getByRole('dialog', { name: '受信トレイ' })
  await expect(inbox).toBeVisible()
  await expect(inbox).not.toContainText('未読のある会話をまとめています。')
  await page.getByRole('button', { name: '閉じる', exact: true }).click()

  const firstMessage = page.locator('.message-row').first()
  await firstMessage.hover()
  await firstMessage.locator('button[aria-label="保存"]').click()
  await page.getByRole('button', { name: /^保存済み/ }).click()
  await expect(page.getByRole('dialog', { name: '保存済み' })).toContainText('新しいカラートークンをまとめました。')
  await page.getByRole('button', { name: '閉じる', exact: true }).click()

  await page.getByRole('button', { name: /^スレッド/ }).click()
  await expect(page.getByRole('dialog', { name: 'スレッド' })).toBeVisible()
  await expect(page.getByRole('dialog', { name: 'スレッド' })).toContainText('件の返信')
  await page.getByRole('button', { name: '閉じる', exact: true }).click()

  await page.getByRole('button', { name: '会話を検索' }).click()
  await page.getByPlaceholder('チャンネルやメンバーを検索').fill('frontend')
  await page.getByRole('button', { name: /^# frontend/ }).click()
  await expect(page.getByRole('main').getByRole('heading', { name: 'frontend', exact: true })).toBeVisible()

  await page.getByRole('button', { name: 'Lumen Labs', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'ワークスペース設定' })).toContainText('Lumen Labs')
  await page.getByRole('button', { name: '閉じる', exact: true }).click()
  await page.getByRole('button', { name: 'ヘルプ' }).click()
  await expect(page.getByRole('dialog', { name: 'ヘルプ' })).toContainText('Ctrl + K')
  await page.getByRole('button', { name: '閉じる', exact: true }).click()

  await page.getByRole('button', { name: '個別メッセージを探す' }).click()
  await expect(page.getByRole('dialog', { name: '会話を検索' })).toBeVisible()
  await page.getByRole('button', { name: '閉じる', exact: true }).click()

  await page.getByRole('button', { name: 'frontend' }).click()
  await page.getByRole('button', { name: /^general(?:\s+\d+)?$/ }).click()
  await expect(page.getByRole('main').getByRole('heading', { name: 'general', exact: true })).toBeVisible()
})

test('DM details show the actual conversation participants', async ({ page }) => {
  await login(page)

  await page.getByRole('button', { name: /Orbit AI/ }).click()
  const details = page.locator('.details-panel')
  await expect(details.getByRole('heading', { name: 'Orbit AI', exact: true })).toBeVisible()
  const detailMembers = details.locator('.member-list')
  await expect(detailMembers.getByText('Orbit AI', { exact: true })).toBeVisible()
  await expect(detailMembers.getByText('Taro Tanaka', { exact: true })).toBeVisible()
  await expect(detailMembers.getByText('Ayaka Mori', { exact: true })).toHaveCount(0)
  await expect(detailMembers.getByText('Ken Ito', { exact: true })).toHaveCount(0)
})

test('can edit the profile name and presence from the user card', async ({ page }) => {
  await login(page)

  const userCard = page.locator('.user-card')
  await userCard.click()
  const profile = page.getByRole('dialog', { name: 'プロフィール' })
  await expect(profile).toBeVisible()
  await page.getByRole('button', { name: 'frontend', exact: true }).click()
  await expect(profile).toHaveCount(0)
  await userCard.click()
  await expect(profile).toBeVisible()
  await profile.getByLabel('表示名').fill('Taro Tanaka UI')
  await profile.getByRole('button', { name: '離席中', exact: true }).click()
  await profile.getByRole('button', { name: '保存', exact: true }).click()
  await expect(userCard).toContainText('Taro Tanaka UI')
  await expect(userCard).toContainText('離席中')

  await userCard.click()
  const reopenedProfile = page.getByRole('dialog', { name: 'プロフィール' })
  await reopenedProfile.getByLabel('表示名').fill('Taro Tanaka')
  await reopenedProfile.getByRole('button', { name: 'オンライン', exact: true }).click()
  await reopenedProfile.getByRole('button', { name: '保存', exact: true }).click()
  await expect(userCard).toContainText('Taro Tanaka')
  await expect(userCard).toContainText('オンライン')
})
