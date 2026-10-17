import { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { Link, NavLink, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import {
  ArrowRight,
  ArrowUpRight,
  Bookmark,
  ChevronDown,
  Feather,
  Hourglass,
  Leaf,
  LockKeyhole,
  LogOut,
  Menu,
  Search,
  Sparkles,
  X,
} from 'lucide-react'
import { toast } from 'sonner'
import { api, type Stats, type User } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Avatar, categories, Footer, Logo } from '@/components/shared'
import { Feed, Topics } from '@/pages/Feed'
import { Editor } from '@/pages/Editor'
import { PostPage } from '@/pages/Post'

interface AppContextValue {
  user: User
  stats: Stats | null
  refresh: () => void
}
const AppContext = createContext<AppContextValue>(null!)
export const useApp = () => useContext(AppContext)

export default function App() {
  const [user, setUser] = useState<User | null>(null)
  const [checking, setChecking] = useState(true)
  const [unavailable, setUnavailable] = useState(false)
  const checkSession = useCallback(() => {
    setChecking(true)
    setUnavailable(false)
    api<User>('/api/me')
      .then(setUser)
      .catch((e: Error & { status?: number }) => {
        if (e.status !== 401) setUnavailable(true)
      })
      .finally(() => setChecking(false))
  }, [])
  useEffect(() => {
    checkSession()
  }, [checkSession])
  useEffect(() => {
    const expired = () => {
      setUser(null)
      toast.error('登录已过期，请重新登录')
    }
    window.addEventListener('session-expired', expired)
    return () => window.removeEventListener('session-expired', expired)
  }, [])
  if (checking)
    return (
      <div className="loading-screen">
        <Hourglass className="text-primary animate-pulse" size={30} />
        <p>正在打开时光记…</p>
      </div>
    )
  if (unavailable)
    return (
      <div className="loading-screen">
        <Logo />
        <p>暂时无法连接时光记</p>
        <Button onClick={checkSession}>重新连接</Button>
      </div>
    )
  return user ? (
    <Community user={user} onLogout={() => setUser(null)} />
  ) : (
    <Login onLogin={setUser} />
  )
}

function Login({ onLogin }: { onLogin: (user: User) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [remember, setRemember] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      onLogin(
        await api<User>('/api/login', {
          method: 'POST',
          body: JSON.stringify({ username, password, remember }),
        }),
      )
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="login-page">
      <div className="login-story">
        <Logo light />
        <div className="login-story-content">
          <span className="eyebrow">
            <span className="tiny-line" /> A LITTLE PLACE FOR YOUR STORIES
          </span>
          <h1>
            把此刻，
            <br />
            写给以后的自己<span>。</span>
          </h1>
          <p>
            日常的片段，偶然的灵感，途中的风景。
            <br />
            在这里，认真记录，也慢慢相遇。
          </p>
          <div className="story-art" aria-hidden="true">
            <div className="art-orbit orbit-one" />
            <div className="art-orbit orbit-two" />
            <div className="art-orbit orbit-three" />
            <div className="art-sun" />
            <div className="art-window">
              <Hourglass size={88} strokeWidth={0.8} />
              <span>TIME, WELL KEPT.</span>
            </div>
            <Leaf className="art-leaf" size={92} strokeWidth={0.7} />
            <span className="art-caption">每一个平凡的日子，都有值得留下的光。</span>
          </div>
        </div>
        <span className="login-copyright">© {new Date().getFullYear()} TimeMemo · 时光记</span>
      </div>
      <main className="login-form-side">
        <div className="mobile-brand">
          <Logo />
        </div>
        <div className="login-form-wrap">
          <span className="login-welcome">
            <Sparkles size={16} /> 好久不见，欢迎回来
          </span>
          <h2>登录时光记</h2>
          <p className="login-subtitle">让故事继续，让时光有迹可循。</p>
          <form onSubmit={submit}>
            <div className="field">
              <label htmlFor="username">用户名</label>
              <Input
                id="username"
                name="username"
                autoComplete="username"
                placeholder="输入你的用户名"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                required
                maxLength={32}
                autoCapitalize="none"
                autoCorrect="off"
              />
            </div>
            <div className="field">
              <label htmlFor="password">密码</label>
              <Input
                id="password"
                name="password"
                type="password"
                autoComplete="current-password"
                placeholder="输入你的密码"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                maxLength={72}
              />
            </div>
            <label className="remember-label">
              <Checkbox checked={remember} onCheckedChange={(v) => setRemember(v === true)} />
              30 天内保持登录
            </label>
            {error && (
              <p className="form-error" role="alert">
                {error}
              </p>
            )}
            <Button className="login-submit" size="lg" disabled={busy}>
              {busy ? '正在登录…' : '开启一段时光'}
              <ArrowRight size={16} />
            </Button>
          </form>
          <div className="login-invite">
            <LockKeyhole size={16} />
            <p>
              这里是一处仅对成员开放的小天地。
              <br />
              <span>如需加入，请联系社区管理者获取账号。</span>
            </p>
          </div>
        </div>
        <div className="login-bottom">
          <Leaf size={13} /> 慢一点，也没关系。
        </div>
      </main>
    </div>
  )
}

function Community({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [stats, setStats] = useState<Stats | null>(null)
  const [about, setAbout] = useState(false)
  const [mobile, setMobile] = useState(false)
  const [search, setSearch] = useState('')
  const [revision, setRevision] = useState(0)
  const location = useLocation()
  const navigate = useNavigate()
  const refresh = useCallback(() => setRevision((x) => x + 1), [])
  useEffect(() => {
    let alive = true
    api<Stats>('/api/stats')
      .then((data) => {
        if (alive) setStats(data)
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [revision, location.pathname])
  useEffect(() => {
    setMobile(false)
    setSearch(new URLSearchParams(location.search).get('q') || '')
  }, [location.pathname, location.search])
  async function logout() {
    try {
      await api('/api/logout', { method: 'POST' })
      onLogout()
    } catch (e) {
      toast.error((e as Error).message)
    }
  }
  return (
    <AppContext.Provider value={{ user, stats, refresh }}>
      <header className="site-header">
        <div className="header-inner">
          <Logo />
          <nav className="header-nav" aria-label="主导航">
            <NavLink to="/" end>
              时光广场
            </NavLink>
            <NavLink to="/topics">话题</NavLink>
            <button onClick={() => setAbout(true)}>
              关于时光记
              <ArrowUpRight size={12} />
            </button>
          </nav>
          <div className="header-actions">
            <form
              className="header-search"
              onSubmit={(e) => {
                e.preventDefault()
                navigate(search.trim() ? `/?q=${encodeURIComponent(search.trim())}` : '/')
              }}
              role="search"
            >
              <Search size={16} />
              <input
                aria-label="搜索文章"
                placeholder="搜索时光里的故事"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                maxLength={100}
              />
              <kbd>↵</kbd>
            </form>
            <Button asChild className="header-write">
              <Link to="/write">
                <Feather />
                写一篇
              </Link>
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <button className="user-menu" aria-label="个人菜单">
                  <Avatar user={user} size="small" />
                  <ChevronDown size={13} />
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <div className="px-3 py-2">
                  <strong>{user.displayName}</strong>
                  <p className="mt-1 text-xs text-muted-foreground">@{user.username}</p>
                </div>
                <DropdownMenuItem onSelect={() => navigate('/?mine=true')}>
                  我的记录
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => navigate('/?bookmarked=true')}>
                  <Bookmark size={14} />
                  我的收藏
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={logout}>
                  <LogOut size={14} />
                  退出登录
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <Button
              variant="ghost"
              size="icon"
              className="mobile-menu-toggle"
              onClick={() => setMobile(!mobile)}
              aria-label={mobile ? '收起导航' : '展开导航'}
              aria-expanded={mobile}
            >
              {mobile ? <X /> : <Menu />}
            </Button>
          </div>
        </div>
      </header>
      <div className="community-layout">
        <aside className={`left-sidebar ${mobile ? 'is-open' : ''}`}>
          <Sidebar openAbout={() => setAbout(true)} />
        </aside>
        <main className="main-content">
          <Routes>
            <Route path="/" element={<Feed />} />
            <Route path="/topics" element={<Topics />} />
            <Route path="/write" element={<Editor />} />
            <Route path="/posts/:id" element={<PostPage />} />
            <Route
              path="*"
              element={
                <div className="empty-state">
                  <Hourglass size={40} />
                  <h2>这段时光还未留下记录</h2>
                  <Button asChild>
                    <Link to="/">回到时光广场</Link>
                  </Button>
                </div>
              }
            />
          </Routes>
        </main>
      </div>
      <Dialog open={about} onOpenChange={setAbout}>
        <DialogContent>
          <span className="about-icon">
            <Hourglass size={30} strokeWidth={1.3} />
          </span>
          <DialogTitle className="dialog-title">给时光，一个落脚的地方。</DialogTitle>
          <DialogDescription className="dialog-description">
            时光记 TimeMemo
            是一个认真记录、真诚交流的成员社区。让生活里的小事、灵感与思考，有一处安放。
          </DialogDescription>
          <div className="about-rules">
            <p>
              <span>01</span>写下真实的自己，尊重每一种表达。
            </p>
            <p>
              <span>02</span>文章与评论发布后永久留存，无法修改或删除。
            </p>
            <p>
              <span>03</span>仅成员可阅读内容，每位成员都能平等交流。
            </p>
          </div>
          <Button className="w-full mt-5" onClick={() => setAbout(false)}>
            很高兴，在这里遇见你
            <Leaf />
          </Button>
        </DialogContent>
      </Dialog>
    </AppContext.Provider>
  )
}

function Sidebar({ openAbout }: { openAbout: () => void }) {
  const { user, stats } = useApp()
  const location = useLocation()
  const params = new URLSearchParams(location.search)
  const [query, setQuery] = useState('')
  const navigate = useNavigate()
  const home = location.pathname === '/'
  return (
    <>
      <form
        className="sidebar-search"
        role="search"
        onSubmit={(e) => {
          e.preventDefault()
          navigate(query.trim() ? `/?q=${encodeURIComponent(query.trim())}` : '/')
        }}
      >
        <Search size={15} />
        <input
          aria-label="在社区中搜索"
          placeholder="搜索时光里的故事"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          maxLength={100}
        />
        <button aria-label="执行搜索">
          <ArrowRight size={15} />
        </button>
      </form>
      <div className="sidebar-section">
        <p className="sidebar-label">我的时光</p>
        <Link to="/" className={`sidebar-link ${home && !location.search ? 'active' : ''}`}>
          <span className="nav-icon">
            <Hourglass size={17} />
          </span>
          时光广场<span className="nav-count">{stats?.posts ?? '—'}</span>
        </Link>
        <Link
          to="/?mine=true"
          className={`sidebar-link ${home && params.has('mine') ? 'active' : ''}`}
        >
          <span className="nav-icon">
            <Feather size={17} />
          </span>
          我的记录<span className="nav-count">{stats?.mine ?? '—'}</span>
        </Link>
        <Link
          to="/?bookmarked=true"
          className={`sidebar-link ${home && params.has('bookmarked') ? 'active' : ''}`}
        >
          <span className="nav-icon">
            <Bookmark size={17} />
          </span>
          我的收藏<span className="nav-count">{stats?.saved ?? '—'}</span>
        </Link>
      </div>
      <div className="sidebar-section topic-navigation">
        <div className="sidebar-label">
          循着话题，发现同频
          <Link to="/topics" aria-label="查看所有话题">
            <ArrowUpRight size={13} />
          </Link>
        </div>
        {categories.map((c) => (
          <Link
            to={`/?category=${c.id}`}
            className={`sidebar-link ${home && params.get('category') === c.id ? 'active' : ''}`}
            key={c.id}
          >
            <span className={`topic-dot ${c.color}`} />
            <span>{c.name}</span>
            <span className="nav-count">{stats?.categories[c.id] ?? 0}</span>
          </Link>
        ))}
      </div>
      <div className="sidebar-note">
        <span className="note-decoration">
          <Leaf size={22} strokeWidth={1.4} />
        </span>
        <h3>
          文字会记得，
          <br />
          我们曾经来过。
        </h3>
        <p>
          在时光记，发布后的文字
          <br />
          无法修改或删除。
          <br />
          愿每次落笔，都是心之所向。
        </p>
        <button onClick={openAbout}>
          认识时光记
          <ArrowUpRight size={13} />
        </button>
      </div>
      <div className="sidebar-user">
        <Avatar user={user} size="small" />
        <div>
          <strong>{user.displayName}</strong>
          <span>今天，也值得被记住。</span>
        </div>
      </div>
    </>
  )
}
