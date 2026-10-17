import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import {
  ArrowDownWideNarrow,
  ArrowLeft,
  ArrowRight,
  ArrowUpRight,
  Bookmark,
  Check,
  ChevronRight,
  Feather,
  Leaf,
  MessageCircle,
  Search,
  Sparkles,
} from 'lucide-react'
import { toast } from 'sonner'
import { useApp } from '@/App'
import { api, dateText, relativeTime, type Post, type PostList } from '@/lib/api'
import { DailyNoteCard } from '@/components/daily-note'
import { Avatar, categories, CategoryBadge, Footer, TimeMemoIcon } from '@/components/shared'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export function Feed() {
  const [params, setParams] = useSearchParams()
  const [data, setData] = useState<PostList | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const { stats } = useApp()
  const query = params.toString()
  const category = categories.find((c) => c.id === params.get('category'))
  const title = params.has('mine')
    ? '我的记录'
    : params.has('bookmarked')
      ? '我的收藏'
      : params.has('q')
        ? '搜索结果'
        : params.has('tag')
          ? `# ${params.get('tag')}`
          : category?.name || '时光广场'
  const sort = params.get('sort') || 'latest'
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    let alive = true
    setLoading(true)
    setError('')
    api<PostList>(`/api/posts?${query}`)
      .then((d) => {
        if (alive) setData(d)
      })
      .catch((e) => {
        if (alive) setError(e.message)
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
  }, [query, retry])
  function update(key: string, value: string) {
    const next = new URLSearchParams(params)
    if (value) next.set(key, value)
    else next.delete(key)
    if (key !== 'page') next.delete('page')
    setParams(next)
  }
  return (
    <>
      <div className="content-columns">
        <div className="feed-column">
          <section className="welcome-banner">
            <div className="welcome-copy">
              <div className="banner-eyebrow">
                <span className="tiny-line" /> 日子缓缓 · 故事慢慢
              </div>
              <h1>
                记录此刻，<span>留给时光。</span>
              </h1>
              <p>
                把生活的细碎，写成岁月的注脚。
                <br />
                在这里，遇见文字，也遇见同频的你。
              </p>
              <Link to="/write">
                写下今天的故事 <ArrowRight size={15} />
              </Link>
            </div>
            <div className="banner-art" aria-hidden="true">
              <div className="banner-orbit" />
              <div className="banner-orbit orbit-small" />
              <div className="banner-sun" />
              <div className="banner-clock">
                <TimeMemoIcon size={66} strokeWidth={0.9} />
              </div>
              <Leaf className="banner-leaf" size={68} strokeWidth={0.8} />
              <span>MAKE TIME A MEMORY</span>
            </div>
          </section>
          <div className="feed-heading">
            <div>
              <h2>
                {title}
                <span>{!loading && data ? data.total : '—'}</span>
              </h2>
              {params.has('q') && <p>关于「{params.get('q')}」的记录</p>}
            </div>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="sm" className="sort-button">
                  <ArrowDownWideNarrow size={14} />
                  {sort === 'popular' ? '最多讨论' : '最新发布'}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => update('sort', '')}>
                  最新发布{sort === 'latest' && <Check size={14} />}
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => update('sort', 'popular')}>
                  最多讨论{sort === 'popular' && <Check size={14} />}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
          <div className="feed-tabs">
            <button
              className={
                !params.has('category') && !params.has('mine') && !params.has('bookmarked')
                  ? 'selected'
                  : ''
              }
              onClick={() => setParams({})}
            >
              全部记录
            </button>
            {categories.map((c) => (
              <button
                className={params.get('category') === c.id ? 'selected' : ''}
                key={c.id}
                onClick={() => setParams({ category: c.id })}
              >
                {c.name}
              </button>
            ))}
          </div>
          {loading ? (
            <div className="post-list" aria-label="正在加载文章">
              {[1, 2, 3].map((n) => (
                <div className="post-skeleton" key={n}>
                  <span />
                  <div />
                  <p />
                  <p />
                </div>
              ))}
            </div>
          ) : error ? (
            <div className="empty-state">
              <Search size={35} />
              <h3>这次没能找到时光</h3>
              <p role="alert">{error}</p>
              <Button variant="outline" onClick={() => setRetry((v) => v + 1)}>
                重新加载
              </Button>
            </div>
          ) : data?.posts.length ? (
            <>
              <div className="post-list">
                {data.posts.map((p) => (
                  <PostCard
                    key={p.id}
                    post={p}
                    onUnsave={() => {
                      if (params.has('bookmarked')) setRetry((x) => x + 1)
                    }}
                  />
                ))}
              </div>
              {data.total > 12 && (
                <div className="pagination">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={data.page <= 1}
                    onClick={() => update('page', String(data.page - 1))}
                  >
                    <ArrowLeft />
                    上一页
                  </Button>
                  <span>
                    {data.page} / {Math.ceil(data.total / 12)}
                  </span>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={data.page * 12 >= data.total}
                    onClick={() => update('page', String(data.page + 1))}
                  >
                    下一页
                    <ArrowRight />
                  </Button>
                </div>
              )}
              <p className="feed-end">
                <span />
                时光在继续，故事也一样
                <Leaf size={12} />
                <span />
              </p>
            </>
          ) : (
            <div className="empty-state">
              <span className="empty-icon">
                <Feather size={28} strokeWidth={1.3} />
              </span>
              <h3>
                {params.has('bookmarked')
                  ? '把喜欢的故事，留在这里'
                  : params.has('q')
                    ? '还没有找到相关记录'
                    : '这段时光，等你落下第一笔'}
              </h3>
              <p>
                {params.has('q')
                  ? '试试换一个关键词，或回广场看看。'
                  : params.has('bookmarked')
                    ? '点击文章旁的书签，下次就能轻松找到。'
                    : '从一件小事、一个念头开始，写下今天。'}
              </p>
              <Button asChild variant="outline">
                <Link to={params.has('q') || params.has('bookmarked') ? '/' : '/write'}>
                  {params.has('q') || params.has('bookmarked') ? '逛逛时光广场' : '写下第一篇记录'}
                  <ArrowRight />
                </Link>
              </Button>
            </div>
          )}
        </div>
        <aside className="right-sidebar">
          <DailyNoteCard />
          <section className="community-stats">
            <div className="right-section-heading">
              <h3>我们的小小天地</h3>
              <span className="live-dot" />
            </div>
            <div className="stats-grid">
              <div>
                <strong>{stats?.posts ?? '—'}</strong>
                <span>篇时光记录</span>
              </div>
              <div>
                <strong>{stats?.members ?? '—'}</strong>
                <span>位同路人</span>
              </div>
              <div>
                <strong>{stats?.comments ?? '—'}</strong>
                <span>次温暖回应</span>
              </div>
            </div>
            <p>每一份分享，都让这里更丰盈。</p>
          </section>
          <section className="popular-tags">
            <div className="right-section-heading">
              <h3>大家在聊</h3>
              <Sparkles size={14} />
            </div>
            {stats?.tags.length ? (
              <div className="tag-cloud">
                {stats.tags.map((t) => (
                  <Link key={t.tag} to={`/?tag=${encodeURIComponent(t.tag)}`}>
                    # {t.tag}
                    <span>{t.count}</span>
                  </Link>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground leading-7">
                故事里的关键词，
                <br />
                会在这里慢慢聚在一起。
              </p>
            )}
          </section>
          <section className="writing-prompt">
            <span className="prompt-label">
              <Feather size={14} />
              给今天的一个小提问
            </span>
            <h3>
              最近，有什么瞬间
              <br />
              让你觉得生活很美好？
            </h3>
            <Link to="/write">
              把它记下来
              <ArrowUpRight size={14} />
            </Link>
          </section>
          <p className="right-bottom">
            <LockTiny />
            仅成员可见 · 真诚自在地表达
          </p>
        </aside>
      </div>
      <Footer />
    </>
  )
}
function LockTiny() {
  return (
    <span className="lock-dot" aria-hidden="true">
      ◦
    </span>
  )
}

export function PostCard({ post, onUnsave }: { post: Post; onUnsave: () => void }) {
  const [saved, setSaved] = useState(post.bookmarked)
  const [busy, setBusy] = useState(false)
  const { refresh } = useApp()
  async function bookmark() {
    setBusy(true)
    try {
      await api(`/api/posts/${post.id}/bookmark`, {
        method: 'POST',
        body: JSON.stringify({ saved: !saved }),
      })
      setSaved(!saved)
      refresh()
      if (saved) onUnsave()
      toast.success(saved ? '已取消收藏' : '已收藏这段时光')
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <article className="post-card">
      <div className="post-author">
        <Avatar user={post.author} size="small" />
        <strong>{post.author.displayName}</strong>
        <span className="author-dot">·</span>
        <time dateTime={post.createdAt} title={dateText(post.createdAt)}>
          {relativeTime(post.createdAt)}
        </time>
        <CategoryBadge id={post.category} />
      </div>
      <div className={`post-card-content ${post.image ? 'with-image' : ''}`}>
        <div>
          <Link to={`/posts/${post.id}`} className="post-title-link">
            <h3>{post.title}</h3>
          </Link>
          <p className="post-excerpt">{post.excerpt}</p>
        </div>
        {post.image && (
          <Link
            to={`/posts/${post.id}`}
            className="post-thumbnail"
            tabIndex={-1}
            aria-hidden="true"
          >
            <img src={post.image} alt="" loading="lazy" />
          </Link>
        )}
      </div>
      <div className="post-card-footer">
        <div className="post-tags">
          {post.tags.slice(0, 3).map((t) => (
            <Link to={`/?tag=${encodeURIComponent(t)}`} key={t}>
              # {t}
            </Link>
          ))}
        </div>
        <div className="post-meta">
          <span className="read-time">{post.readMinutes} 分钟阅读</span>
          <Link to={`/posts/${post.id}#comments`} aria-label={`${post.commentCount} 条评论`}>
            <MessageCircle size={14} />
            <span>{post.commentCount}</span>
          </Link>
          <button
            onClick={bookmark}
            disabled={busy}
            aria-label={saved ? '取消收藏' : '收藏文章'}
            aria-pressed={saved}
            className={saved ? 'is-saved' : ''}
          >
            <Bookmark size={15} fill={saved ? 'currentColor' : 'none'} />
          </button>
        </div>
      </div>
    </article>
  )
}

export function Topics() {
  const { stats } = useApp()
  return (
    <>
      <div className="page-intro">
        <span className="eyebrow">FIND YOUR PEOPLE</span>
        <h1>
          循着话题，<span>遇见同频。</span>
        </h1>
        <p>总有一些共同的喜欢，让陌生的我们靠近一点。</p>
      </div>
      <div className="topics-grid">
        {categories.map((c) => (
          <Link to={`/?category=${c.id}`} className={`topic-card ${c.color}`} key={c.id}>
            <span className="topic-card-icon">
              <c.icon size={28} strokeWidth={1.4} aria-hidden="true" />
            </span>
            <h2>{c.name}</h2>
            <p>{c.description}</p>
            <div>
              <span>{stats?.categories[c.id] ?? 0} 篇记录</span>
              <ArrowUpRight size={19} />
            </div>
          </Link>
        ))}
      </div>
      <Footer />
    </>
  )
}
