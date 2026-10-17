import { useId, type SVGProps } from 'react'
import {
  ArrowUpRight,
  Leaf,
  LockKeyhole,
  BookOpen,
  Compass,
  Coffee,
  Lightbulb,
  Code2,
  Archive,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkBreaks from 'remark-breaks'
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
  {
    id: 'archive',
    name: '时光旧匣',
    description: '安放旧日的文字，珍藏尘封的时光。',
    icon: Archive,
    color: 'archive',
  },
]
export function TimeMemoIcon({
  size = 24,
  strokeWidth = 1.7,
  className,
  ...props
}: Omit<SVGProps<SVGSVGElement>, 'strokeWidth'> & {
  size?: number | string
  strokeWidth?: number
}) {
  const bookMaskId = useId()
  return (
    <svg
      width={size}
      height={size}
      viewBox="5 6 32 32"
      fill="none"
      stroke="currentColor"
      className={cn('timememo-icon', className)}
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      <defs>
        <mask id={bookMaskId} maskUnits="userSpaceOnUse" x="0" y="0" width="40" height="40">
          <rect width="40" height="40" fill="white" stroke="none" />
          <circle cx="29" cy="27" r="7" fill="black" stroke="none" />
        </mask>
      </defs>
      <path
        d="M20 12C16 9 11 10 7 12v17c4-2 9-3 13 0 4-3 9-2 13 0V12c-4-2-9-3-13 0Zm0 0v17"
        strokeWidth={strokeWidth}
        strokeLinejoin="round"
        mask={`url(#${bookMaskId})`}
      />
      <circle cx="29" cy="27" r="7" strokeWidth={strokeWidth * (1.5 / 1.7)} />
      <path
        d="M29 23v4l2.5 1.5"
        strokeWidth={strokeWidth * (1.5 / 1.7)}
        strokeLinecap="round"
      />
    </svg>
  )
}

export function Logo({ light = false }: { light?: boolean }) {
  return (
    <Link to="/" className={cn('brand', light && 'brand-light')} aria-label="时光记首页">
      <span className="brand-symbol" aria-hidden="true">
        <TimeMemoIcon size="100%" viewBox="0 0 40 40" />
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
        remarkPlugins={[remarkGfm, remarkBreaks]}
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
