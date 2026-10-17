import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import {
  ArrowLeft,
  ArrowRight,
  Bold,
  Check,
  Eye,
  Feather,
  Heading2,
  ImagePlus,
  Italic,
  Leaf,
  LoaderCircle,
  LockKeyhole,
  Quote,
} from 'lucide-react'
import { toast } from 'sonner'
import { useApp } from '@/App'
import { api, imageUploadAccept, uploadImage } from '@/lib/api'
import { categories, Footer, Markdown } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'

interface Draft {
  title: string
  body: string
  category: string
  tags: string
}
const emptyDraft: Draft = { title: '', body: '', category: 'daily', tags: '' }
export function Editor() {
  const { user, refresh } = useApp()
  const draftKey = `timememo-draft-${user.id}`
  const [draft, setDraft] = useState<Draft>(() => {
    try {
      const stored = JSON.parse(sessionStorage.getItem(draftKey) || 'null')
      if (
        stored &&
        typeof stored.title === 'string' &&
        typeof stored.body === 'string' &&
        categories.some((c) => c.id === stored.category) &&
        typeof stored.tags === 'string'
      )
        return stored
    } catch {
      /* Start with an empty draft when browser storage is unavailable. */
    }
    return emptyDraft
  })
  const [preview, setPreview] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [publishing, setPublishing] = useState(false)
  const [confirm, setConfirm] = useState(false)
  const [error, setError] = useState('')
  const [stored, setStored] = useState(true)
  const bodyRef = useRef<HTMLTextAreaElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()
  useEffect(() => {
    const timer = setTimeout(() => {
      try {
        sessionStorage.setItem(draftKey, JSON.stringify(draft))
        setStored(true)
      } catch {
        setStored(false)
      }
    }, 400)
    return () => clearTimeout(timer)
  }, [draft, draftKey])
  function field<K extends keyof Draft>(key: K, value: Draft[K]) {
    setDraft((d) => ({ ...d, [key]: value }))
    setStored(false)
  }
  function insert(before: string, after = '') {
    const el = bodyRef.current
    const start = el?.selectionStart ?? draft.body.length
    const end = el?.selectionEnd ?? draft.body.length
    setDraft((d) => ({
      ...d,
      body: d.body.slice(0, start) + before + d.body.slice(start, end) + after + d.body.slice(end),
    }))
    setStored(false)
    requestAnimationFrame(() => {
      el?.focus()
      el?.setSelectionRange(start + before.length, end + before.length)
    })
  }
  async function upload(file?: File) {
    if (!file) return
    setUploading(true)
    try {
      const image = await uploadImage(file)
      setDraft((d) => ({ ...d, body: d.body + `\n\n![图片](${image.url})\n` }))
      toast.success(`图片已压缩并插入（${Math.ceil(image.size / 1024)} KB）`)
      setPreview(false)
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setUploading(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }
  function requestPublish(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    if (!draft.title.trim() || !draft.body.trim()) {
      setError('请填写标题和正文')
      return
    }
    const tags = draft.tags
      .split(/[,，]/)
      .map((t) => t.trim())
      .filter(Boolean)
    if (tags.length > 5 || tags.some((t) => Array.from(t).length > 20)) {
      setError('最多 5 个标签，每个不超过 20 个字符')
      return
    }
    setConfirm(true)
  }
  async function publish() {
    setPublishing(true)
    setError('')
    try {
      const result = await api<{ id: number }>('/api/posts', {
        method: 'POST',
        body: JSON.stringify({
          ...draft,
          tags: draft.tags
            .split(/[,，]/)
            .map((t) => t.trim())
            .filter(Boolean),
        }),
      })
      sessionStorage.removeItem(draftKey)
      refresh()
      toast.success('这段时光，已经被好好留下')
      navigate(`/posts/${result.id}`)
    } catch (e) {
      setError((e as Error).message)
      setConfirm(false)
    } finally {
      setPublishing(false)
    }
  }
  return (
    <div className="editor-page">
      <Link className="back-link" to="/">
        <ArrowLeft size={15} />
        回到时光广场
      </Link>
      <div className="editor-heading">
        <div>
          <span className="eyebrow">A MOMENT WORTH KEEPING</span>
          <h1>
            写下你的<span>这一刻。</span>
          </h1>
        </div>
        <Leaf size={34} strokeWidth={1} />
      </div>
      <form onSubmit={requestPublish}>
        <div className="editor-card">
          <div className="editor-category">
            <label htmlFor="category">这篇记录属于</label>
            <select
              id="category"
              value={draft.category}
              onChange={(e) => field('category', e.target.value)}
            >
              {categories.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
            <span className="draft-state">
              <Check size={12} />
              {stored ? '草稿已保存至本次浏览会话' : '草稿尚未保存'}
            </span>
          </div>
          <label htmlFor="post-title" className="sr-only">
            文章标题
          </label>
          <input
            id="post-title"
            className="editor-title-input"
            placeholder="给这段时光，一个名字…"
            value={draft.title}
            onChange={(e) => field('title', e.target.value)}
            maxLength={120}
            required
          />
          <div className="editor-toolbar">
            <div>
              <button
                type="button"
                aria-label="加粗"
                title="加粗"
                onClick={() => insert('**', '**')}
                disabled={preview}
              >
                <Bold size={17} />
              </button>
              <button
                type="button"
                aria-label="斜体"
                title="斜体"
                onClick={() => insert('*', '*')}
                disabled={preview}
              >
                <Italic size={17} />
              </button>
              <button
                type="button"
                aria-label="插入标题"
                title="插入标题"
                onClick={() => insert('\n## ')}
                disabled={preview}
              >
                <Heading2 size={19} />
              </button>
              <span className="toolbar-divider" />
              <button
                type="button"
                aria-label="插入引用"
                title="插入引用"
                onClick={() => insert('\n> ')}
                disabled={preview}
              >
                <Quote size={17} />
              </button>
              <button
                type="button"
                aria-label="插入图片"
                title="插入图片"
                onClick={() => fileRef.current?.click()}
                disabled={uploading}
              >
                {uploading ? (
                  <LoaderCircle size={17} className="animate-spin" />
                ) : (
                  <ImagePlus size={18} />
                )}
              </button>
              <input
                ref={fileRef}
                type="file"
                accept={imageUploadAccept}
                onChange={(e) => upload(e.target.files?.[0])}
                className="sr-only"
                tabIndex={-1}
                aria-label="选择文章图片"
              />
            </div>
            <button
              type="button"
              className={`preview-toggle ${preview ? 'selected' : ''}`}
              onClick={() => setPreview(!preview)}
              aria-pressed={preview}
            >
              <Eye size={15} />
              {preview ? '继续编辑' : '预览'}
            </button>
          </div>
          {preview ? (
            <div className="editor-preview">
              {draft.body ? (
                <Markdown>{draft.body}</Markdown>
              ) : (
                <p className="text-muted-foreground">你的文字，会在这里慢慢成形。</p>
              )}
            </div>
          ) : (
            <Textarea
              id="post-body"
              aria-label="文章正文"
              ref={bodyRef}
              className="editor-body"
              placeholder={
                '从一件小事，一个念头，或一束刚好落下的光开始…\n\n这里，装得下你的日常，也装得下你的天马行空。'
              }
              value={draft.body}
              onChange={(e) => field('body', e.target.value)}
              maxLength={50000}
              required
            />
          )}
          <div className="editor-bottom">
            <span>支持 Markdown · JPEG / PNG / WebP 自动压缩</span>
            <span>{Array.from(draft.body).length.toLocaleString()} / 50,000 字</span>
          </div>
          <div className="editor-tags">
            <label htmlFor="tags">添加标签</label>
            <Input
              id="tags"
              placeholder="例如：生活里的小美好，秋日（逗号分隔，最多 5 个）"
              value={draft.tags}
              onChange={(e) => field('tags', e.target.value)}
              maxLength={104}
            />
          </div>
        </div>
        <div className="publish-note">
          <LockKeyhole size={17} />
          <div>
            <strong>落笔之前，再读一遍。</strong>
            <p>文章发布后无法修改或删除。愿这里留下的，都是你想珍藏的表达。</p>
          </div>
        </div>
        {error && (
          <p className="form-error mb-4" role="alert">
            {error}
          </p>
        )}
        <div className="editor-actions">
          <Link to="/">稍后再写</Link>
          <Button size="lg" disabled={uploading || publishing}>
            <Feather size={16} />
            发布这段时光
            <ArrowRight size={16} />
          </Button>
        </div>
      </form>
      <Dialog
        open={confirm}
        onOpenChange={(v) => {
          if (!publishing) setConfirm(v)
        }}
      >
        <DialogContent>
          <span className="about-icon">
            <Feather size={25} />
          </span>
          <DialogTitle className="dialog-title">让这段时光，成为永远？</DialogTitle>
          <DialogDescription className="dialog-description">
            发布后，所有成员都可以阅读这篇文章。标题、正文、图片与标签都将永久留存，无法修改或删除。
          </DialogDescription>
          <div className="publish-preview-title">{draft.title}</div>
          <div className="dialog-actions">
            <Button variant="outline" disabled={publishing} onClick={() => setConfirm(false)}>
              再读一遍
            </Button>
            <Button onClick={publish} disabled={publishing}>
              {publishing ? '正在发布…' : '确认并发布'}
              <ArrowRight />
            </Button>
          </div>
        </DialogContent>
      </Dialog>
      <Footer />
    </div>
  )
}
