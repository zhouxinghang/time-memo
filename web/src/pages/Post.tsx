import { useEffect, useRef, useState } from 'react'
import { Link, useLocation, useParams } from 'react-router-dom'
import {
  ArrowLeft,
  ArrowRight,
  Bookmark,
  Feather,
  ImagePlus,
  LoaderCircle,
  LockKeyhole,
  MessageCircle,
} from 'lucide-react'
import { toast } from 'sonner'
import { useApp } from '@/App'
import { api, dateText, relativeTime, uploadImage, type CommentList, type Post } from '@/lib/api'
import { Avatar, CategoryBadge, Footer, Markdown } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'

export function PostPage() {
  const { id } = useParams()
  const { user, refresh } = useApp()
  const [post, setPost] = useState<Post | null>(null)
  const [comments, setComments] = useState<CommentList | null>(null)
  const [page, setPage] = useState(1)
  const [version, setVersion] = useState(0)
  const [body, setBody] = useState('')
  const [error, setError] = useState('')
  const [commentError, setCommentError] = useState('')
  const [loading, setLoading] = useState(true)
  const [commentLoading, setCommentLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [saving, setSaving] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [confirm, setConfirm] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)
  const location = useLocation()
  useEffect(() => {
    setPage(1)
    setBody('')
    setPost(null)
    setComments(null)
    setError('')
    setCommentError('')
  }, [id])
  useEffect(() => {
    let alive = true
    setLoading(true)
    api<Post>(`/api/posts/${id}`)
      .then((p) => {
        if (alive) setPost(p)
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
  }, [id])
  useEffect(() => {
    let alive = true
    setCommentLoading(true)
    api<CommentList>(`/api/posts/${id}/comments?page=${page}`)
      .then((c) => {
        if (alive) {
          setComments(c)
          setCommentError('')
        }
      })
      .catch((e) => {
        if (alive) setCommentError(e.message)
      })
      .finally(() => {
        if (alive) setCommentLoading(false)
      })
    return () => {
      alive = false
    }
  }, [id, page, version])
  useEffect(() => {
    if (!loading && location.hash === '#comments')
      document.getElementById('comments')?.scrollIntoView({ behavior: 'instant' })
  }, [loading, location.hash])
  async function bookmark() {
    if (!post) return
    setSaving(true)
    try {
      await api(`/api/posts/${id}/bookmark`, {
        method: 'POST',
        body: JSON.stringify({ saved: !post.bookmarked }),
      })
      setPost({ ...post, bookmarked: !post.bookmarked })
      refresh()
      toast.success(post.bookmarked ? '已取消收藏' : '已收藏这段时光')
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setSaving(false)
    }
  }
  async function upload(file?: File) {
    if (!file) return
    setUploading(true)
    try {
      const img = await uploadImage(file)
      setBody((b) => b + `\n\n![图片](${img.url})\n`)
      toast.success('图片已压缩并插入评论')
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setUploading(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }
  async function publish() {
    setBusy(true)
    try {
      await api(`/api/posts/${id}/comments`, { method: 'POST', body: JSON.stringify({ body }) })
      setBody('')
      setConfirm(false)
      setPage(Math.ceil(((comments?.total || 0) + 1) / 30))
      setVersion((x) => x + 1)
      setPost((p) => (p ? { ...p, commentCount: p.commentCount + 1 } : p))
      refresh()
      toast.success('你的回应，已经留下')
    } catch (e) {
      setConfirm(false)
      setCommentError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  if (loading)
    return (
      <div className="article-loading" aria-label="正在加载文章">
        <div className="post-skeleton">
          <span />
          <div />
          <p />
          <p />
        </div>
      </div>
    )
  if (error || !post)
    return (
      <div className="empty-state">
        <Feather size={35} />
        <h2>这段时光暂时无法打开</h2>
        <p role="alert">{error}</p>
        <Button asChild variant="outline">
          <Link to="/">回到时光广场</Link>
        </Button>
      </div>
    )
  return (
    <div className="article-page">
      <Link to="/" className="back-link">
        <ArrowLeft size={15} />
        回到时光广场
      </Link>
      <article className="article-content">
        <div className="article-category">
          <CategoryBadge id={post.category} />
          <span>{post.readMinutes} 分钟阅读</span>
        </div>
        <h1>{post.title}</h1>
        <div className="article-byline">
          <Avatar user={post.author} />
          <div>
            <strong>{post.author.displayName}</strong>
            <span>
              <time dateTime={post.createdAt}>{dateText(post.createdAt)}</time>
              <span>·</span>留下的时光
            </span>
          </div>
          <Button
            variant="ghost"
            size="icon"
            aria-label={post.bookmarked ? '取消收藏' : '收藏文章'}
            aria-pressed={post.bookmarked}
            disabled={saving}
            onClick={bookmark}
          >
            <Bookmark size={19} fill={post.bookmarked ? 'currentColor' : 'none'} />
          </Button>
        </div>
        <Markdown>{post.body || ''}</Markdown>
        <div className="article-tags">
          {post.tags.map((t) => (
            <Link to={`/?tag=${encodeURIComponent(t)}`} key={t}>
              # {t}
            </Link>
          ))}
        </div>
        <div className="article-ending">
          <span />
          <Feather size={17} />
          <span />
        </div>
        <p className="article-permanent">
          <LockKeyhole size={12} />
          这段文字已成为时光的一部分，永久留存。
        </p>
      </article>
      <section className="comments-section" id="comments">
        <div className="comments-heading">
          <h2>
            留下你的回响<span>{comments?.total ?? post.commentCount}</span>
          </h2>
          <MessageCircle size={21} strokeWidth={1.3} />
        </div>
        <form
          className="comment-form"
          onSubmit={(e) => {
            e.preventDefault()
            if (body.trim()) setConfirm(true)
          }}
        >
          <Avatar user={user} size="small" />
          <div className="comment-input-wrap">
            <label className="sr-only" htmlFor="comment-body">
              写下评论
            </label>
            <Textarea
              id="comment-body"
              placeholder="读到这里，有什么想说的？"
              value={body}
              onChange={(e) => setBody(e.target.value)}
              maxLength={5000}
              required
            />
            <div className="comment-toolbar">
              <button
                type="button"
                aria-label="为评论插入图片"
                title="插入图片"
                disabled={uploading}
                onClick={() => fileRef.current?.click()}
              >
                {uploading ? (
                  <LoaderCircle className="animate-spin" size={17} />
                ) : (
                  <ImagePlus size={17} />
                )}
              </button>
              <input
                className="sr-only"
                tabIndex={-1}
                ref={fileRef}
                type="file"
                accept="image/jpeg,image/png,image/webp"
                aria-label="选择评论图片"
                onChange={(e) => upload(e.target.files?.[0])}
              />
              <span>发布后不可修改或删除</span>
              <Button size="sm" disabled={!body.trim() || busy || uploading}>
                留下回应
                <ArrowRight />
              </Button>
            </div>
          </div>
        </form>
        {commentError && (
          <div role="alert" className="form-error mb-5">
            {commentError}
            <button className="ml-3 underline" onClick={() => setVersion((v) => v + 1)}>
              重新加载评论
            </button>
          </div>
        )}
        {commentLoading ? (
          <p className="loading-comments">正在收集回响…</p>
        ) : comments?.comments.length ? (
          <>
            <div className="comment-list">
              {comments.comments.map((c) => (
                <article className="comment" key={c.id}>
                  <Avatar user={c.author} size="small" />
                  <div>
                    <div className="comment-author">
                      <strong>{c.author.displayName}</strong>
                      {c.author.id === post.author.id && <span className="author-label">作者</span>}
                      <time dateTime={c.createdAt}>{relativeTime(c.createdAt)}</time>
                    </div>
                    <Markdown>{c.body}</Markdown>
                  </div>
                </article>
              ))}
            </div>
            {comments.total > 30 && (
              <div className="pagination">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page <= 1}
                  onClick={() => setPage((p) => p - 1)}
                >
                  上一页
                </Button>
                <span>
                  {page} / {Math.ceil(comments.total / 30)}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page * 30 >= comments.total}
                  onClick={() => setPage((p) => p + 1)}
                >
                  下一页
                </Button>
              </div>
            )}
          </>
        ) : (
          <p className="comments-empty">一篇记录，一场对话。成为第一个回应的人吧。</p>
        )}
      </section>
      <Dialog
        open={confirm}
        onOpenChange={(v) => {
          if (!busy) setConfirm(v)
        }}
      >
        <DialogContent>
          <DialogTitle className="dialog-title">留下一份真诚的回应</DialogTitle>
          <DialogDescription className="dialog-description">
            评论与图片发布后将永久留存，无法修改或删除。确认这就是你想说的话吗？
          </DialogDescription>
          <div className="comment-preview">
            <Markdown>{body}</Markdown>
          </div>
          <div className="dialog-actions">
            <Button variant="outline" disabled={busy} onClick={() => setConfirm(false)}>
              再想一想
            </Button>
            <Button onClick={publish} disabled={busy}>
              {busy ? '正在发布…' : '确认发布评论'}
              <ArrowRight />
            </Button>
          </div>
        </DialogContent>
      </Dialog>
      <Footer />
    </div>
  )
}
