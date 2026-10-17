import { test, expect, type Page } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'

test.use({ timezoneId: 'America/Los_Angeles' })

async function login(page: Page) {
  const username = `note_${randomBytes(6).toString('hex')}`
  const password = randomBytes(18).toString('hex')
  execFileSync(
    'go',
    [
      'run',
      './cmd/timememo-user',
      'create',
      '--username',
      username,
      '--name',
      '手记测试员',
      '--password-stdin',
    ],
    {
      input: password + '\n',
      env: { ...process.env, DATABASE_URL: process.env.E2E_DATABASE_URL },
      stdio: ['pipe', 'pipe', 'pipe'],
    },
  )
  await page.goto('/')
  await page.getByLabel('用户名', { exact: true }).fill(username)
  await page.getByLabel('密码', { exact: true }).fill(password)
  await page.getByRole('button', { name: '开启一段时光' }).click()
  await expect(page.getByRole('heading', { name: '记录此刻，留给时光。' })).toBeVisible()
}

test('今日手记在北京时间零点更新，并显示作者出处', async ({ page }, testInfo) => {
  test.skip(
    !process.env.E2E_DATABASE_URL || testInfo.project.name !== 'desktop',
    'Desktop daily note test with a disposable database',
  )
  // Use a US browser timezone to prove that the displayed date follows the server.
  await page.clock.install({ time: new Date('2026-10-17T23:59:00+08:00') })
  await page.clock.pauseAt(new Date('2026-10-17T23:59:59+08:00'))
  let tomorrow = false
  let requests = 0
  await page.route('**/api/daily-note', async (route) => {
    requests++
    await route.fulfill({
      json: {
        date: tomorrow ? '2026-10-18' : '2026-10-17',
        content: tomorrow
          ? '新的一天，把新的故事慢慢写给时光。'
          : '留下一段日常，也留住今天的阳光。',
        author: '林间',
        source: '生活手记',
        sourceUrl: 'https://hitokoto.cn/?uuid=12345678-1234-1234-1234-123456789abc',
        nextRefreshAt: tomorrow ? '2026-10-19T00:00:00+08:00' : '2026-10-18T00:00:00+08:00',
      },
    })
  })
  await login(page)
  await expect(page.locator('.today-note-content')).toHaveText('留下一段日常，也留住今天的阳光。')
  await expect(page.locator('.today-date')).toHaveText('2026年10月17日')
  await expect(page.locator('.today-note-attribution')).toHaveText('— 林间 · 生活手记')
  await expect(page.getByRole('link', { name: '在一言查看本句来源' })).toHaveAttribute(
    'rel',
    'noopener noreferrer',
  )
  tomorrow = true
  await page.clock.runFor(2000)
  await expect(page.locator('.today-note-content')).toHaveText('新的一天，把新的故事慢慢写给时光。')
  await expect(page.locator('.today-date')).toHaveText('2026年10月18日')
  expect(requests).toBeGreaterThanOrEqual(2)
  await page.screenshot({ path: testInfo.outputPath('daily-note.png'), fullPage: true })
})

test('今日手记加载失败时可以重试，不影响文章列表', async ({ page }, testInfo) => {
  test.skip(
    !process.env.E2E_DATABASE_URL || testInfo.project.name !== 'desktop',
    'Desktop daily note test with a disposable database',
  )
  let available = false
  await page.route('**/api/daily-note', async (route) => {
    if (!available) {
      await route.fulfill({ status: 503, json: { error: 'temporarily unavailable' } })
      return
    }
    await route.fulfill({
      json: {
        date: '2026-10-17',
        content: '留下一段日常，也留住今天的阳光。',
        author: '',
        source: '时光记',
        sourceUrl: '',
        nextRefreshAt: '2099-10-18T00:00:00+08:00',
      },
    })
  })
  await login(page)
  await expect(page.locator('.today-note')).toContainText('暂时无法加载手记')
  await expect(page.getByRole('heading', { name: '记录此刻，留给时光。' })).toBeVisible()
  available = true
  await page.locator('.today-note').getByRole('button', { name: '重试' }).click()
  await expect(page.locator('.today-note-content')).toHaveText('留下一段日常，也留住今天的阳光。')
  await expect(page.locator('.today-note-provider')).toHaveCount(0)
})
