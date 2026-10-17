import { useEffect, useState } from 'react'
import { Leaf } from 'lucide-react'
import { api, type DailyNote } from '@/lib/api'

export function DailyNoteCard() {
  const [note, setNote] = useState<DailyNote | null>(null)
  const [error, setError] = useState(false)
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    let active = true
    let running = false
    let timer: ReturnType<typeof setTimeout> | undefined
    async function load() {
      if (!active || running || document.visibilityState === 'hidden') return
      running = true
      clearTimeout(timer)
      try {
        const next = await api<DailyNote>('/api/daily-note')
        if (!active) return
        setNote(next)
        setError(false)
        // Refresh at Beijing midnight; hourly checks also tolerate clock changes.
        const delay = Math.max(
          1000,
          Math.min(3_600_000, Date.parse(next.nextRefreshAt) - Date.now() + 250),
        )
        timer = setTimeout(load, delay)
      } catch {
        if (active) {
          setError(true)
          timer = setTimeout(load, 60_000)
        }
      } finally {
        running = false
      }
    }
    const onVisibility = () => {
      if (document.visibilityState === 'visible') void load()
    }
    void load()
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      active = false
      clearTimeout(timer)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [retry])
  const parts = note?.date.split('-')
  const date = parts ? `${parts[0]}年${Number(parts[1])}月${Number(parts[2])}日` : ''
  return (
    <section className="today-note" aria-labelledby="daily-note-heading">
      <div className="today-note-heading">
        <span className="tiny-line" />
        <h2 id="daily-note-heading">今日手记</h2>
        <Leaf size={14} />
      </div>
      {note ? (
        <>
          <p className="today-note-content">{note.content}</p>
          <p className="today-note-attribution">
            {note.author ? `— ${note.author} · ` : ''}
            {note.source}
          </p>
          <div className="today-note-footer">
            <time className="today-date" dateTime={note.date}>
              {date}
            </time>
            {note.sourceUrl && (
              <a
                className="today-note-provider"
                href={note.sourceUrl}
                target="_blank"
                rel="noopener noreferrer"
                aria-label="在一言查看本句来源"
              >
                一言 ↗
              </a>
            )}
          </div>
        </>
      ) : error ? (
        <p className="today-note-status" role="status">
          暂时无法加载手记。<button onClick={() => setRetry((v) => v + 1)}>重试</button>
        </p>
      ) : (
        <p className="today-note-status" role="status">
          正在翻开今日手记…
        </p>
      )}
      {note && error && (
        <p className="today-note-status" role="status">
          暂未同步最新手记，稍后自动重试。
        </p>
      )}
    </section>
  )
}
