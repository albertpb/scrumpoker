import { FormEvent, useEffect, useRef, useState } from 'react'
import { Check, Coffee, Copy, Eye, LogOut, RefreshCw, Users, Wifi, WifiOff } from 'lucide-react'

type Participant = {
  id: string
  name: string
  hasVoted: boolean
  vote: string | null
}

type Room = {
  code: string
  name: string
  round: number
  revealed: boolean
  hostId: string
  participants: Participant[]
  onlineParticipantIds: string[]
}

type Session = { participantId: string; room: Room }

const votes = ['0', '1', '2', '3', '5', '8', '13', '21', '34', '55', '89', '?', 'coffee']

function roomCodeFromPath() {
  const match = window.location.pathname.match(/^\/room\/([A-Z0-9]+)\/?$/i)
  return match?.[1].toUpperCase() ?? null
}

function storedParticipant(code: string) {
  return localStorage.getItem(`pointdeck:${code}`)
}

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...options,
    headers: { 'Content-Type': 'application/json', ...options?.headers },
  })
  const body = await response.json()
  if (!response.ok) throw new Error(body.error || 'Request failed')
  return body as T
}

function Brand() {
  return (
    <button className="group flex items-center gap-3" onClick={() => (window.location.href = '/')}>
      <span className="grid size-10 rotate-3 place-items-center rounded-xl bg-[var(--coral)] text-sm font-black text-[var(--ink)] shadow-[3px_3px_0_var(--ink)] transition-transform group-hover:-rotate-3">SP</span>
      <span className="text-xl font-black tracking-[-0.04em] text-[var(--ink)]">ScrumPoker</span>
    </button>
  )
}

function Landing({ defaultCode }: { defaultCode: string | null }) {
  const [mode, setMode] = useState<'create' | 'join'>(defaultCode ? 'join' : 'create')
  const [roomName, setRoomName] = useState('Sprint planning')
  const [userName, setUserName] = useState('')
  const [code, setCode] = useState(defaultCode ?? '')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      const result = mode === 'create'
        ? await request<Session>('/api/rooms', { method: 'POST', body: JSON.stringify({ roomName, userName }) })
        : await request<Session>(`/api/rooms/${code.trim()}/join`, { method: 'POST', body: JSON.stringify({ userName }) })
      localStorage.setItem(`pointdeck:${result.room.code}`, result.participantId)
      window.location.href = `/room/${result.room.code}`
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Something went wrong')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="min-h-screen overflow-hidden px-5 py-6 sm:px-10">
      <nav className="mx-auto flex max-w-6xl items-center justify-between">
        <Brand />
        <span className="hidden rounded-full border border-black/10 bg-white/50 px-4 py-2 text-xs font-bold uppercase tracking-[0.16em] text-[var(--green)] sm:block">Realtime planning poker</span>
      </nav>

      <section className="mx-auto grid max-w-6xl items-center gap-14 pb-16 pt-16 lg:grid-cols-[1.05fr_.95fr] lg:pt-24">
        <div className="relative">
          <div className="mb-7 inline-flex -rotate-1 items-center gap-2 rounded-full border-2 border-[var(--ink)] bg-[var(--yellow)] px-4 py-2 text-sm font-extrabold shadow-[3px_3px_0_var(--ink)]">
            <span className="size-2 rounded-full bg-[var(--green)]" /> No sign-up. No fuss.
          </div>
          <h1 className="max-w-2xl text-6xl font-black leading-[0.9] tracking-[-0.065em] text-[var(--ink)] sm:text-7xl lg:text-[5.5rem]">
            Make estimates, <span className="relative text-[var(--green)]">not noise.<span className="absolute -bottom-2 left-0 h-2 w-full -rotate-1 rounded-full bg-[var(--coral)]" /></span>
          </h1>
          <p className="mt-9 max-w-xl text-lg font-medium leading-relaxed text-[var(--muted)] sm:text-xl">
            A focused room for teams to vote together, reveal together, and move work forward.
          </p>
          <div className="mt-10 flex flex-wrap gap-7 text-sm font-bold text-[var(--ink)]">
            <span className="flex items-center gap-2"><Check className="size-5 text-[var(--green)]" /> Instant rooms</span>
            <span className="flex items-center gap-2"><Check className="size-5 text-[var(--green)]" /> Live votes</span>
            <span className="flex items-center gap-2"><Check className="size-5 text-[var(--green)]" /> Zero accounts</span>
          </div>
        </div>

        <div className="relative mx-auto w-full max-w-lg">
          <div className="absolute -right-8 -top-8 size-24 rotate-12 rounded-3xl border-2 border-[var(--ink)] bg-[var(--coral)] shadow-[5px_5px_0_var(--ink)]" />
          <div className="absolute -bottom-6 -left-7 grid size-20 -rotate-12 place-items-center rounded-full border-2 border-[var(--ink)] bg-[var(--yellow)] text-3xl font-black shadow-[4px_4px_0_var(--ink)]">8</div>
          <form onSubmit={submit} className="relative rounded-[2rem] border-2 border-[var(--ink)] bg-[var(--paper)] p-6 shadow-[8px_8px_0_var(--ink)] sm:p-9">
            <div className="mb-8 grid grid-cols-2 rounded-xl bg-[var(--sand)] p-1.5">
              <button type="button" onClick={() => setMode('create')} className={`rounded-lg px-4 py-3 text-sm font-extrabold transition ${mode === 'create' ? 'bg-white text-[var(--ink)] shadow-sm' : 'text-[var(--muted)]'}`}>Create a room</button>
              <button type="button" onClick={() => setMode('join')} className={`rounded-lg px-4 py-3 text-sm font-extrabold transition ${mode === 'join' ? 'bg-white text-[var(--ink)] shadow-sm' : 'text-[var(--muted)]'}`}>Join a room</button>
            </div>

            <div className="space-y-5">
              {mode === 'create' ? (
                <Field label="Room name" value={roomName} onChange={setRoomName} placeholder="Sprint planning" autoFocus />
              ) : (
                <Field label="Room code" value={code} onChange={(value) => setCode(value.toUpperCase())} placeholder="ABC123" autoFocus />
              )}
              <Field label="Your name" value={userName} onChange={setUserName} placeholder="Ada Lovelace" />
            </div>
            {error && <p role="alert" className="mt-5 rounded-xl bg-red-50 px-4 py-3 text-sm font-bold text-red-700">{error}</p>}
            <button disabled={busy} className="mt-7 w-full rounded-xl border-2 border-[var(--ink)] bg-[var(--green)] px-5 py-4 font-extrabold text-white shadow-[4px_4px_0_var(--ink)] transition hover:-translate-y-0.5 hover:shadow-[5px_5px_0_var(--ink)] disabled:opacity-60">
              {busy ? 'Setting the table...' : mode === 'create' ? 'Create room' : 'Join the team'}
            </button>
          </form>
        </div>
      </section>
    </main>
  )
}

