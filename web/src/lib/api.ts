export interface User {
  id: number
  username: string
  displayName: string
  createdAt: string
}
export interface Post {
  id: number
  title: string
  body?: string
  excerpt: string
  category: string
  tags: string[]
  createdAt: string
  author: User
  commentCount: number
  bookmarked: boolean
  image: string
  readMinutes: number
}
export interface Comment {
  id: number
  body: string
  createdAt: string
  author: User
}
export interface Stats {
  posts: number
  comments: number
  members: number
  mine: number
  saved: number
  categories: Record<string, number>
  tags: { tag: string; count: number }[]
}
export interface PostList {
  posts: Post[]
  total: number
  page: number
  pageSize: number
}
export interface CommentList {
  comments: Comment[]
  total: number
  page: number
  pageSize: number
}
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}
export async function api<T>(url: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers)
  if (options.body && !(options.body instanceof FormData))
    headers.set('Content-Type', 'application/json')
  const response = await fetch(url, { ...options, headers, credentials: 'same-origin' })
  const data = await response.json().catch(() => ({}))
  if (!response.ok) {
    if (response.status === 401 && url !== '/api/login' && url !== '/api/me')
      window.dispatchEvent(new Event('session-expired'))
    throw new ApiError(response.status, data.error || '请求失败，请稍后重试')
  }
  return data as T
}
export async function uploadImage(file: File) {
  if (file.size > 12 * 1024 * 1024) throw new Error('图片不能超过 12 MB')
  const data = new FormData()
  data.append('image', file)
  return api<{ url: string; width: number; height: number; size: number; originalSize: number }>(
    '/api/uploads',
    { method: 'POST', body: data },
  )
}
export function dateText(s: string) {
  return new Date(s).toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' })
}
export function relativeTime(s: string) {
  const minutes = Math.max(0, Math.floor((Date.now() - new Date(s).getTime()) / 60000))
  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes} 分钟前`
  if (minutes < 1440) return `${Math.floor(minutes / 60)} 小时前`
  if (minutes < 10080) return `${Math.floor(minutes / 1440)} 天前`
  return new Date(s).toLocaleDateString('zh-CN', { month: 'long', day: 'numeric' })
}
