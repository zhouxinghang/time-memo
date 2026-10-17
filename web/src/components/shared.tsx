import {
  Hourglass,
  ArrowUpRight,
  Leaf,
  LockKeyhole,
  BookOpen,
  Compass,
  Coffee,
  Lightbulb,
  Code2,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Link } from 'react-router-dom'
import type { User } from '@/lib/api'
import { cn } from '@/lib/utils'

export const categories = [
  {
    id: 'daily',
    name: '日常碎片',
    description: '把平凡的日子，过成值得记录的生活。',
    icon: Coffee,
    color: 'ochre',
  },
  {
    id: 'thoughts',
    name: '随想与感悟',
    description: '留一点空白，让想法慢慢生长。',
    icon: Lightbulb,
    color: 'green',
  },
  {
    id: 'travel',
    name: '在路上',
    description: '走过的路，和路上遇见的风景。',
    icon: Compass,
    color: 'blue',
  },
  {
    id: 'culture',
    name: '书影音',
    description: '在别人的故事里，与自己相遇。',
    icon: BookOpen,
    color: 'purple',
  },
  {
    id: 'tech',
    name: '探索与创造',
    description: '保持好奇，分享每一个新的发现。',
    icon: Code2,
    color: 'green',
  },
]
export function Logo({ light = false }: { light?: boolean }) {
  return (
    <Link to="/" className={cn('brand', light && 'brand-light')} aria-label="时光记首页">
      <span className="brand-symbol">
        <Hourglass size={25} strokeWidth={1.6} />
      </span>
      <span className="brand-name">
        时光记<span>TimeMemo</span>
      </span>
    </Link>
  )
}
export function Avatar({
  user,
  size = 'normal',
}: {
  user: User
  size?: 'small' | 'normal' | 'large'
}) {
  const colors = ['#e7ede5', '#ede4d9', '#e2e9ef', '#eee3e3', '#e7e3ed']
  return (
    <span
      className={cn('avatar', `avatar-${size}`)}
      style={{ background: colors[user.id % colors.length] }}
      aria-hidden="true"
    >
      {Array.from(user.displayName)[0]}
    </span>
  )
}
export function CategoryBadge({ id }: { id: string }) {
  const c = categories.find((c) => c.id === id)
  return <span className={cn('category-badge', c?.color)}>{c?.name || id}</span>
}
export function Markdown({ children }: { children: string }) {
  return (
    <div className="markdown">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          img: ({ src, alt }) =>
            typeof src === 'string' && /^\/media\/[a-f0-9]{32}\.jpg$/.test(src) ? (
              <img src={src} alt={alt || '文章图片'} loading="lazy" />
            ) : (
              <span className="text-muted-foreground">[请通过上传按钮插入图片]</span>
            ),
          a: ({ href, children }) => (
            <a href={href} target="_blank" rel="noopener noreferrer">
              {children}
              <ArrowUpRight size={12} className="inline" />
            </a>
          ),
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  )
}
export function Footer() {
  return (
    <footer className="site-footer">
      <span>© {new Date().getFullYear()} 时光记 TimeMemo</span>
      <span>
        <Leaf size={12} /> 让每一次表达，都有迹可循。
      </span>
    </footer>
  )
}
export function ImmutableNote() {
  return (
    <span className="immutable-note">
      <LockKeyhole size={12} /> 发布即留存，每一次表达都值得认真对待
    </span>
  )
}
