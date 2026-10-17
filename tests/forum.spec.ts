import { test, expect } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'

test('CLI 创建用户后可登录、发布图片文章、评论、收藏和退出', async ({ page }, testInfo) => {
  test.skip(
    !process.env.E2E_DATABASE_URL,
    'E2E_DATABASE_URL must point to a disposable test database',
  )
  const username = `qa_${randomBytes(6).toString('hex')}`
  const password = randomBytes(18).toString('hex')
  const title = `值得留下的一束光 ${username}`
  execFileSync(
    'go',
    [
      'run',
      './cmd/timememo-user',
      'create',
      '--username',
      username,
      '--name',
      '时光测试员',
      '--password-stdin',
    ],
    {
      input: password + '\n',
      env: { ...process.env, DATABASE_URL: process.env.E2E_DATABASE_URL },
      stdio: ['pipe', 'pipe', 'pipe'],
    },
  )
  const consoleErrors: string[] = []
  page.on('pageerror', (error) => consoleErrors.push(error.message))
  await page.goto('/')
  await expect(page.getByRole('heading', { name: '登录时光记' })).toBeVisible()
  await expect(page.getByText(title)).toHaveCount(0)
  await expect(page.getByRole('link', { name: /注册/ })).toHaveCount(0)
  await page.getByLabel('用户名', { exact: true }).fill(username)
  await page.getByLabel('密码', { exact: true }).fill(password)
  await page.getByRole('button', { name: '开启一段时光' }).click()
  await expect(page.getByRole('heading', { name: '记录此刻，留给时光。' })).toBeVisible()
  await page.getByRole('link', { name: '写一篇', exact: true }).click()
  await page.getByLabel('文章标题').fill(title)
  await page
    .getByLabel('文章正文')
    .fill('今天的阳光，刚好落在书页上。\n\n## 把平凡留住\n\n愿每一天，都有值得记录的小事。')
  await page
    .getByLabel('选择文章图片')
    .setInputFiles({
      name: 'moment.png',
      mimeType: 'image/png',
      buffer: Buffer.from(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGMomJAAAALUAWGohAHOAAAAAElFTkSuQmCC',
        'base64',
      ),
    })
  await expect(page.getByLabel('文章正文')).toHaveValue(/\/media\/[a-f0-9]{32}\.jpg/)
  await page.getByLabel('添加标签').fill('日常，光影')
  await page.getByRole('button', { name: '预览', exact: true }).click()
  await expect(page.getByRole('heading', { name: '把平凡留住' })).toBeVisible()
  await page.getByRole('button', { name: '发布这段时光' }).click()
  await expect(page.getByRole('dialog')).toContainText('无法修改或删除')
  await page.getByRole('button', { name: '确认并发布', exact: true }).click()
  await expect(page.getByRole('heading', { level: 1, name: title })).toBeVisible()
  await expect(page.locator('.article-content .markdown img')).toBeVisible()
  await page.getByLabel('写下评论', { exact: true }).fill('愿这些微小的美好，都被好好记住。')
  await page.getByRole('button', { name: '留下回应' }).click()
  await page.getByRole('button', { name: '确认发布评论' }).click()
  await expect(page.locator('.comment-list')).toContainText('愿这些微小的美好，都被好好记住。')
  await page.getByRole('button', { name: '收藏文章', exact: true }).click()
  await expect(page.getByRole('button', { name: '取消收藏', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  )
  await expect(
    page.getByRole('button', { name: /编辑文章|删除文章|编辑评论|删除评论/ }),
  ).toHaveCount(0)
  await page.getByRole('link', { name: '回到时光广场' }).click()
  await expect(page.getByRole('heading', { name: title })).toBeVisible()
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  )
  expect(overflow).toBe(false)
  await page.screenshot({ path: testInfo.outputPath('forum.png'), fullPage: true })
  if (testInfo.project.name === 'desktop') {
    await page.getByLabel('搜索文章').fill(username)
    await page.getByLabel('搜索文章').press('Enter')
    await expect(page.getByRole('heading', { name: '搜索结果' })).toBeVisible()
    await expect(page.getByRole('heading', { name: title })).toBeVisible()
  } else {
    await page.getByRole('button', { name: '展开导航' }).click()
    await expect(page.getByRole('link', { name: /我的收藏/ }).first()).toBeVisible()
    await page.getByLabel('在社区中搜索').fill(username)
    await page.getByLabel('在社区中搜索').press('Enter')
    await expect(page.getByRole('heading', { name: '搜索结果' })).toBeVisible()
    await expect(page.getByRole('heading', { name: title })).toBeVisible()
  }
  await page.getByRole('button', { name: '个人菜单' }).click()
  await page.getByRole('menuitem', { name: '我的收藏' }).click()
  await expect(page.getByRole('heading', { name: title })).toBeVisible()
  await page.getByRole('button', { name: '个人菜单' }).click()
  await page.getByRole('menuitem', { name: '退出登录' }).click()
  await expect(page.getByRole('heading', { name: '登录时光记' })).toBeVisible()
  const response = await page.request.get('/api/posts')
  expect(response.status()).toBe(401)
  expect(consoleErrors).toEqual([])
})
