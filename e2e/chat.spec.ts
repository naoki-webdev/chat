import { expect, test } from '@playwright/test'

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
  await page.goto('/')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByText('Lumen Labs')).toBeVisible()

  await page.getByRole('button', { name: /frontend/ }).click()
  await expect(page.getByRole('main').getByRole('heading', { name: 'frontend', exact: true })).toBeVisible()
  await expect(page.getByText('APIレスポンスの型定義、shared/typesに置いておくと使いやすそうです。')).toBeVisible()
})

test('shows channel members and edits channel settings from the header', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByText('Lumen Labs')).toBeVisible()

  const details = page.locator('.details-panel')
  await expect(details).toContainText('Mio Tanaka')
  await expect(details).toContainText('Orbit AI')

  await page.getByRole('button', { name: 'メンバーを表示' }).click()
  await expect(details).toHaveCount(0)
  await page.getByRole('button', { name: 'メンバーを表示' }).click()
  await expect(details).toBeVisible()

  await page.getByRole('button', { name: 'チャンネルを編集' }).click()
  const dialog = page.getByRole('dialog', { name: 'チャンネルを編集' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByLabel('チャンネル名')).toHaveValue('design-system')
  const kenMember = dialog.getByRole('checkbox', { name: /Ken Ito/ })
  await expect(kenMember).toBeChecked()
  await kenMember.uncheck()
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await expect(details).not.toContainText('Ken Ito')

  await page.getByRole('button', { name: 'チャンネルを編集' }).click()
  const reopenedDialog = page.getByRole('dialog', { name: 'チャンネルを編集' })
  const restoredKenMember = reopenedDialog.getByRole('checkbox', { name: /Ken Ito/ })
  await restoredKenMember.check()
  await reopenedDialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(reopenedDialog).toHaveCount(0)
  await expect(details).toContainText('Ken Ito')
})

test('Orbit AI streams a response in its DM', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByText('Lumen Labs')).toBeVisible()

  await page.getByRole('button', { name: 'Orbit AI' }).click()
  await expect(page.getByRole('main').getByRole('heading', { name: 'Orbit AI', exact: true })).toBeVisible()

  const composer = page.getByPlaceholder('@Orbit AIにメッセージを送信')
  await composer.fill('今日の会話をまとめて')
  await composer.press('Enter')
  await expect(page.getByText('Orbit AI（デモ）', { exact: false }).first()).toBeVisible({ timeout: 10000 })
})

test('AI Work Summary organizes the current conversation', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByText('Lumen Labs')).toBeVisible()

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

  await page.goto('/')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByText('Lumen Labs')).toBeVisible()

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

test('can open a thread and add a persistent reaction', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByText('Lumen Labs')).toBeVisible()

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

test('quick links and conversation search open real workspace views', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByText('Lumen Labs')).toBeVisible()

  await page.getByRole('button', { name: /^受信トレイ/ }).click()
  await expect(page.getByRole('dialog', { name: '受信トレイ' })).toBeVisible()
  await expect(page.getByText('未読のある会話をまとめています。')).toBeVisible()
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
  await page.getByRole('button', { name: 'general', exact: true }).click()
  await expect(page.getByRole('main').getByRole('heading', { name: 'general', exact: true })).toBeVisible()
})

test('DM details show the actual conversation participants', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByText('Lumen Labs')).toBeVisible()

  await page.getByRole('button', { name: /Orbit AI/ }).click()
  await expect(page.getByRole('heading', { name: '会話の詳細' })).toBeVisible()
  const details = page.locator('.details-panel')
  const detailMembers = details.locator('.member-list')
  await expect(detailMembers.getByText('Orbit AI', { exact: true })).toBeVisible()
  await expect(detailMembers.getByText('Taro Tanaka', { exact: true })).toBeVisible()
  await expect(detailMembers.getByText('Ayaka Mori', { exact: true })).toHaveCount(0)
  await expect(detailMembers.getByText('Ken Ito', { exact: true })).toHaveCount(0)
})

test('can edit the profile name and presence from the user card', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByText('Lumen Labs')).toBeVisible()

  await page.getByRole('button', { name: 'プロフィールを開く' }).first().click()
  await expect(page.getByRole('dialog', { name: 'プロフィール' })).toBeVisible()
  await page.getByRole('button', { name: 'プロフィールを閉じる' }).click()

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

test('creates a channel in the group whose plus button was clicked', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByText('Lumen Labs')).toBeVisible()

  await page.getByRole('button', { name: '開発にチャンネルを追加' }).click()
  const dialog = page.getByRole('dialog', { name: 'チャンネルを作成' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByLabel('所属')).toHaveValue('Engineering')
  await dialog.getByRole('checkbox', { name: /Ayaka Mori/ }).check()
  await expect(dialog.getByRole('checkbox', { name: /Ayaka Mori/ })).toBeChecked()

  const channelName = `e2e-channel-${Date.now()}`
  await dialog.getByLabel('チャンネル名').fill(channelName)
  await dialog.getByLabel('説明').fill('E2Eで作成したチャンネル')
  await dialog.getByRole('button', { name: '作成', exact: true }).click()
  await expect(page.getByRole('main').getByRole('heading', { name: channelName, exact: true })).toBeVisible()

  await page.getByRole('button', { name: 'プロダクトにチャンネルを追加' }).click()
  await expect(dialog).toBeVisible()
  await expect(dialog.getByLabel('所属')).toHaveValue('Product')
  await dialog.getByRole('button', { name: 'キャンセル', exact: true }).click()

  await page.getByRole('button', { name: 'ログアウト' }).click()
  await expect(page.getByRole('heading', { name: 'チームにログイン' })).toBeVisible()
  await page.getByLabel('メールアドレス').fill('ayaka@example.com')
  await page.getByLabel('パスワード').fill('demo-password')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByRole('button', { name: channelName, exact: true })).toBeVisible()
})
