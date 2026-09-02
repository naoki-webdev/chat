import { expect, test } from '@playwright/test'
import { login } from './support/auth'

test('shows channel members and edits channel settings from the header', async ({ page }) => {
  await login(page)

  const details = page.locator('.details-panel')
  await expect(details).toContainText('Mio Tanaka')
  await expect(details).toContainText('Orbit AI')
  await expect(details.getByRole('heading', { name: 'design-system', exact: true })).toBeVisible()
  await expect(details.getByText('会話', { exact: true })).toHaveCount(0)

  await page.getByRole('button', { name: 'メンバーを表示' }).click()
  await expect(details).toHaveCount(0)
  await expect(page.locator('.app-shell')).not.toHaveClass(/app-shell-with-details/)
  await page.getByRole('button', { name: 'メンバーを表示' }).click()
  await expect(details).toBeVisible()
  await expect(page.locator('.app-shell')).toHaveClass(/app-shell-with-details/)

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

test('hides channel settings from a regular channel member', async ({ page }) => {
  await login(page, 'ken@example.com')
  await page.getByRole('button', { name: /^design-system/ }).click()
  await expect(page.getByRole('main').getByRole('heading', { name: 'design-system', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'チャンネルを編集' })).toHaveCount(0)
})

test('syncs channel membership changes to another open client', async ({ browser }) => {
  const ownerContext = await browser.newContext()
  const memberContext = await browser.newContext()
  const ownerPage = await ownerContext.newPage()
  const memberPage = await memberContext.newPage()
  try {
    await ownerPage.goto('/')
    await ownerPage.getByRole('button', { name: 'ログイン' }).click()
    await expect(ownerPage.getByText('Lumen Labs')).toBeVisible()

    await memberPage.goto('/')
    await memberPage.getByLabel('メールアドレス').fill('ken@example.com')
    await memberPage.getByLabel('パスワード').fill('demo-password')
    await memberPage.getByRole('button', { name: 'ログイン' }).click()
    await expect(memberPage.getByText('Lumen Labs')).toBeVisible()
    const designSystemButton = memberPage.getByRole('button', { name: /^design-system/ })
    await expect(designSystemButton).toBeVisible()

    await ownerPage.getByRole('button', { name: 'チャンネルを編集' }).click()
    const dialog = ownerPage.getByRole('dialog', { name: 'チャンネルを編集' })
    await dialog.getByRole('checkbox', { name: /Ken Ito/ }).uncheck()
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(designSystemButton).toHaveCount(0)

    await ownerPage.getByRole('button', { name: 'チャンネルを編集' }).click()
    const restoreDialog = ownerPage.getByRole('dialog', { name: 'チャンネルを編集' })
    await restoreDialog.getByRole('checkbox', { name: /Ken Ito/ }).check()
    await restoreDialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(memberPage.getByRole('button', { name: /^design-system/ })).toBeVisible()
  } finally {
    await ownerContext.close()
    await memberContext.close()
  }
})

test('creates a channel in the group whose plus button was clicked', async ({ page }) => {
  await login(page)

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

  await page.getByRole('button', { name: 'プロフィールを開く' }).click()
  await page.getByRole('button', { name: 'ログアウト' }).click()
  await expect(page.getByRole('heading', { name: 'チームにログイン' })).toBeVisible()
  await page.getByLabel('メールアドレス').fill('ayaka@example.com')
  await page.getByLabel('パスワード').fill('demo-password')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByRole('button', { name: channelName, exact: true })).toBeVisible()
})