function Field({ label, value, onChange, placeholder, autoFocus = false }: { label: string; value: string; onChange: (value: string) => void; placeholder: string; autoFocus?: boolean }) {
  return (
    <label className="block">
      <span className="mb-2 block text-sm font-extrabold text-[var(--ink)]">{label}</span>
      <input required autoFocus={autoFocus} value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} className="w-full rounded-xl border-2 border-black/15 bg-white px-4 py-3.5 font-bold text-[var(--ink)] outline-none transition placeholder:font-medium placeholder:text-black/25 focus:border-[var(--green)] focus:ring-4 focus:ring-[var(--green)]/10" />
    </label>
  )
}

function PokerRoom({ initial, participantId }: { initial: Room; participantId: string }) {
  const [room, setRoom] = useState(initial)
  const [selected, setSelected] = useState<string | null>(null)
  const [connected, setConnected] = useState(false)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)
  const socketRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    let retry: number
    let stopped = false
    const connect = () => {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const defaultHost = import.meta.env.DEV ? `${window.location.hostname}:8080` : window.location.host
      const websocketBase = import.meta.env.VITE_WS_URL || `${protocol}//${defaultHost}`
      const socket = new WebSocket(`${websocketBase}/ws?roomCode=${room.code}&participantId=${participantId}`)
      socketRef.current = socket
      socket.onopen = () => setConnected(true)
      socket.onmessage = (event) => {
        const message = JSON.parse(event.data)
        if (message.type === 'state') {
          setRoom(message.room)
          if (!message.room.participants.find((person: Participant) => person.id === participantId)?.hasVoted) setSelected(null)
        }
        if (message.type === 'error') setError(message.message)
      }
      socket.onclose = () => {
        setConnected(false)
        if (!stopped) retry = window.setTimeout(connect, 1500)
      }
    }
    connect()
    return () => {
      stopped = true
      window.clearTimeout(retry)
      const socket = socketRef.current
      if (socket?.readyState === WebSocket.OPEN) {
        socket.close(1000, 'Component unmounted')
      } else if (socket?.readyState === WebSocket.CONNECTING) {
        socket.onopen = () => socket.close(1000, 'Component unmounted')
      }
    }
  }, [participantId, room.code])

  const send = (type: string, value?: string) => {
    setError('')
    if (socketRef.current?.readyState !== WebSocket.OPEN) {
      setError('Reconnecting to the room...')
      return
    }
    socketRef.current.send(JSON.stringify({ type, value }))
  }

  const chooseVote = (vote: string) => {
    if (room.revealed) return
    setSelected(vote)
    send('vote', vote)
  }

  const copyInvite = async () => {
    await navigator.clipboard.writeText(`${window.location.origin}/room/${room.code}`)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1800)
  }

  const currentUser = room.participants.find((person) => person.id === participantId)
  const isHost = room.hostId === participantId
  const votedCount = room.participants.filter((person) => person.hasVoted).length
  const online = new Set(room.onlineParticipantIds)

  return (
    <main className="min-h-screen px-4 py-5 sm:px-8">
      <nav className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4 rounded-2xl border border-black/10 bg-white/65 px-5 py-4 backdrop-blur">
        <Brand />
        <div className="flex items-center gap-3">
          <span className={`hidden items-center gap-1.5 text-xs font-extrabold sm:flex ${connected ? 'text-[var(--green)]' : 'text-red-600'}`}>{connected ? <Wifi className="size-4" /> : <WifiOff className="size-4" />}{connected ? 'Live' : 'Reconnecting'}</span>
          <button onClick={copyInvite} className="flex items-center gap-2 rounded-xl border-2 border-[var(--ink)] bg-[var(--yellow)] px-4 py-2.5 text-sm font-extrabold shadow-[3px_3px_0_var(--ink)] transition hover:-translate-y-0.5">
            {copied ? <Check className="size-4" /> : <Copy className="size-4" />}{copied ? 'Copied' : room.code}
          </button>
          <button title="Leave room" onClick={() => (window.location.href = '/')} className="grid size-11 place-items-center rounded-xl border border-black/10 bg-white text-[var(--muted)] transition hover:text-red-600"><LogOut className="size-5" /></button>
        </div>
      </nav>

      <div className="mx-auto mt-8 grid max-w-7xl gap-6 lg:grid-cols-[1fr_300px]">
        <section className="min-w-0 rounded-[2rem] border border-black/10 bg-white/55 p-5 sm:p-8">
          <div className="flex flex-wrap items-end justify-between gap-5">
            <div>
              <div className="mb-2 flex items-center gap-2 text-xs font-black uppercase tracking-[0.16em] text-[var(--green)]"><span>Round {room.round}</span><span className="size-1 rounded-full bg-[var(--coral)]" /><span>{votedCount}/{room.participants.length} voted</span></div>
              <h1 className="text-3xl font-black tracking-[-0.045em] text-[var(--ink)] sm:text-4xl">{room.name}</h1>
            </div>
            {isHost && (
              room.revealed
                ? <button onClick={() => send('reset')} className="action-button"><RefreshCw className="size-5" /> New round</button>
                : <button onClick={() => send('reveal')} disabled={votedCount === 0} className="action-button"><Eye className="size-5" /> Reveal votes</button>
            )}
          </div>

          {error && <p className="mt-5 rounded-xl bg-red-50 px-4 py-3 text-sm font-bold text-red-700">{error}</p>}

          <div className="mt-10 min-h-72 rounded-3xl bg-[var(--green)] p-5 shadow-inner sm:p-8">
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-4">
              {room.participants.map((person) => (
                <div key={person.id} className="flex min-w-0 flex-col items-center">
                  <div className={`relative grid h-28 w-20 place-items-center rounded-xl border-2 border-white/70 text-3xl font-black shadow-[4px_5px_0_rgba(0,0,0,.24)] transition sm:h-32 sm:w-24 ${person.hasVoted ? 'rotate-1 bg-[var(--paper)] text-[var(--ink)]' : 'bg-white/10 text-white/40'}`}>
                    {room.revealed && person.vote ? person.vote === 'coffee' ? <Coffee className="size-9" /> : person.vote : person.hasVoted ? <Check className="size-9 text-[var(--green)]" /> : '...'}
                    {person.id === room.hostId && <span className="absolute -right-3 -top-3 rounded-full border-2 border-[var(--ink)] bg-[var(--yellow)] px-2 py-1 text-[9px] font-black uppercase tracking-wide text-[var(--ink)]">Host</span>}
                  </div>
                  <div className="mt-3 flex max-w-full items-center gap-1.5 text-sm font-extrabold text-white">
                    <span className={`size-2 shrink-0 rounded-full ${online.has(person.id) ? 'bg-[var(--yellow)]' : 'bg-white/25'}`} />
                    <span className="truncate">{person.name}{person.id === participantId ? ' (you)' : ''}</span>
                  </div>
                </div>
              ))}
            </div>
          </div>

          <div className="mt-9">
            <div className="mb-4 flex items-center justify-between">
              <h2 className="font-black text-[var(--ink)]">{room.revealed ? 'Votes revealed' : currentUser?.hasVoted ? 'Vote locked in. You can still change it.' : 'Choose your estimate'}</h2>
              {!room.revealed && selected && <span className="text-sm font-bold text-[var(--green)]">Selected: {selected}</span>}
            </div>
            <div className="flex gap-3 overflow-x-auto pb-4 pt-1">
              {votes.map((vote) => (
                <button key={vote} disabled={room.revealed} onClick={() => chooseVote(vote)} className={`grid h-24 min-w-16 place-items-center rounded-xl border-2 text-xl font-black shadow-[3px_4px_0_var(--ink)] transition enabled:hover:-translate-y-1 disabled:cursor-not-allowed disabled:opacity-45 ${selected === vote ? 'border-[var(--ink)] bg-[var(--coral)] text-[var(--ink)] -translate-y-1' : 'border-[var(--ink)] bg-[var(--paper)] text-[var(--green)]'}`}>
                  {vote === 'coffee' ? <Coffee className="size-6" /> : vote}
                </button>
              ))}
            </div>
          </div>
        </section>

        <aside className="rounded-[2rem] border border-black/10 bg-[var(--paper)] p-6 lg:self-start">
          <div className="flex items-center justify-between">
            <h2 className="flex items-center gap-2 font-black text-[var(--ink)]"><Users className="size-5 text-[var(--green)]" /> Team</h2>
            <span className="rounded-full bg-[var(--sand)] px-3 py-1 text-xs font-black">{room.participants.length}</span>
          </div>
          <div className="mt-6 space-y-3">
            {room.participants.map((person) => (
              <div key={person.id} className="flex items-center gap-3 rounded-xl border border-black/5 bg-white/70 p-3">
                <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-[var(--green)] text-sm font-black text-white">{person.name.slice(0, 1).toUpperCase()}</span>
                <div className="min-w-0 flex-1"><p className="truncate text-sm font-extrabold text-[var(--ink)]">{person.name}</p><p className="text-xs font-semibold text-[var(--muted)]">{person.id === room.hostId ? 'Facilitator' : online.has(person.id) ? 'Online' : 'Offline'}</p></div>
                <span className={`size-2.5 rounded-full ${person.hasVoted ? 'bg-[var(--coral)]' : 'bg-black/10'}`} />
              </div>
            ))}
          </div>
          <div className="mt-7 rounded-2xl bg-[var(--sand)] p-4 text-sm leading-relaxed text-[var(--muted)]">
            <strong className="text-[var(--ink)]">Invite the team</strong><br />Share code <button onClick={copyInvite} className="font-black text-[var(--green)] underline decoration-2 underline-offset-2">{room.code}</button> or copy the room link.
          </div>
        </aside>
      </div>
    </main>
  )
}

function App() {
  const code = roomCodeFromPath()
  const participantId = code ? storedParticipant(code) : null
  const [state, setState] = useState<{ status: 'loading' | 'landing' | 'room'; room?: Room; participantId?: string }>({ status: code && participantId ? 'loading' : 'landing' })

  useEffect(() => {
    if (!code || !participantId) return
    request<Room>(`/api/rooms/${code}?participantId=${encodeURIComponent(participantId)}`)
      .then((room) => setState({ status: 'room', room, participantId }))
      .catch(() => {
        localStorage.removeItem(`pointdeck:${code}`)
        setState({ status: 'landing' })
      })
  }, [code, participantId])

  if (state.status === 'loading') return <div className="grid min-h-screen place-items-center"><RefreshCw className="size-8 animate-spin text-[var(--green)]" /></div>
  if (state.status === 'room' && state.room && state.participantId) return <PokerRoom initial={state.room} participantId={state.participantId} />
  return <Landing defaultCode={code} />
}

export default App
